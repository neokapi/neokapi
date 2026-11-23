package workspace

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// FileRemote keeps a project's shared context in a directory: a mounted team
// share, a network volume, or a folder a sync client copies.
//
// Each object is one file, created once under a temporary name and linked into
// place, so a reader never sees half an object and a second writer of one
// name finds it there. A synchronizing client copies whole files it did not
// write and never meets two writers of one file, which is why the layout, and
// not a database, is what lives in a synchronized folder.
//
// The directory is opened once, as an os.Root, and every read and write goes
// beneath it. A link that another writer of the folder plants inside it
// cannot lead a read or a write out of the folder, and a change to the path
// after the open does not move the remote.
type FileRemote struct {
	dir string

	mu   sync.Mutex
	root *os.Root
}

// NewFileRemote names a directory as a remote. The directory is created on
// the first write.
func NewFileRemote(dir string) *FileRemote { return &FileRemote{dir: filepath.Clean(dir)} }

// Describe reports the directory.
func (r *FileRemote) Describe() RemoteDescriptor {
	return RemoteDescriptor{Kind: "file", Location: r.dir}
}

// open returns the directory opened as a root, opening it on the first call.
// A directory that is not there yet is nil with no error, unless create is
// set, which makes it first.
func (r *FileRemote) open(create bool) (*os.Root, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.root != nil {
		return r.root, nil
	}
	root, err := os.OpenRoot(r.dir)
	if errors.Is(err, fs.ErrNotExist) {
		if !create {
			return nil, r.reachable()
		}
		root, err = makeRoot(r.dir)
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %w", ErrRemoteUnreachable, r.dir, err)
	}
	r.root = root
	return root, nil
}

// makeRoot creates dir beneath its nearest existing ancestor, opened as a
// root itself, and opens the result.
func makeRoot(dir string) (*os.Root, error) {
	base := filepath.Dir(dir)
	for {
		if _, err := os.Stat(base); err == nil {
			break
		}
		next := filepath.Dir(base)
		if next == base {
			break
		}
		base = next
	}
	parent, err := os.OpenRoot(base)
	if err != nil {
		return nil, err
	}
	defer func() { _ = parent.Close() }()
	rel, err := filepath.Rel(base, dir)
	if err != nil {
		return nil, err
	}
	if err := parent.MkdirAll(rel, 0o755); err != nil {
		return nil, err
	}
	return parent.OpenRoot(rel)
}

// List walks one directory of the layout.
func (r *FileRemote) List(ctx context.Context, dir string) ([]string, error) {
	if err := checkListDir(dir); err != nil {
		return nil, err
	}
	root, err := r.open(false)
	if err != nil || root == nil {
		return nil, err
	}
	start := strings.Trim(dir, "/")
	if start == "" {
		start = "."
	}
	var out []string
	err = fs.WalkDir(root.FS(), start, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// A link is neither walked into nor listed: an object is a file the
		// layout wrote.
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		// Temporary files of a write in progress, and anything a person or a
		// sync client left beside the layout, are not objects.
		if ValidObjectName(p) {
			out = append(out, p)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("workspace: list %s: %w", filepath.Join(r.dir, filepath.FromSlash(dir)), err)
	}
	sort.Strings(out)
	return out, nil
}

// Get reads one object.
func (r *FileRemote) Get(_ context.Context, name string) ([]byte, error) {
	if err := checkObjectName(name); err != nil {
		return nil, err
	}
	root, err := r.open(false)
	if err != nil {
		return nil, err
	}
	if root == nil {
		return nil, fmt.Errorf("%w: %s", ErrNoObject, name)
	}
	data, err := readRegular(root, filepath.FromSlash(name))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrNoObject, name)
	}
	if err != nil {
		return nil, fmt.Errorf("workspace: read %s: %w", name, err)
	}
	return data, nil
}

// readRegular reads a regular file beneath root and refuses anything else,
// a link among them.
func readRegular(root *os.Root, name string) ([]byte, error) {
	info, err := root.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", name)
	}
	return root.ReadFile(name)
}

// Put creates each object that is not there yet.
func (r *FileRemote) Put(ctx context.Context, objs ...Object) error {
	for _, obj := range objs {
		if err := checkObjectName(obj.Name); err != nil {
			return err
		}
	}
	if len(objs) == 0 {
		return nil
	}
	root, err := r.open(true)
	if err != nil {
		return err
	}
	for _, obj := range objs {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := create(root, obj); err != nil {
			return err
		}
	}
	return nil
}

// create writes one object under a temporary name and links it into place.
// A link fails when the name exists, which is the create-only write every
// filesystem offers, local or mounted.
func create(root *os.Root, obj Object) error {
	final := filepath.FromSlash(obj.Name)
	if err := root.MkdirAll(filepath.Dir(final), 0o755); err != nil {
		return fmt.Errorf("%w: %s: %w", ErrRemoteUnreachable, root.Name(), err)
	}
	tmp, tmpName, err := createTemp(root, filepath.Dir(final))
	if err != nil {
		return fmt.Errorf("workspace: write %s: %w", obj.Name, err)
	}
	defer func() { _ = root.Remove(tmpName) }()
	if _, err := tmp.Write(obj.Data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("workspace: write %s: %w", obj.Name, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("workspace: write %s: %w", obj.Name, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("workspace: write %s: %w", obj.Name, err)
	}
	err = root.Link(tmpName, final)
	if err == nil {
		return nil
	}
	if !errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("workspace: write %s: %w", obj.Name, err)
	}
	held, rerr := readRegular(root, final)
	if rerr != nil {
		return fmt.Errorf("workspace: read %s: %w", obj.Name, rerr)
	}
	if !bytes.Equal(held, obj.Data) {
		return fmt.Errorf("%w: %s", ErrObjectExists, obj.Name)
	}
	return nil
}

// createTemp makes a new file named .tmp-<random> in dir beneath root.
func createTemp(root *os.Root, dir string) (*os.File, string, error) {
	for range 16 {
		var b [8]byte
		if _, err := rand.Read(b[:]); err != nil {
			return nil, "", err
		}
		name := filepath.Join(dir, ".tmp-"+hex.EncodeToString(b[:]))
		f, err := root.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		return f, name, err
	}
	return nil, "", errors.New("no free temporary name")
}

// Close releases the opened directory.
func (r *FileRemote) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.root == nil {
		return nil
	}
	err := r.root.Close()
	r.root = nil
	return err
}

// reachable reports a remote directory that is not there as unreachable
// rather than empty, when its parent is missing too: a share that is not
// mounted looks like that. A missing directory under a parent that exists is a
// remote nobody has written to yet.
func (r *FileRemote) reachable() error {
	if _, err := os.Stat(r.dir); err == nil {
		return nil
	}
	parent := filepath.Dir(r.dir)
	if _, err := os.Stat(parent); err != nil {
		return fmt.Errorf("%w: %s is not there (%s)", ErrRemoteUnreachable, r.dir, strings.TrimPrefix(err.Error(), "stat "))
	}
	return nil
}
