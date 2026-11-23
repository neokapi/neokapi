//go:build unix

package project

import (
	"fmt"
	"os"
	"syscall"
)

// dirIdentity is the device, inode and birth time of the file or directory at
// path: the same for every spelling of it and for it renamed on its volume,
// different for a copy and for one removed and created again.
func dirIdentity(path string) (string, error) {
	info, err := os.Stat(path) //pathguard: the project root or its state directory, which CheckState has walked; only the identity is read
	if err != nil {
		return "", err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", fmt.Errorf("project: no file identity for %s", path)
	}
	return fmt.Sprintf("%d:%d:%s", st.Dev, st.Ino, birthTimeOf(path, st)), nil
}

// birthTimeOf is when the file at path was created, where the platform and the
// file system say so, and "" elsewhere; the device and inode alone identify a
// file there. statid_birth_linux.go and statid_birth_darwin.go set it.
var birthTimeOf = func(string, *syscall.Stat_t) string { return "" }

// keyPrivate refuses a key that another user could read or replace: the key
// must be a regular file of mode 0600 owned by this user, in a directory owned
// by this user that no one else can write. dinfo and info are the Lstat of the
// directory and of the key.
func keyPrivate(dir, path string, dinfo, info os.FileInfo) error {
	uid := uint32(os.Getuid())
	dst, ok := dinfo.Sys().(*syscall.Stat_t)
	if !dinfo.IsDir() || !ok || dst.Uid != uid || dinfo.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("project: the state key directory %s must be a directory you own that only you can write; fix it with `chmod 700 %s`, or remove it and vouch for each checkout's state again", dir, dir)
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !info.Mode().IsRegular() || !ok || st.Uid != uid || info.Mode().Perm() != 0o600 {
		return fmt.Errorf("project: the state key %s must be a regular file you own with mode 0600; fix it with `chmod 600 %s`, or remove it and vouch for each checkout's state again", path, path)
	}
	return nil
}
