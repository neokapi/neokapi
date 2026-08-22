package ktb

import (
	"path/filepath"
	"strings"

	"github.com/neokapi/neokapi/core/format"
)

// Ext is the terms bundle suffix.
//
// It is a compound suffix: the ".terms" segment says what the document is,
// while the ".json" tail keeps it a plain JSON file for `jq`, editors, syntax
// highlighting, and line-by-line review in a diff — which is what makes a
// committed, reviewed terms reviewable at all. Because it is compound,
// path/filepath.Ext reports only ".json" for one; use [format.Ext] or
// [IsBundlePath].
const Ext = format.TermsBundleExt

// ConventionalName is the bundle's name when a project keeps its committed
// terms at a well-known location instead of binding one through the recipe's
// defaults.terms_source. It is also the member name a .kpz carries terms
// under, so unpacking an archive by hand yields the conventional spelling.
const ConventionalName = "terms.json"

// IsBundlePath reports whether path names a terms bundle: either the
// conventional bare name or any file carrying [Ext].
func IsBundlePath(path string) bool {
	if strings.EqualFold(filepath.Base(path), ConventionalName) {
		return true
	}
	return format.HasExt(path, Ext)
}
