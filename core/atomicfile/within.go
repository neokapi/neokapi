package atomicfile

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"

	"github.com/neokapi/neokapi/core/rootpath"
)

// StageWithin is StageWithParents for a destination beneath root, with every
// operation done through an os.Root opened on root. The temporary file is
// created, written and given its mode there. Commit creates the missing
// directories and renames there. os.Root follows a symbolic link only while
// it stays inside root. A link that leads out of root, even one put in place
// after the caller checked the path, is refused at the moment of the
// operation, and no byte is written outside root.
//
// A destination that is a link to a file inside root is written through: the
// file it points at is replaced and the link stays a link. A link whose file
// does not exist is replaced by a regular file. path must lie beneath root as
// written, or beneath the directory root resolves to.
func StageWithin(root, path string, write func(io.Writer) error) (*Staged, error) {
	return stageWithinRoot(root, path, nil, write)
}

// StageWithinMode is StageWithin for a file that takes mode, whatever mode a
// file already at path has: a backup copy, which keeps the mode of the file
// it copies.
func StageWithinMode(root, path string, mode fs.FileMode, write func(io.Writer) error) (*Staged, error) {
	return stageWithinRoot(root, path, &mode, write)
}

func stageWithinRoot(root, path string, force *fs.FileMode, write func(io.Writer) error) (*Staged, error) {
	realRoot, err := realDir(root)
	if err != nil {
		return nil, &Error{Op: "replace", Path: path, Err: err}
	}
	rel, err := resolveTarget(realRoot, path)
	if err != nil {
		if errors.Is(err, errOutsideRoot) {
			// Named as rootpath names it: by the link, relative to the root,
			// never by where it points or by the host's directories.
			if _, cerr := rootpath.Contain(root, path); cerr != nil {
				return nil, cerr
			}
		}
		return nil, &Error{Op: "replace", Path: path, Err: err}
	}
	r, err := os.OpenRoot(realRoot)
	if err != nil {
		return nil, &Error{Op: "replace", Path: path, Err: err}
	}
	s, err := stageWithin(r, rel, force, write)
	if err != nil {
		_ = r.Close()
		return nil, err
	}
	s.abs = filepath.Join(realRoot, rel)
	return s, nil
}

var errOutsideRoot = errors.New("the path leads outside the root")

// realDir is the absolute path dir resolves to.
func realDir(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

// resolveTarget returns the file a write to path lands on, relative to
// realRoot, with every symbolic link resolved against the directory that
// really holds it. A link to a file that exists is followed, an absolute one
// included when it leads inside the root. A link whose file does not exist is
// the target itself, so the write replaces it. A missing directory is kept as
// a name beneath the nearest one that exists. A target that resolves outside
// realRoot is refused.
func resolveTarget(realRoot, path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	var real string
	if info, lerr := os.Lstat(abs); lerr == nil && info.Mode()&fs.ModeSymlink != 0 {
		if _, serr := os.Stat(abs); errors.Is(serr, fs.ErrNotExist) {
			// A link to a missing file: it is replaced, never followed.
			parent, perr := resolveExisting(filepath.Dir(abs))
			if perr != nil {
				return "", perr
			}
			real = filepath.Join(parent, filepath.Base(abs))
		}
	}
	if real == "" {
		if real, err = resolveExisting(abs); err != nil {
			return "", err
		}
	}
	rel, ok := rootpath.Beneath(realRoot, real)
	if !ok || rel == "." {
		return "", fmt.Errorf("%s: %w", path, errOutsideRoot)
	}
	return rel, nil
}

// resolveExisting resolves the nearest part of p that exists and appends the
// names below it that do not.
func resolveExisting(p string) (string, error) {
	var missing []string
	cur := p
	for {
		real, err := filepath.EvalSymlinks(cur)
		if err == nil {
			for _, name := range slices.Backward(missing) {
				real = filepath.Join(real, name)
			}
			return real, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		if info, lerr := os.Lstat(cur); lerr == nil && info.Mode()&fs.ModeSymlink != 0 {
			// A directory link whose target is missing: where a file
			// beneath it would land cannot be decided.
			return "", fmt.Errorf("%s: %w", p, errOutsideRoot)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", err
		}
		missing = append(missing, filepath.Base(cur))
		cur = parent
	}
}

// stageWithin stages the file at target, a path relative to the root with no
// link in it, through r. A link put in place after resolveTarget is refused by
// r at the moment of each operation.
func stageWithin(r *os.Root, target string, force *fs.FileMode, write func(io.Writer) error) (*Staged, error) {
	mode, exists := newFileMode, false
	if info, err := r.Stat(target); err == nil {
		if !info.Mode().IsRegular() {
			return nil, &Error{Op: "replace", Path: target, Err: errors.New("not a regular file")}
		}
		mode, exists = info.Mode().Perm(), true
	}
	dir, mkdir, err := existingDirWithin(r, filepath.Dir(target))
	if err != nil {
		return nil, &Error{Op: "create a temporary file beside", Path: target, Err: err}
	}
	tmp, name, err := createTempWithin(r, dir, filepath.Base(target), mode)
	if err != nil {
		return nil, &Error{Op: "create a temporary file beside", Path: target, Err: err}
	}
	staged := false
	defer func() {
		if !staged {
			_ = tmp.Close()
			_ = r.Remove(name)
		}
	}()
	werr := write(tmp)
	// The mode is set on the open file, before it is closed, so nothing is
	// done to it by name.
	var merr error
	if force != nil {
		mode, exists = *force, true
	}
	if exists {
		merr = tmp.Chmod(mode)
	}
	cerr := tmp.Close()
	if err := firstError(werr, merr, cerr); err != nil {
		return nil, err
	}
	staged = true
	return &Staged{target: target, name: name, mkdir: mkdir, root: r}, nil
}

// existingDirWithin is existingDir inside the root.
func existingDirWithin(r *os.Root, dir string) (string, []string, error) {
	var missing []string
	for {
		info, err := r.Stat(dir)
		switch {
		case err == nil && info.IsDir():
			slices.Reverse(missing)
			return dir, missing, nil
		case err == nil:
			return "", nil, fmt.Errorf("%s is not a directory", dir)
		case !errors.Is(err, fs.ErrNotExist):
			return "", nil, err
		}
		if dir == "." {
			return "", nil, err
		}
		missing = append(missing, dir)
		dir = filepath.Dir(dir)
	}
}

// createTempWithin is createTemp inside the root.
func createTempWithin(r *os.Root, dir, base string, mode fs.FileMode) (*os.File, string, error) {
	var err error
	for range 1000 {
		name := filepath.Join(dir, fmt.Sprintf(".%s.kapi-%d", base, rand.Uint64()))
		var f *os.File
		f, err = r.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, mode)
		if err == nil {
			return f, name, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, "", err
		}
	}
	return nil, "", err
}

// ReadAll reads the staged bytes back, through the root for a file staged
// with StageWithin.
func (s *Staged) ReadAll() ([]byte, error) {
	if s.root == nil {
		return os.ReadFile(s.name)
	}
	return s.root.ReadFile(s.name)
}
