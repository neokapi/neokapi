package safeio

import (
	"os"
	"path/filepath"
)

// SafeJoin joins a content-derived relative path onto root, rejecting any name
// that is absolute or escapes root via "..". It is the guard for the zip-slip /
// arbitrary-file-write class: archive entry names, data: URI paths, and any
// other path taken from document content must pass through here before they
// reach the filesystem.
//
// name is interpreted with forward-slash separators (the zip / URI convention)
// and converted to the OS separator internally, then validated with
// filepath.IsLocal (Go 1.20+), which rejects absolute paths, paths that escape
// the root via "..", the empty path, and — on Windows — reserved device names.
//
// IMPORTANT (the pandoc CVE-2023-35936 lesson): if name arrives percent- or
// otherwise-encoded, the caller MUST decode it *before* calling SafeJoin.
// Validating an encoded path and then decoding it for use re-opens the
// traversal hole.
func SafeJoin(root, name string) (string, error) {
	local := filepath.FromSlash(name)
	if !filepath.IsLocal(local) {
		return "", newLimitError(ErrPathEscape, 0, 0, name)
	}
	return filepath.Join(root, local), nil
}

// IsLocalPath reports whether a forward-slash content-derived path is local to
// its root — i.e. relative and non-escaping. It is the boolean form of the
// check [SafeJoin] performs, for callers that want to validate without joining.
func IsLocalPath(name string) bool {
	return filepath.IsLocal(filepath.FromSlash(name))
}

// OpenInRoot opens a content-derived path strictly within dir, using os.Root
// (Go 1.24+) confinement so the open cannot traverse outside dir even via
// symlinks evaluated by the OS. It is the strongest guard for extraction
// features that read or write files at paths taken from document content.
//
// name is forward-slash separated (zip / URI convention). The returned file
// must be closed by the caller. Note: on the WASM/wasip1 build there is no
// usable filesystem, so this returns an error at runtime there — it exists for
// the CLI and server extraction paths; pure path validation that works
// everywhere is [SafeJoin] / [IsLocalPath].
func OpenInRoot(dir, name string) (*os.File, error) {
	if !IsLocalPath(name) {
		return nil, newLimitError(ErrPathEscape, 0, 0, name)
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return root.Open(filepath.FromSlash(name))
}
