// Package atomicfile replaces what a file holds without replacing the file.
//
// The obvious way to write a file safely is to write a temporary file beside
// it and rename that onto the path. The rename is what makes the write atomic:
// a reader sees the old bytes or the new ones, never half of either, and a
// failed write leaves the file as it was.
//
// It also replaces the directory entry, which is a different thing from
// replacing the contents. Handed a symlink, rename puts a regular file where
// the link was and leaves the file the link pointed at holding the old text,
// while the write reports success. Sharing one file between trees through a
// link is ordinary, so the path a person names is often not the file. The
// temporary file's mode travels with it too: it belongs to its owner alone,
// so a file that was readable stops being readable.
//
// Replace resolves the link first, renames onto the file itself, and gives
// that file the mode it had. Stage does the same in two steps, for a caller
// that writes several files and renames them only once every one is ready.
package atomicfile

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
)

// newFileMode is the mode a file that does not exist yet is created with. It is
// what os.Create uses, so the umask decides the rest and a file written here
// carries the mode every other file the engine writes carries.
const newFileMode fs.FileMode = 0o666

// Error is a failure of the replacement itself: resolving the path, creating
// the temporary file, or renaming it onto the destination. An error from the
// write is returned as it is, so a caller can tell a failure of its own work
// from a failure at the destination.
type Error struct {
	Op   string
	Path string
	Err  error
}

func (e *Error) Error() string { return fmt.Sprintf("%s %s: %v", e.Op, e.Path, e.Err) }
func (e *Error) Unwrap() error { return e.Err }

// Resolve returns the file a write to path should land on, the mode that file
// carries, and whether it exists. A symlink resolves to the file it points at,
// so writing there leaves the link a link. A path that does not exist resolves
// to itself; Resolve creates nothing.
func Resolve(path string) (target string, mode fs.FileMode, exists bool, err error) {
	target = path
	if resolved, rerr := filepath.EvalSymlinks(path); rerr == nil {
		target = resolved
	}
	info, serr := os.Stat(target)
	switch {
	case serr == nil:
		return target, info.Mode().Perm(), true, nil
	case errors.Is(serr, fs.ErrNotExist):
		return target, newFileMode, false, nil
	default:
		return "", 0, false, &Error{Op: "read", Path: path, Err: serr}
	}
}

// Replace writes what write produces onto path: atomically, onto the file a
// symlink points at rather than onto the link, and keeping the mode an existing
// file has. It returns the path the bytes landed on.
//
// An error from write, or from the replacement, leaves the file as it was and
// removes the temporary file.
//
// A hard-linked file is the limit the rename cannot cross: the path gets a new
// inode, so the file's other names keep the old contents.
func Replace(path string, write func(io.Writer) error) (string, error) {
	s, err := Stage(path, write)
	if err != nil {
		return "", err
	}
	if err := s.Commit(); err != nil {
		return "", err
	}
	return s.Target(), nil
}

// Staged is a replacement written to a temporary file beside its destination
// and not yet renamed onto it. Commit puts it in place; Discard removes it.
// Between the two, the destination is as it was, so a caller can stage several
// files and decide afterwards whether any of them lands.
type Staged struct {
	target string
	name   string
	done   bool
}

// Stage writes what write produces to a temporary file beside the file a
// write to path should land on (Resolve), with the mode that file has, and
// returns it staged. An error from write, or from creating the temporary file,
// leaves nothing behind.
func Stage(path string, write func(io.Writer) error) (*Staged, error) {
	target, mode, exists, err := Resolve(path)
	if err != nil {
		return nil, err
	}
	tmp, err := createTemp(filepath.Dir(target), filepath.Base(target), mode)
	if err != nil {
		return nil, &Error{Op: "create a temporary file beside", Path: target, Err: err}
	}
	name := tmp.Name()
	werr := write(tmp)
	cerr := tmp.Close()
	// A temporary file is created under the umask, which is right for a file
	// that did not exist and wrong for one that did: that file keeps the mode
	// it had, whatever the umask would take off.
	var merr error
	if exists {
		merr = os.Chmod(name, mode)
	}
	if err := firstError(werr, cerr, merr); err != nil {
		_ = os.Remove(name)
		return nil, err
	}
	return &Staged{target: target, name: name}, nil
}

// Target is the file Commit replaces: the path the replacement was staged
// for, with any symlink resolved.
func (s *Staged) Target() string { return s.target }

// Name is the temporary file that holds the staged bytes.
func (s *Staged) Name() string { return s.name }

// Commit renames the staged file onto its target. A failed rename removes
// the staged file and leaves the target as it was.
func (s *Staged) Commit() error {
	if s.done {
		return &Error{Op: "replace", Path: s.target, Err: errors.New("the staged file was already committed or discarded")}
	}
	s.done = true
	if err := os.Rename(s.name, s.target); err != nil {
		_ = os.Remove(s.name)
		return &Error{Op: "replace", Path: s.target, Err: err}
	}
	return nil
}

// Discard removes the staged file. It is safe after Commit, and safe to call
// more than once.
func (s *Staged) Discard() error {
	if s == nil || s.done {
		return nil
	}
	s.done = true
	if err := os.Remove(s.name); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// ReplaceBytes writes data onto path, as Replace does.
func ReplaceBytes(path string, data []byte) (string, error) {
	return Replace(path, func(w io.Writer) error {
		_, err := w.Write(data)
		return err
	})
}

// createTemp makes a new file in dir, named after base so a leftover says which
// write left it, with the given mode. os.CreateTemp is not used because it
// fixes the mode at 0600, which a file that did not exist would then keep.
func createTemp(dir, base string, mode fs.FileMode) (*os.File, error) {
	var err error
	for range 1000 {
		name := filepath.Join(dir, fmt.Sprintf(".%s.kapi-%d", base, rand.Uint64()))
		var f *os.File
		f, err = os.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, mode)
		if err == nil {
			return f, nil
		}
		if !errors.Is(err, fs.ErrExist) {
			return nil, err
		}
	}
	return nil, err
}

func firstError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
