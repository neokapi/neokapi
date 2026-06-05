// Flow I/O binding (AD-026).
//
// A flow is a pure transformation over a stream of Parts; where content enters
// (the source binding) and where results go (the sink binding) are resolved from
// invocation context, not baked into the flow graph. A binding is named by a small
// scheme vocabulary shared with the resource URIs the tool resolver understands
// (tm:, termbase:, srx: in resolve.go):
//
//	file       a DataFormat reader/writer over file bytes (the default)
//	store/klz  a persistent block store — the project store or a .klz workspace
//	xliff/po/tmx/tbx   an interchange file (bilingual round-trip or asset exchange)
//	none       discard (sink only — analysis / checks)
//
// A bare value is bound by detection (its extension or kind); a "scheme:" prefix
// forces the binding and removes ambiguity (`-o store:` is the block store, while
// `-o l10n/` is a directory of files). `file:` forces a path that would otherwise
// read as a scheme.
package flow

import (
	"fmt"
	"path/filepath"
	"strings"
)

// BindingKind identifies where content enters or leaves a flow.
type BindingKind string

const (
	// BindingFile reads/writes file bytes through a DataFormat reader/writer.
	BindingFile BindingKind = "file"
	// BindingStore reads/commits blocks + overlays in a persistent block store
	// (the project store or a .klz workspace).
	BindingStore BindingKind = "store"
	// BindingInterchange imports from / exports to an interchange file
	// (bilingual .klz, XLIFF, PO, TMX, TBX).
	BindingInterchange BindingKind = "interchange"
	// BindingNone discards output (a sink that materializes nothing).
	BindingNone BindingKind = "none"
)

// Binding schemes recognized as the prefix of a locator (the part before ":").
const (
	SchemeFile  = "file"
	SchemeStore = "store"
	SchemeKLZ   = "klz"
	SchemeXLIFF = "xliff"
	SchemePO    = "po"
	SchemeTMX   = "tmx"
	SchemeTBX   = "tbx"
	SchemeNone  = "none"
)

// knownSchemes are the binding schemes ParseLocator recognizes. Any other
// "prefix:" is treated as part of a path (e.g. a Windows drive letter), so a
// bare path never accidentally parses as a scheme.
var knownSchemes = map[string]bool{
	SchemeFile:  true,
	SchemeStore: true,
	SchemeKLZ:   true,
	SchemeXLIFF: true,
	SchemePO:    true,
	SchemeTMX:   true,
	SchemeTBX:   true,
	SchemeNone:  true,
}

// interchangeExts are file extensions detected as interchange when no scheme is
// given (bilingual and asset exchange formats).
var interchangeExts = map[string]bool{
	".xliff": true,
	".xlf":   true,
	".po":    true,
	".pot":   true,
	".tmx":   true,
	".tbx":   true,
}

// Locator is a parsed -i/-o value or source:/sink: field: an optional scheme plus
// the remaining path. A bare value (no recognized scheme) has Scheme == "".
type Locator struct {
	Scheme string // one of the Scheme* constants, or "" for a bare path
	Path   string // the path after "scheme:", or the whole value when bare
	Raw    string // the original input, for diagnostics
}

// ParseLocator splits a locator into its scheme and path. A recognized
// "scheme:" prefix is honored (the CLI locator form, e.g. "store:work.klz"); a
// bare known-scheme word with no path is that binding kind (the flow-intent
// form, e.g. "store", "none", "xliff"); anything else is a bare path that is
// bound by detection.
func ParseLocator(s string) Locator {
	raw := s
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, ':'); i > 0 {
		scheme := strings.ToLower(s[:i])
		if knownSchemes[scheme] {
			return Locator{Scheme: scheme, Path: s[i+1:], Raw: raw}
		}
	}
	if knownSchemes[strings.ToLower(s)] {
		return Locator{Scheme: strings.ToLower(s), Raw: raw}
	}
	return Locator{Path: s, Raw: raw}
}

// Kind classifies the locator into a binding kind. An explicit scheme wins;
// otherwise the path's extension decides (.klz → store, an interchange extension
// → interchange, anything else → file).
func (l Locator) Kind() BindingKind {
	switch l.Scheme {
	case SchemeNone:
		return BindingNone
	case SchemeStore, SchemeKLZ:
		return BindingStore
	case SchemeXLIFF, SchemePO, SchemeTMX, SchemeTBX:
		return BindingInterchange
	case SchemeFile:
		return BindingFile
	}
	switch ext := strings.ToLower(filepath.Ext(l.Path)); {
	case ext == ".klz":
		return BindingStore
	case interchangeExts[ext]:
		return BindingInterchange
	default:
		return BindingFile
	}
}

// Format returns the explicit interchange/file format implied by a format scheme
// (xliff/po/tmx/tbx), or "" when the format should be auto-detected from the path.
func (l Locator) Format() string {
	switch l.Scheme {
	case SchemeXLIFF:
		return "xliff"
	case SchemePO:
		return "po"
	case SchemeTMX:
		return "tmx"
	case SchemeTBX:
		return "tbx"
	default:
		return ""
	}
}

// Explain renders the resolved binding for `kapi run --explain`, e.g.
// "file(a.json)", "store(work.klz)", "interchange(hand.xliff)", or "none".
func (l Locator) Explain() string {
	k := l.Kind()
	if k == BindingNone {
		return string(BindingNone)
	}
	if l.Path == "" {
		return string(k)
	}
	return fmt.Sprintf("%s(%s)", k, l.Path)
}
