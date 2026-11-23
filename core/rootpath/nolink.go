package rootpath

import (
	"fmt"
	"os"
	"path/filepath"
)

// OpenRegularNoLink opens p for reading when it is a regular file inside root
// reached through no symbolic link at all, not even one that stays inside
// root. It suits a directory kapi writes itself, a cache, where a link is
// something another party put there.
//
// The last component is opened with O_NOFOLLOW where the platform has it, so
// a link swapped in after the check is refused by the open itself. A regular
// file renamed over the name in the meantime (a concurrent writer replacing
// a content-addressed file with identical bytes) is opened as what it now is.
// On Windows, where os.OpenFile takes no flag that refuses a reparse point, a
// link swapped in for the last component between the check and the open is
// followed; the open still cannot leave root.
func OpenRegularNoLink(root, p string) (*os.File, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return nil, err
	}
	if l := firstLink(root, abs); l != "" {
		return nil, fmt.Errorf("%s is a symbolic link", l)
	}
	real, err := Contain(root, abs)
	if err != nil {
		return nil, err
	}
	r, rel, err := openRoot(root, real)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	f, err := r.OpenFile(rel, os.O_RDONLY|openNonBlock|openNoFollow, 0)
	if err != nil {
		return nil, explain(root, abs, err)
	}
	info, err := f.Stat()
	if err == nil && !info.Mode().IsRegular() {
		err = fmt.Errorf("%s: %w", relativeTo(root, abs), ErrNotRegular)
	}
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}
