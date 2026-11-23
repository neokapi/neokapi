//go:build !unix && !windows

package dataroot

import "os"

// identity is constant where the platform gives a directory no identity of its
// own (the browser build, whose files live in one file system on one machine):
// the resolved path alone keys a checkout there. A directory that does not
// exist has no identity.
func identity(path string) (string, error) {
	if _, err := os.Stat(path); err != nil {
		return "", err
	}
	return "path", nil
}

// followable reports whether a rename may be followed by identity: never here,
// where the identity says nothing about the directory.
func followable(string) bool { return false }
