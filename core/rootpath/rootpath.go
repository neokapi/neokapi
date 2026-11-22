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
// OpenRegular then opens through os.Root, which refuses a link that leads
// out of the root at the moment of the open, so a link swapped in after
// Contain is refused too. A write goes through atomicfile.StageWithin, which
// stages and renames through the same kind of root and never cuts a file in
// place.
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
// that is the link, and is empty when no link was found on the way. The
// error never names where the link points.
//
// Err is fs.ErrNotExist for a link whose file does not exist, which is refused
// because where it leads cannot be decided; it is nil otherwise.
type EscapeError struct {
	Path string
	Link string
	Err  error
}

func (e *EscapeError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: the symbolic link %s names a file that does not exist", e.Path, e.linkName())
	}
	if e.Link == "" {
		// No link was found on the way: the path itself lies outside.
		return e.Path + " lies outside the root"
	}
	if e.Link == e.Path {
		return e.Path + " is a symbolic link that leads outside the root"
	}
	return fmt.Sprintf("%s: the symbolic link %s leads outside the root", e.Path, e.Link)
}

// Unwrap returns Err.
func (e *EscapeError) Unwrap() error { return e.Err }

func (e *EscapeError) linkName() string {
	if e.Link == "" {
		return e.Path
	}
	return e.Link
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
	if !Supported {
		return containLexically(root, p)
	}
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
			if _, ok := Beneath(realRoot, real); !ok {
				return "", &EscapeError{Path: rel, Link: firstLink(root, abs)}
			}
			return resolved, nil
		}
		if !errors.Is(rerr, fs.ErrNotExist) {
			return "", rerr
		}
		if info, lerr := os.Lstat(cur); lerr == nil && info.Mode()&fs.ModeSymlink != 0 {
			// A link whose target is missing, at cur or below it.
			return "", &EscapeError{Path: rel, Link: firstLink(root, abs), Err: fs.ErrNotExist}
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
	if !Supported {
		// Contain was lexical, so real may still be a link (Node's file system,
		// which the js/wasm tests run over, holds them). Resolve it and judge
		// where it leads, as Contain does where links are resolved.
		if real, err = resolveWithin(root, p, real); err != nil {
			return "", err
		}
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

// resolveWithin resolves every link on real and returns the result when it
// lies under root, resolved the same way, and an *EscapeError naming p when it
// does not. It stands in for Contain's resolution where Contain is lexical.
func resolveWithin(root, p, real string) (string, error) {
	resolved, err := filepath.EvalSymlinks(real)
	if err != nil {
		return "", err
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(realRoot, resolved)
	if err != nil || !filepath.IsLocal(rel) {
		name := relativeTo(root, p)
		return "", &EscapeError{Path: name, Link: name}
	}
	return resolved, nil
}

// OpenRegular opens p for reading when it is a regular file inside root. The
// open goes through os.Root, which follows a symbolic link only while it stays
// inside root, so a link swapped in after Contain is refused rather than read.
// A FIFO is opened without blocking and refused.
func OpenRegular(root, p string) (*os.File, error) {
	real, err := Contain(root, p)
	if err != nil {
		return nil, err
	}
	r, rel, err := openRoot(root, real)
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

// Remove removes the name p beneath root through os.Root, so a link that
// leads out of root, even one put in place after a check, is refused. The
// directory that holds p is resolved; p itself is removed as the name it is,
// so a link is removed and the file it points at is left alone.
func Remove(root, p string) error {
	dir, err := Contain(root, filepath.Dir(p))
	if err != nil {
		return err
	}
	r, rel, err := openRoot(root, filepath.Join(dir, filepath.Base(p)))
	if err != nil {
		return err
	}
	defer r.Close()
	if err := r.Remove(rel); err != nil {
		return explain(root, p, err)
	}
	return nil
}

// openRoot opens the directory root resolves to and returns real, the path
// Contain resolved, relative to it. The name handed to the root holds no
// link, so a link Contain followed (an absolute one inside the root
// included) is not refused by the root, while one swapped in afterwards is.
func openRoot(root, real string) (*os.Root, string, error) {
	realRoot, err := realDir(root)
	if err != nil {
		return nil, "", err
	}
	rel, ok := Beneath(realRoot, real)
	if !ok || rel == "." {
		return nil, "", &EscapeError{Path: filepath.Base(real)}
	}
	r, err := os.OpenRoot(realRoot)
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

// Beneath returns real relative to realRoot when real is realRoot or lies
// beneath it. Both are resolved paths, free of links. The spelling decides
// first; failing that, an ancestor of real that is the same directory as
// realRoot does, so a path spelled in another case on a case-insensitive
// volume is beneath the root it names. The relative path keeps real's
// spelling.
func Beneath(realRoot, real string) (string, bool) {
	if rel, err := filepath.Rel(realRoot, real); err == nil && (rel == "." || filepath.IsLocal(rel)) {
		return rel, true
	}
	if !Supported {
		// Links were not resolved on the way here (containLexically), so the
		// two may be spelled through different links to one directory.
		rr, rerr := resolveExisting(realRoot)
		ra, aerr := resolveExisting(real)
		rel, err := filepath.Rel(rr, ra)
		if rerr == nil && aerr == nil && err == nil && (rel == "." || filepath.IsLocal(rel)) {
			return rel, true
		}
		return "", false
	}
	rootInfo, err := os.Stat(realRoot)
	if err != nil {
		return "", false
	}
	for dir := real; ; dir = filepath.Dir(dir) {
		if info, serr := os.Stat(dir); serr == nil && os.SameFile(info, rootInfo) {
			rel, rerr := filepath.Rel(dir, real)
			return rel, rerr == nil
		}
		if parent := filepath.Dir(dir); parent == dir {
			return "", false
		}
	}
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
		if rel, ok := Beneath(absRoot, abs); ok && rel != "." {
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
	rel, ok := Beneath(absRoot, abs)
	if !ok || rel == "." {
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

// containLexically is Contain where the platform gives no os.Root (Supported
// is false, the js/wasm build). Where its file system resolves links (Node's,
// which the js/wasm tests run over), it judges p as Contain does: the links on
// the longest existing prefix of p are resolved, a link to nothing is refused,
// and the result must lie beneath root with its links resolved. Comparing the
// resolved spellings also makes a root and a path named through different
// links to one directory (a temporary directory under /var, which is
// /private/var on macOS) one place. Where the file system resolves nothing
// (the browser's), p, made absolute, must lie beneath root as written.
func containLexically(root, p string) (string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	realRoot, rerr := filepath.EvalSymlinks(absRoot)
	if rerr != nil {
		if !inside(absRoot, abs) {
			return "", &EscapeError{Path: relativeTo(root, p)}
		}
		return abs, nil
	}
	resolved, err := resolveExisting(abs)
	if err != nil {
		name := relativeTo(root, p)
		return "", &EscapeError{Path: name, Link: name, Err: err}
	}
	if !inside(realRoot, resolved) {
		name := relativeTo(root, p)
		if inside(absRoot, abs) {
			return "", &EscapeError{Path: name, Link: name}
		}
		return "", &EscapeError{Path: name}
	}
	return resolved, nil
}

// resolveExisting resolves the links on the longest existing prefix of abs and
// appends the names that do not exist yet. A name on the way that exists and
// cannot be resolved is a link to nothing, and its error is returned.
func resolveExisting(abs string) (string, error) {
	var missing []string
	for cur := abs; ; cur = filepath.Dir(cur) {
		real, err := filepath.EvalSymlinks(cur)
		if err == nil {
			slices.Reverse(missing)
			return filepath.Join(append([]string{real}, missing...)...), nil
		}
		if _, lerr := os.Lstat(cur); lerr == nil {
			return "", err
		}
		if filepath.Dir(cur) == cur {
			return abs, nil
		}
		missing = append(missing, filepath.Base(cur))
	}
}
