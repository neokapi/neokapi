package format

import (
	"path/filepath"
	"slices"
	"strings"
)

// Compound suffixes used by kapi's own on-disk conventions.
//
// Every one of these files is JSON or JSON Lines, and each keeps a marker
// segment ahead of the serialization suffix so the file still says what it is
// while `jq`, GitHub, editors, and syntax highlighting all still see JSON. The
// cost is that [path/filepath.Ext] no longer identifies them — it reports
// ".json" for every one — so extension-driven code must go through [Ext],
// [TrimExt], or [Stem] rather than filepath.Ext.
const (
	// KBFExt is the Kapi Bundle Format document suffix.
	KBFExt = ".kbf.json"
	// OverlaySetExt is the JSON overlay-set sidecar suffix.
	OverlaySetExt = ".overlays.json"
	// AnnotationExt is the JSON Lines stand-off annotation overlay sidecar suffix.
	AnnotationExt = ".overlays.jsonl"
	// MemoryBundleExt is the content-memory bundle suffix (see memory/kmb).
	MemoryBundleExt = ".memory.json"
	// TermsBundleExt is the terms bundle suffix (see terms/ktb).
	TermsBundleExt = ".terms.json"
)

// compoundExts is the canonical set, longest first so [Ext] returns the most
// specific match. Order matters only for suffixes that are suffixes of each
// other; keeping the slice length-ordered makes that impossible to get wrong
// when a new one is added.
var compoundExts = []string{
	AnnotationExt,
	OverlaySetExt,
	MemoryBundleExt,
	TermsBundleExt,
	KBFExt,
}

// Ext returns the format-significant extension of path.
//
// It behaves like [path/filepath.Ext] except that a kapi compound suffix wins
// over the bare serialization suffix: "en-US.kbf.json" reports ".kbf.json",
// not ".json". Comparison is case-insensitive and the result is lowercased,
// matching how the format registry keys its extension table.
func Ext(path string) string {
	lower := strings.ToLower(path)
	for _, ext := range compoundExts {
		if strings.HasSuffix(lower, ext) {
			return ext
		}
	}
	return strings.ToLower(filepath.Ext(path))
}

// IsCompoundExt reports whether ext is one of the compound suffixes.
func IsCompoundExt(ext string) bool {
	return slices.Contains(compoundExts, strings.ToLower(ext))
}

// TrimExt returns path with its format-significant extension removed, so
// "i18n/en-US.kbf.json" becomes "i18n/en-US". Deriving an output name with
// [path/filepath.Ext] instead would leave a stray ".kbf" behind.
func TrimExt(path string) string {
	return path[:len(path)-len(Ext(path))]
}

// Stem returns the base name of path without its format-significant
// extension: "i18n/en-US.kbf.json" becomes "en-US".
func Stem(path string) string {
	return TrimExt(filepath.Base(path))
}

// HasExt reports whether path carries ext, comparing case-insensitively
// against the format-significant extension rather than a raw suffix. It
// distinguishes a compound suffix from the bare one it ends with, so a plain
// "data.json" does not match ".kbf.json" and a "en.kbf.json" does not match
// ".json".
func HasExt(path, ext string) bool {
	return Ext(path) == strings.ToLower(ext)
}
