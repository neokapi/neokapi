//go:build unix

package dataroot

import (
	"fmt"
	"os"
	"strings"
	"syscall"
)

// identity is the device, inode and birth time of the directory at path: the
// same for every spelling of it and for it renamed on its volume, different
// for another directory, a copy, and one removed and created again in its
// place.
func identity(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", fmt.Errorf("dataroot: no file identity for %s", path)
	}
	return fmt.Sprintf("%d:%d:%s", st.Dev, st.Ino, birthTimeOf(path, st)), nil
}

// birthTimeOf is when the file at path was created, where the platform and the
// file system say so, and "" elsewhere; the device and inode alone identify a
// directory there. birth_linux.go and birth_darwin.go set it.
var birthTimeOf = func(string, *syscall.Stat_t) string { return "" }

// followable reports whether a rename may be followed by identity: only when
// the identity carries a birth time, so a new directory that reuses a deleted
// one's inode is not taken for it.
func followable(id string) bool { return !strings.HasSuffix(id, ":") }
