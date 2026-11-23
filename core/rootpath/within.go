package rootpath

import (
	"io"
	"os"
	"slices"
	"strings"
)

// OpenWithin opens p for reading, for a path that content names rather than a
// person: a media reference in the content model, or a file a recipe names.
// With root set, p must lie beneath it and is opened through OpenRegular, so
// a path beneath no root is refused rather than opened as named. With root
// empty there is no project, every path came from a person, and p is opened
// as named.
func OpenWithin(root, p string) (*os.File, error) {
	if root == "" {
		return os.Open(p)
	}
	if !IsBeneath(root, p) {
		return nil, &EscapeError{Path: relativeTo(root, p)}
	}
	return OpenRegular(root, p)
}

// ReadDir lists the directory p beneath root. p is resolved as Contain
// resolves it and listed through an os.Root opened on root, so a directory
// that leads out of root, by a link on the way or one swapped in afterwards,
// is refused.
func ReadDir(root, p string) ([]os.DirEntry, error) {
	real, err := Contain(root, p)
	if err != nil {
		return nil, err
	}
	realRoot, err := realDir(root)
	if err != nil {
		return nil, err
	}
	rel, ok := Beneath(realRoot, real)
	if !ok {
		return nil, &EscapeError{Path: relativeTo(root, p)}
	}
	r, err := os.OpenRoot(realRoot)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	d, err := r.Open(rel)
	if err != nil {
		return nil, explain(root, p, err)
	}
	defer d.Close()
	// Sorted by name, as os.ReadDir returns them: a file's ReadDir returns
	// directory order.
	entries, err := d.ReadDir(-1)
	slices.SortFunc(entries, func(a, b os.DirEntry) int { return strings.Compare(a.Name(), b.Name()) })
	return entries, err
}

// Stat returns the file information of what p beneath root resolves to,
// links followed, and an *EscapeError when that lies outside root.
func Stat(root, p string) (os.FileInfo, error) {
	real, err := Contain(root, p)
	if err != nil {
		return nil, err
	}
	return os.Lstat(real)
}

// ReadFileWithin reads the file p names, as OpenWithin opens it.
func ReadFileWithin(root, p string) ([]byte, error) {
	f, err := OpenWithin(root, p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}
