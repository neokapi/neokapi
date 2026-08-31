package flow

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

// A run's destination is decided before the pipeline runs: a target path that
// cannot be written must refuse the run up front, with a reason a person can
// act on. The failure mode this closes (#1449) is a *blocked* path — most often
// a directory standing where the writer needs a file — where the underlying
// syscall reports something structurally true but practically useless
// ("rename …: file exists") and a partially-informed caller drops it.
//
// Two entry points, one vocabulary:
//
//   - CheckOutputPath is the pre-flight. Call it before doing any work toward a
//     destination (before the tools run, so a blocked path never costs an LLM
//     call).
//   - ClassifyOutputPathError wraps a write/rename/mkdir failure that happened
//     anyway — a race, or a condition the pre-flight cannot see.
//
// Both return *OutputPathError, so a UI can classify with errors.As rather than
// by matching prose (the sentence is copy; the type is the contract).

// OutputPathErrorKind says why a destination cannot be written.
type OutputPathErrorKind int

const (
	// OutputPathIsDir — a directory occupies the destination path.
	OutputPathIsDir OutputPathErrorKind = iota
	// OutputPathIrregular — something that is neither a regular file nor a
	// directory occupies the path (a device, socket, or named pipe).
	OutputPathIrregular
	// OutputPathParentNotDir — an ancestor of the destination is a file, so
	// the containing directory cannot exist.
	OutputPathParentNotDir
	// OutputPathPermission — the destination (or its directory) denies writing.
	OutputPathPermission
	// OutputPathReadOnly — the destination is on a read-only filesystem.
	OutputPathReadOnly
	// OutputPathOther — a write failure none of the above explains.
	OutputPathOther
)

// OutputPathError reports that a run's destination cannot be written. It names
// the path, says what is in the way, and says what to do about it.
type OutputPathError struct {
	Kind OutputPathErrorKind
	// Path is the destination the run wanted to write.
	Path string
	// Dir is the directory implicated when the fault is about the container
	// rather than the destination itself (parent-not-a-directory, permission).
	// Empty when the destination itself is the fault.
	Dir string
	// Mode is the file mode found at Path, for OutputPathIrregular.
	Mode fs.FileMode
	// Err is the underlying filesystem error, when there was one.
	Err error
}

func (e *OutputPathError) Error() string {
	switch e.Kind {
	case OutputPathIsDir:
		return fmt.Sprintf("cannot write %s: a directory occupies that path — remove or rename the directory, or point the target at a different path", e.Path)
	case OutputPathIrregular:
		return fmt.Sprintf("cannot write %s: the path is not a regular file (mode %s) — remove it, or point the target at a different path", e.Path, e.Mode)
	case OutputPathParentNotDir:
		// "is not a directory" rather than "is a file": the obstruction is often
		// a device or a symlink to one (`-o /dev/null`), and calling that a file
		// would be wrong where being precise costs nothing.
		return fmt.Sprintf("cannot write %s: %s is not a directory — the output directory cannot be created under it", e.Path, e.Dir)
	case OutputPathPermission:
		where := e.Dir
		if where == "" {
			where = e.Path
		}
		return fmt.Sprintf("cannot write %s: permission denied — check the write permissions on %s", e.Path, where)
	case OutputPathReadOnly:
		return fmt.Sprintf("cannot write %s: the filesystem is mounted read-only — write to a writable location", e.Path)
	default:
		if e.Err != nil {
			return fmt.Sprintf("cannot write %s: %v", e.Path, e.Err)
		}
		return "cannot write " + e.Path
	}
}

func (e *OutputPathError) Unwrap() error { return e.Err }

// CheckOutputPath reports whether path is usable as a run's destination. It is
// the pre-flight: nil means the writer may proceed (the path is free, or holds a
// regular file to replace), and a non-nil *OutputPathError names the
// obstruction. An empty path is not a destination and passes.
//
// It deliberately does not attempt a write: a probe file at the destination
// would be indistinguishable from real output if the process died between the
// probe and the run.
func CheckOutputPath(path string) error {
	if path == "" {
		return nil
	}
	st, err := os.Stat(path)
	switch {
	case err == nil && st.IsDir():
		return &OutputPathError{Kind: OutputPathIsDir, Path: path}
	case err == nil && !st.Mode().IsRegular():
		return &OutputPathError{Kind: OutputPathIrregular, Path: path, Mode: st.Mode()}
	case err == nil:
		return nil // a regular file the writer replaces
	case !errors.Is(err, fs.ErrNotExist):
		return ClassifyOutputPathError(path, filepath.Dir(path), err)
	}
	// The destination does not exist: the nearest EXISTING ancestor must be a
	// directory, else MkdirAll cannot create the output directory.
	dir := filepath.Dir(path)
	for {
		st, err := os.Stat(dir)
		switch {
		case err == nil && st.IsDir():
			return nil
		case err == nil:
			return &OutputPathError{Kind: OutputPathParentNotDir, Path: path, Dir: dir}
		case !errors.Is(err, fs.ErrNotExist):
			return ClassifyOutputPathError(path, dir, err)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil // walked to the root without finding an obstruction
		}
		dir = parent
	}
}

// ClassifyOutputPathError turns a filesystem failure against a destination into
// an *OutputPathError naming the path and the remedy. It re-reads the
// destination first, so a raw errno the caller cannot interpret ("file exists"
// from a rename onto a directory) still reports the real obstruction. Already
// classified errors pass through unchanged; a nil error stays nil.
func ClassifyOutputPathError(path, dir string, err error) error {
	if err == nil {
		return nil
	}
	if _, ok := errors.AsType[*OutputPathError](err); ok {
		return err
	}
	// The obstruction itself is more informative than the errno: a rename onto
	// an existing directory reports EEXIST/ENOTEMPTY/EISDIR depending on the
	// platform, none of which name the directory.
	if st, serr := os.Stat(path); serr == nil {
		if st.IsDir() {
			return &OutputPathError{Kind: OutputPathIsDir, Path: path, Err: err}
		}
		if !st.Mode().IsRegular() {
			return &OutputPathError{Kind: OutputPathIrregular, Path: path, Mode: st.Mode(), Err: err}
		}
	}
	switch {
	case errors.Is(err, fs.ErrPermission):
		return &OutputPathError{Kind: OutputPathPermission, Path: path, Dir: dir, Err: err}
	case errors.Is(err, syscall.EROFS):
		return &OutputPathError{Kind: OutputPathReadOnly, Path: path, Dir: dir, Err: err}
	case errors.Is(err, syscall.ENOTDIR):
		return &OutputPathError{Kind: OutputPathParentNotDir, Path: path, Dir: dir, Err: err}
	default:
		return &OutputPathError{Kind: OutputPathOther, Path: path, Dir: dir, Err: err}
	}
}
