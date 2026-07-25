package kmb

import (
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/format"
)

// Ext is the content-memory bundle suffix.
//
// It is a compound suffix: the ".memory" segment says what the document is,
// while the ".json" tail keeps it a plain JSON file for `jq`, editors, syntax
// highlighting, and line-by-line review in a diff — which is the whole point of
// committing a bundle rather than a database. Because it is compound,
// path/filepath.Ext reports only ".json" for one; use [format.Ext] or
// [IsBundlePath].
const Ext = format.MemoryBundleExt

// ConventionalName is the bundle's name when a project keeps a single one at a
// well-known location rather than naming it per content surface.
const ConventionalName = "memory.json"

// IsBundlePath reports whether path names a content-memory bundle: either the
// conventional bare name or any file carrying [Ext].
//
// A project is not limited to one bundle — this repository's own recipe commits
// a dozen, one per content surface — so the suffix, not the location, is what
// identifies a bundle.
func IsBundlePath(path string) bool {
	if strings.EqualFold(filepath.Base(path), ConventionalName) {
		return true
	}
	return format.HasExt(path, Ext)
}
