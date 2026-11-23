package rootpath

import (
	"io"
	"os"
	"path/filepath"
)

// A path beneath a project is project content: a run found it by walking the
// project, or a recipe named it, so it is read through the root. A path that
// lies beneath no root is one a person named, on the command line or in a
// dialog, and is read as named, links followed. Open and ReadFile make that
// decision in one place, from the file system rather than the spelling, so a
// path spelled through another case or another name for the root is still
// beneath it.

// Open opens p for reading. When root is set and p lies beneath it, p is
// opened through OpenRegular: a link that leads out of root is refused, and
// so is a file that is not a regular file. Otherwise p is opened as named.
func Open(root, p string) (*os.File, error) {
	if IsBeneath(root, p) {
		return OpenRegular(root, p)
	}
	return os.Open(p)
}

// ReadFile reads the file p names, as Open opens it.
func ReadFile(root, p string) ([]byte, error) {
	f, err := Open(root, p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

// IsBeneath reports whether p, as named, lies beneath root: by its spelling,
// or because one of the directories above it is root (Beneath). An empty root
// holds nothing.
func IsBeneath(root, p string) bool {
	if root == "" {
		return false
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return false
	}
	_, ok := Beneath(absRoot, abs)
	return ok
}
