// Package rootpath keeps reads and writes inside a root directory once
// symbolic links are resolved.
//
// A lexical check (filepath.Join, then filepath.IsLocal or a prefix test)
// answers for the path as written. The file system answers for the path as
// resolved: a link anywhere in it can lead out of the root, and a directory
// link turns every new file created beneath it into a file outside the root.
// Contain resolves the nearest part of the path that exists, decides from the
// resolved path, and refuses rather than rewrites. A root that is itself
// reached through a link (/tmp and /var on macOS) is resolved first, so a
// path inside it stays inside.
//
// OpenRegular and CreateRegular then open through os.Root, which refuses a
// link that leads out of the root at the moment of the open, so a link
// swapped in after Contain is refused too.
package rootpath

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
)

// EscapeError says a path leads outside its root. Link is the symbolic link,
// relative to the root, through which it does; it names the path itself when
// that is the link. The error never names where the link points.
type EscapeError struct {
	Path string
	Link string
}

func (e *EscapeError) Error() string {
	if e.Link == "" || e.Link == e.Path {
		return e.Path + " is a symbolic link that leads outside the root"
	}
	return fmt.Sprintf("%s: the symbolic link %s leads outside the root", e.Path, e.Link)
}

// ErrNotRegular is the error OpenRegular and CheckRegular return for a path
// that is not a regular file: a directory, a device, a FIFO or a socket.
var ErrNotRegular = errors.New("not a regular file")

// Contain returns the path p resolves to, and an *EscapeError when that path
// lies outside root. p is absolute or relative to the working directory, and
// must already lie under root as written. When p does not exist, the nearest
// ancestor that does is resolved and the missing names are appended to it, so
// a new file beneath a directory link is judged by where the link leads. A
// link to a missing file is refused: what it would create cannot be decided.
func Contain(root, p string) (string, error) {
	realRoot, err := realDir(root)
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	rel := relativeTo(root, abs)
	cur := abs
	var missing []string
	for {
		real, rerr := filepath.EvalSymlinks(cur)
		if rerr == nil {
			slices.Reverse(missing)
			resolved := filepath.Join(append([]string{real}, missing...)...)
			if !inside(realRoot, real) {
				return "", &EscapeError{Path: rel, Link: firstLink(root, abs)}
			}
			return resolved, nil
		}
		if !errors.Is(rerr, fs.ErrNotExist) {
			return "", rerr
		}
		if info, lerr := os.Lstat(cur); lerr == nil && info.Mode()&fs.ModeSymlink != 0 {
			// A link whose target is missing, at cur or below it.
			return "", &EscapeError{Path: rel, Link: firstLink(root, abs)}
		}
		missing = append(missing, filepath.Base(cur))
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", rerr
		}
		cur = parent
	}
}

// CheckRegular returns the resolved path of p when it lies inside root and
// is a regular file.
func CheckRegular(root, p string) (string, error) {
	real, err := Contain(root, p)
	if err != nil {
		return "", err
	}
	info, err := os.Lstat(real)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%s: %w", relativeTo(root, p), ErrNotRegular)
	}
	return real, nil
}

// OpenRegular opens p for reading when it is a regular file inside root. The
// open goes through os.Root, which follows a symbolic link only while it stays
// inside root, so a link swapped in after Contain is refused rather than read.
// A FIFO is opened without blocking and refused.
func OpenRegular(root, p string) (*os.File, error) {
	if _, err := Contain(root, p); err != nil {
		return nil, err
	}
	r, rel, err := openRoot(root, p)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	f, err := r.OpenFile(rel, os.O_RDONLY|openNonBlock, 0)
	if err != nil {
		return nil, explain(root, p, err)
	}
	info, err := f.Stat()
	if err == nil && !info.Mode().IsRegular() {
		err = fmt.Errorf("%s: %w", relativeTo(root, p), ErrNotRegular)
	}
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

// CreateRegular creates or truncates p for writing when it lies inside root,
// creating the missing directories first. Both go through os.Root, so no
// directory is created and no file is written through a link that leads out
// of root, even one swapped in after Contain.
func CreateRegular(root, p string) (*os.File, error) {
	if _, err := Contain(root, p); err != nil {
		return nil, err
	}
	r, rel, err := openRoot(root, p)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	if dir := filepath.Dir(rel); dir != "." {
		if err := r.MkdirAll(dir, 0o755); err != nil {
			return nil, explain(root, p, err)
		}
	}
	// No O_TRUNC: the file is cut only once it is known to be a regular file.
	f, err := r.OpenFile(rel, os.O_WRONLY|os.O_CREATE|openNonBlock, 0o666)
	if err != nil {
		return nil, explain(root, p, err)
	}
	info, err := f.Stat()
	if err == nil && !info.Mode().IsRegular() {
		err = fmt.Errorf("%s: %w", relativeTo(root, p), ErrNotRegular)
	}
	if err == nil {
		err = f.Truncate(0)
	}
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}

// Remove removes the file p beneath root through os.Root, so a link that
// leads out of root, even one put in place after a check, is refused.
func Remove(root, p string) error {
	r, rel, err := openRoot(root, p)
	if err != nil {
		return err
	}
	defer r.Close()
	if err := r.Remove(rel); err != nil {
		return explain(root, p, err)
	}
	return nil
}

// openRoot opens root and returns p relative to it.
func openRoot(root, p string) (*os.Root, string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, "", err
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return nil, "", err
	}
	rel, err := filepath.Rel(absRoot, abs)
	if err != nil || !filepath.IsLocal(rel) {
		return nil, "", &EscapeError{Path: filepath.Base(p)}
	}
	r, err := os.OpenRoot(absRoot)
	if err != nil {
		return nil, "", err
	}
	return r, rel, nil
}

// explain turns an os.Root refusal into an *EscapeError naming the link when
// the path now leads outside root, and returns any other error as it is.
func explain(root, p string, err error) error {
	if _, cerr := Contain(root, p); cerr != nil {
		return cerr
	}
	return err
}

// realDir resolves root, which must exist.
func realDir(root string) (string, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}

// inside reports whether p is root or lies beneath it.
func inside(root, p string) bool {
	rel, err := filepath.Rel(root, p)
	return err == nil && (rel == "." || filepath.IsLocal(rel))
}

// relativeTo names p relative to root for an error, falling back to its base
// name so an error never carries the host's directory layout.
func relativeTo(root, p string) string {
	absRoot, err1 := filepath.Abs(root)
	abs, err2 := filepath.Abs(p)
	if err1 == nil && err2 == nil {
		if rel, err := filepath.Rel(absRoot, abs); err == nil && filepath.IsLocal(rel) {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.Base(p)
}

// firstLink names, relative to root, the first symbolic link on the way from
// root down to abs, or "" when there is none.
func firstLink(root, abs string) string {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return ""
	}
	rel, err := filepath.Rel(absRoot, abs)
	if err != nil || !filepath.IsLocal(rel) {
		return ""
	}
	cur := absRoot
	for _, name := range splitPath(rel) {
		cur = filepath.Join(cur, name)
		info, err := os.Lstat(cur)
		if err != nil {
			return ""
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return relativeTo(root, cur)
		}
	}
	return ""
}

func splitPath(rel string) []string {
	var parts []string
	for rel != "." && rel != "" {
		dir, base := filepath.Split(rel)
		parts = append(parts, base)
		rel = filepath.Clean(dir)
		if dir == "" {
			break
		}
	}
	slices.Reverse(parts)
	return parts
}
