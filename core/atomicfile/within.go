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
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, &Error{Op: "replace", Path: path, Err: err}
	}
	rel, err := relativeTo(absRoot, path)
	if err != nil {
		return nil, &Error{Op: "replace", Path: path, Err: err}
	}
	r, err := os.OpenRoot(absRoot)
	if err != nil {
		return nil, &Error{Op: "replace", Path: path, Err: err}
	}
	s, err := stageWithin(r, rel, write)
	if err != nil {
		_ = r.Close()
		if errors.Is(err, errOutsideRoot) {
			return nil, &Error{Op: "replace", Path: path, Err: err}
		}
		return nil, err
	}
	s.abs = filepath.Join(absRoot, s.target)
	return s, nil
}

var errOutsideRoot = errors.New("the path leads outside the root")

func stageWithin(r *os.Root, rel string, write func(io.Writer) error) (*Staged, error) {
	target, err := resolveWithin(r, rel)
	if err != nil {
		return nil, err
	}
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
	cerr := tmp.Close()
	var merr error
	if exists {
		merr = r.Chmod(name, mode)
	}
	if err := firstError(werr, cerr, merr); err != nil {
		return nil, err
	}
	staged = true
	return &Staged{target: target, name: name, mkdir: mkdir, root: r}, nil
}

// resolveWithin follows the symbolic links at rel, inside the root, to the
// file a write lands on. A link to a file that does not exist resolves to the
// link itself. A link that leads out of the root is refused.
func resolveWithin(r *os.Root, rel string) (string, error) {
	cur := rel
	for range 40 {
		info, err := r.Lstat(cur)
		if err != nil || info.Mode()&fs.ModeSymlink == 0 {
			// Missing, or not a link: the name itself. A directory link
			// above it is followed, or refused, by each operation.
			return cur, nil
		}
		if _, err := r.Stat(cur); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return cur, nil
			}
			return "", fmt.Errorf("%s: %w", cur, errOutsideRoot)
		}
		dest, err := r.Readlink(cur)
		if err != nil {
			return "", err
		}
		if filepath.IsAbs(dest) {
			return "", fmt.Errorf("%s: %w", cur, errOutsideRoot)
		}
		next := filepath.Join(filepath.Dir(cur), dest)
		if !filepath.IsLocal(next) {
			return "", fmt.Errorf("%s: %w", cur, errOutsideRoot)
		}
		cur = next
	}
	return "", fmt.Errorf("%s: too many links", rel)
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
