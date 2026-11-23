//go:build unix

package project

import (
	"fmt"
	"os"
	"syscall"
)

// dirIdentity is the device and inode of the directory at path: the same for
// every spelling of one directory and for a directory renamed on its volume,
// different for a copy.
func dirIdentity(path string) (string, error) {
	info, err := os.Stat(path) //pathguard: the project root or its state directory, which CheckState has walked; only the identity is read
	if err != nil {
		return "", err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", fmt.Errorf("project: no file identity for %s", path)
	}
	return fmt.Sprintf("%d:%d", st.Dev, st.Ino), nil
}
