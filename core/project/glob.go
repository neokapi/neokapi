package project

import (
	"os"

	"github.com/bmatcuk/doublestar/v4"
)

// MatchGlob reports whether path matches the doublestar glob pattern (same
// semantics as ExpandGlob: `**` recursive, `{a,b}` alternation). Both should be
// slash-separated, relative to the same root.
func MatchGlob(pattern, path string) bool {
	ok, err := doublestar.Match(pattern, path)
	return err == nil && ok
}

// ExpandGlob returns relative paths under root that match the given glob
// pattern. Supports `**` for recursive directory matching via doublestar.
// Any matches matching one of the exclude patterns are filtered out.
//
// `**` does not follow a symbolic link into another directory, as git ls-files
// does, so the linked packages a package manager keeps under node_modules stay
// out of a match and a link loop is read once. A link that matches the pattern
// itself is returned, and a linked directory the pattern names before its first
// wildcard is read through.
func ExpandGlob(root, pattern string, excludes ...string) ([]string, error) {
	fsys := os.DirFS(root)
	matches, err := doublestar.Glob(fsys, pattern, doublestar.WithNoFollow())
	if err != nil {
		return nil, err
	}
	if len(excludes) == 0 {
		return matches, nil
	}
	filtered := matches[:0]
	for _, m := range matches {
		excluded := false
		for _, exc := range excludes {
			if ok, _ := doublestar.Match(exc, m); ok {
				excluded = true
				break
			}
		}
		if !excluded {
			filtered = append(filtered, m)
		}
	}
	return filtered, nil
}
