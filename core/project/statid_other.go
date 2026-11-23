//go:build !unix && !windows

package project

import (
	"fmt"
	"os"
	"path/filepath"
)

// dirIdentity is the cleaned absolute path where the platform gives a
// directory no identity of its own (the browser build, whose state lives in
// one in-memory file system on one machine).
func dirIdentity(path string) (string, error) {
	return filepath.Abs(path)
}

// keyPrivate refuses a key that is not a regular file. The browser's file
// system has one user.
func keyPrivate(_, path string, _, info os.FileInfo) error {
	if !info.Mode().IsRegular() {
		return fmt.Errorf("project: the state key %s must be a regular file; remove it and vouch for each checkout's state again", path)
	}
	return nil
}
