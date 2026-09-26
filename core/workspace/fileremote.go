package workspace

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FileRemote keeps a project's shared context in a directory: a mounted team
// share, a network volume, or a folder a sync client copies.
//
// Each object is one file, created once under a temporary name and linked into
// place, so a reader never sees half an object and a second writer of one
// name finds it there. A synchronizing client copies whole files it did not
// write and never meets two writers of one file, which is why the layout, and
// not a database, is what lives in a synchronized folder.
type FileRemote struct {
	dir string
}

// NewFileRemote names a directory as a remote. The directory is created on
// the first write.
func NewFileRemote(dir string) *FileRemote { return &FileRemote{dir: filepath.Clean(dir)} }

// Describe reports the directory.
func (r *FileRemote) Describe() RemoteDescriptor {
	return RemoteDescriptor{Kind: "file", Location: r.dir}
}

// List walks one directory of the layout.
func (r *FileRemote) List(ctx context.Context, dir string) ([]string, error) {
	if err := checkListDir(dir); err != nil {
		return nil, err
	}
	if err := r.reachable(); err != nil {
		return nil, err
	}
	root := filepath.Join(r.dir, filepath.FromSlash(dir))
	var out []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(r.dir, p)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(rel)
		// Temporary files of a write in progress, and anything a person or a
		// sync client left beside the layout, are not objects.
		if ValidObjectName(name) {
			out = append(out, name)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("workspace: list %s: %w", root, err)
	}
	sort.Strings(out)
	return out, nil
}

// Get reads one object.
func (r *FileRemote) Get(_ context.Context, name string) ([]byte, error) {
	if err := checkObjectName(name); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(r.path(name))
	if errors.Is(err, fs.ErrNotExist) {
		if rerr := r.reachable(); rerr != nil {
			return nil, rerr
		}
		return nil, fmt.Errorf("%w: %s", ErrNoObject, name)
	}
	if err != nil {
		return nil, fmt.Errorf("workspace: read %s: %w", name, err)
	}
	return data, nil
}

// Put creates each object that is not there yet.
func (r *FileRemote) Put(ctx context.Context, objs ...Object) error {
	for _, obj := range objs {
		if err := checkObjectName(obj.Name); err != nil {
			return err
		}
	}
	for _, obj := range objs {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := r.create(obj); err != nil {
			return err
		}
	}
	return nil
}

// create writes one object under a temporary name and links it into place.
// A link fails when the name exists, which is the create-only write every
// filesystem offers, local or mounted.
func (r *FileRemote) create(obj Object) error {
	final := r.path(obj.Name)
	if err := os.MkdirAll(filepath.Dir(final), 0o755); err != nil {
		return fmt.Errorf("%w: %s: %w", ErrRemoteUnreachable, r.dir, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(final), ".tmp-*")
	if err != nil {
		return fmt.Errorf("workspace: write %s: %w", obj.Name, err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
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
	err = os.Link(tmpName, final)
	if err == nil {
		return nil
	}
	if !errors.Is(err, fs.ErrExist) {
		return fmt.Errorf("workspace: write %s: %w", obj.Name, err)
	}
	held, rerr := os.ReadFile(final)
	if rerr != nil {
		return fmt.Errorf("workspace: read %s: %w", obj.Name, rerr)
	}
	if !bytes.Equal(held, obj.Data) {
		return fmt.Errorf("%w: %s", ErrObjectExists, obj.Name)
	}
	return nil
}

// Close holds nothing open.
func (r *FileRemote) Close() error { return nil }

func (r *FileRemote) path(name string) string {
	return filepath.Join(r.dir, filepath.FromSlash(name))
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
