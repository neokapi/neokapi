package dataroot

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// ensurePrivateDir creates dir with mode 0700, or accepts it when it already
// exists as a directory this user owns that nobody else can reach. Anything
// else is an error: a link, a file, a directory owned by another user, or one
// with group or other access.
//
// A test binary's root sits under the system temporary directory at a name
// anyone can predict, and on a shared temporary directory another user could
// create it first, as a directory of theirs or as a link to one. The check is
// what keeps a test from then reading and writing where that user chose.
func ensurePrivateDir(dir string) error {
	if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
		return err
	}
	return checkPrivateDir(dir)
}

// checkPrivateDir reports why dir is unfit to hold a test binary's files, or
// nil when it is a real directory owned by this user with no group or other
// access.
func checkPrivateDir(dir string) error {
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if err := checkOwnedDir(dir, info); err != nil {
		return err
	}
	if privateModeApplies && info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%s: mode %v gives group or other users access", dir, info.Mode().Perm())
	}
	return nil
}

// checkOwnedDir reports why dir, described by its Lstat result, is not a real
// directory owned by this user. The sweep removes only what passes it, so it
// cannot be made to delete through a link or inside another user's directory.
func checkOwnedDir(dir string, info fs.FileInfo) error {
	if info.Mode()&fs.ModeSymlink != 0 {
		return fmt.Errorf("%s: is a symbolic link", dir)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s: is not a directory", dir)
	}
	if !ownedByMe(info) {
		return fmt.Errorf("%s: is owned by another user", dir)
	}
	return nil
}
