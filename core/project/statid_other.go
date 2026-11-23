//go:build !unix && !windows

package project

import "path/filepath"

// dirIdentity is the cleaned absolute path where the platform gives a
// directory no identity of its own (the browser build, whose state lives in
// one in-memory file system on one machine).
func dirIdentity(path string) (string, error) {
	return filepath.Abs(path)
}
