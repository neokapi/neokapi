package rootpath

import (
	"fmt"
	"os"
	"path/filepath"
)

// OpenRegularNoLink opens p for reading when it is a regular file inside root
// reached through no symbolic link at all, not even one that stays inside
// root. It suits a directory kapi writes itself, a cache, where a link is
// something another party put there. A link swapped in after the check is
// refused too: what opened must be the file p names.
func OpenRegularNoLink(root, p string) (*os.File, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return nil, err
	}
	if l := firstLink(root, abs); l != "" {
		return nil, fmt.Errorf("%s is a symbolic link", l)
	}
	f, err := OpenRegular(root, abs)
	if err != nil {
		return nil, err
	}
	named, lerr := os.Lstat(abs)
	opened, serr := f.Stat()
	if lerr != nil || serr != nil || !os.SameFile(named, opened) {
		_ = f.Close()
		return nil, fmt.Errorf("%s changed while it was opened", relativeTo(root, abs))
	}
	return f, nil
}
