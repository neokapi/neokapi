package host

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/safeio"
)

// A `.kpz` exists to cross a trust boundary: it is packed, handed to an outside
// translator or reviewer, and returned to be merged. Every path and identifier
// it carries is therefore content, not configuration, and must be contained
// before it reaches the filesystem. The helpers below are that containment, and
// they are the ONLY idiom for it in this package — core/safeio owns the actual
// rule so unpack, merge, and the workspace flow cannot drift apart on what
// "inside the project" means.

// containedJoin resolves a content-derived relative path against root, refusing
// any name that is absolute or climbs out of root via "..". `what` names the
// kind of value for the error ("unpack: source member", "merge: package source
// path", …).
//
// Refusal, not repair: a package that asks to read or write outside the project
// is not one to half-apply.
func containedJoin(root, name, what string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("%s has no usable relative path", what)
	}
	joined, err := safeio.SafeJoin(root, filepath.ToSlash(name))
	if err != nil {
		return "", fmt.Errorf("%s %q escapes the project root", what, name)
	}
	return joined, nil
}

// checkPathSegment refuses a content-derived identifier that is not usable as a
// single path segment. A locale and a project name are identifiers, not paths:
// they are interpolated into an output path as one component, so a value
// carrying a separator or ".." would silently redirect the write.
func checkPathSegment(value, what string) error {
	if value == "" || value == "." || value == ".." ||
		strings.ContainsAny(value, `/\`) || !safeio.IsLocalPath(value) {
		return fmt.Errorf("%s %q is not usable as a path segment", what, value)
	}
	return nil
}
