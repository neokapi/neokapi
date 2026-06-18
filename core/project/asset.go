package project

import (
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/core/model"
)

// IsBinaryAssetFormat reports whether a format name denotes a binary asset — one
// whose file *is* the localizable unit, localized by supplying a per-locale
// variant file, rather than text kapi can regenerate from a source. For these
// formats an existing localized target variant is authoritative: kapi cannot
// reproduce a real image localization, so reprocessing the source must not
// clobber a hand- (or connector-) supplied variant.
func IsBinaryAssetFormat(name string) bool {
	switch name {
	case "image":
		return true
	default:
		return false
	}
}

// AssetVariant pairs a target locale with the output path a localized binary
// asset resolves to and whether that file already exists on disk. It is the
// local counterpart of the server-side asset-variant model (Bowrain AD-007): the
// "connector" that resolves the right per-locale file for a source asset, here
// against the working tree using the recipe's target template.
type AssetVariant struct {
	Locale model.LocaleID
	Path   string // resolved output path (absolute when root is absolute)
	Exists bool
}

// ResolveAssetVariants pairs a source asset (path relative to root) with its
// per-locale target files for the given content item: each locale's target
// template is resolved via ResolveTargetPath and checked for existence on disk.
// root anchors any relative target path. The result enumerates which locales
// have a localized variant present and which are missing — the coverage view a
// whole-image-replacement workflow needs.
func ResolveAssetVariants(root string, item ContentItem, source string, locales []model.LocaleID) []AssetVariant {
	out := make([]AssetVariant, 0, len(locales))
	for _, loc := range locales {
		p := ResolveTargetPath(item.Path, item.Base, item.Target, source, string(loc))
		if !filepath.IsAbs(p) {
			p = filepath.Join(root, p)
		}
		exists := false
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			exists = true
		}
		out = append(out, AssetVariant{Locale: loc, Path: p, Exists: exists})
	}
	return out
}
