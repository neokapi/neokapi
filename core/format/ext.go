package format

import (
	"fmt"
	"path/filepath"
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
	lower := strings.ToLower(ext)
	for _, e := range compoundExts {
		if lower == e {
			return true
		}
	}
	return false
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

// retiredExts maps an extension this project used to emit to an explanation of
// what replaced it.
//
// These renames were deliberate hard cuts with no compatibility shim, so a
// stale file is simply not recognised. That is only defensible if the failure
// explains itself: "unknown format .kmb" tells a user nothing, while naming the
// convention that replaced it and the command that rewrites the file turns a
// dead end into a one-line fix.
var retiredExts = map[string]retiredExt{
	".klf": {replacement: KBFExt, regen: "kapi extract"},
	".kbf": {replacement: KBFExt, regen: "kapi extract"},
	".klz": {replacement: ".kpz", regen: "kapi workspace pack"},

	".klftm": {replacement: MemoryBundleExt, regen: "kapi memory export"},
	".kmb":   {replacement: MemoryBundleExt, regen: "kapi memory export"},
	".klftb": {replacement: TermsBundleExt, regen: "kapi terms export"},
	".ktb":   {replacement: TermsBundleExt, regen: "kapi terms export"},

	".klfo": {replacement: OverlaySetExt, regen: "kapi extract"},
	".klfl": {replacement: AnnotationExt, regen: "kapi extract"},

	// The compressed archive tier was withdrawn rather than renamed: .kpz
	// already packages project state, and zipping a single bundle is what a zip
	// tool is for.
	".kmz": {replacement: MemoryBundleExt, regen: "kapi memory export", withdrawn: true},
	".ktz": {replacement: TermsBundleExt, regen: "kapi terms export", withdrawn: true},
}

type retiredExt struct {
	replacement string
	regen       string
	// withdrawn marks a tier that was removed outright, not renamed, so the
	// message does not imply a like-for-like substitution.
	withdrawn bool
}

// RetiredExtHint returns an explanation for a path carrying a retired kapi
// extension, or "" when the path carries none.
//
// Callers append it to whatever "unrecognised format" error they already
// produce, so the diagnosis stays with the operation that failed.
func RetiredExtHint(path string) string {
	info, ok := retiredExts[strings.ToLower(filepath.Ext(path))]
	if !ok {
		return ""
	}
	ext := strings.ToLower(filepath.Ext(path))
	if info.withdrawn {
		return fmt.Sprintf(
			"%s is no longer read: the compressed archive tier was withdrawn — "+
				"unzip it, or regenerate the bundle with %q, which writes %s",
			ext, info.regen, info.replacement)
	}
	return fmt.Sprintf(
		"%s was renamed to %s — this file was written by an earlier release and "+
			"is not read as-is; regenerate it with %q",
		ext, info.replacement, info.regen)
}

// ErrorWithRetiredExtHint wraps err with [RetiredExtHint] when path carries a
// retired extension, and returns err unchanged otherwise.
func ErrorWithRetiredExtHint(err error, path string) error {
	hint := RetiredExtHint(path)
	if err == nil || hint == "" {
		return err
	}
	return fmt.Errorf("%w (%s)", err, hint)
}
