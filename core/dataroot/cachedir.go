package dataroot

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// PrepareCacheDir holds a cache directory named in the environment variable
// env to the standard of kapi's own directories, creating it (0700) when it
// is missing:
//
//   - it is an absolute path; a relative one is refused rather than read
//     against the working directory;
//   - it is a real directory, not a link, owned by this user with no group
//     or other access (checkPrivateDir, the test root's check); the cache
//     opens each file beneath it with no link followed. On Windows access is
//     an ACL, which the check does not read;
//   - it lies outside the checkout, ignored or not: a cache inside one can be
//     committed by accident;
//   - in a test binary it lies beneath the temporary directory, so a test
//     cannot write outside its own tree.
func PrepareCacheDir(env, dir, checkout string) (string, error) {
	if !filepath.IsAbs(dir) {
		return "", fmt.Errorf("%s must be an absolute path; %q is relative", env, dir)
	}
	dir = filepath.Clean(dir)
	// The location is resolved through its parent, so a link at the directory
	// itself is seen by checkPrivateDir below rather than followed here.
	resolved := filepath.Join(NormalizePath(filepath.Dir(dir)), filepath.Base(dir))
	if UnderTest() && !within(NormalizePath(os.TempDir()), resolved) {
		return "", fmt.Errorf("%s names %s, outside the temporary directory a test binary writes in", env, dir)
	}
	if checkout != "" && within(NormalizePath(checkout), resolved) {
		return "", fmt.Errorf("%s names %s, inside the checkout %s; name a directory outside the checkout", env, dir, checkout)
	}
	if _, err := os.Lstat(dir); errors.Is(err, fs.ErrNotExist) {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return "", fmt.Errorf("create %s %s: %w", env, dir, err)
		}
	}
	// The directory meets the test root's standard: a real directory this
	// user owns, with no group or other access. One someone else prepared is
	// refused, not repaired.
	if err := checkPrivateDir(dir); err != nil {
		return "", fmt.Errorf("%s is refused: %w", env, err)
	}
	return dir, nil
}

// within reports whether path is dir or lies beneath it.
func within(dir, path string) bool {
	rel, err := filepath.Rel(dir, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel))
}
