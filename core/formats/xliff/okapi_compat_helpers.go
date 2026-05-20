package xliff

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/ianaindex"
)

// canonicalBCP47 normalizes a BCP-47 language tag to its canonical
// lowercase form (RFC 5646 §2.1.1). For the simple subtag-only forms
// neokapi sees (e.g. "DE", "en-US"), this is just lowercasing the
// language subtag. We intentionally do not lowercase region/script
// subtags — BCP-47 says region is title-case-uppercase and script is
// title-case (e.g. "zh-Hans-CN") — but for the okapi-compat use case
// the relevant fixtures only have language subtags.
//
// Used by OkapiCompatConfig.LowercaseLangSubtag.
func canonicalBCP47(tag string) string {
	if tag == "" {
		return tag
	}
	parts := strings.SplitN(tag, "-", 2)
	parts[0] = strings.ToLower(parts[0])
	return strings.Join(parts, "-")
}

// escapeUnencodableAsEntities replaces every rune in s that the given
// encoder cannot represent with its XML numeric character reference
// `&#xNNNN;` (lowercase hex, 4-digit padding for ≤0xFFFF). Encodable
// chars (and all ASCII) pass through unchanged.
//
// Mirrors okapi's XMLEncoder._encode behavior (XMLEncoder.java:191-213):
// when the source-declared encoding is non-UTF-8 (windows-1252,
// ISO-8859-1, …), chars not representable in the declared charset are
// emitted as numeric entities. ALL Latin-1 chars and the windows-1252
// "Windows extension" chars in the C1 range (e.g. U+0192 ƒ, U+2026 …,
// U+20AC €) are encodable as windows-1252 single bytes and stay literal;
// other chars (Latin Extended-A/B, CJK, …) are escaped.
//
// Used by OkapiCompatConfig.EscapeBeyondLatin1AsEntities (the flag
// name reflects the typical case but the actual rule is encoder-driven).
// Applied AFTER standard XML attribute/text escaping; pre-existing
// `&…;` entities are ASCII-only and stay literal.
func escapeUnencodableAsEntities(s string, enc *encoding.Encoder) string {
	if s == "" || enc == nil {
		return s
	}
	hasUnencodable := false
	for _, r := range s {
		if r >= 0x80 && !canEncode(enc, r) {
			hasUnencodable = true
			break
		}
	}
	if !hasUnencodable {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r < 0x80 || canEncode(enc, r) {
			b.WriteRune(r)
			continue
		}
		if r <= 0xFFFF {
			fmt.Fprintf(&b, "&#x%04x;", r)
		} else {
			fmt.Fprintf(&b, "&#x%x;", r)
		}
	}
	return b.String()
}

// canEncode reports whether enc can represent the single rune r.
// Encoders are not safe for concurrent use, but our writer paths each
// build their own encoder once per write and call canEncode many times,
// so the encoder lifetime here matches a single Write call.
func canEncode(enc *encoding.Encoder, r rune) bool {
	if r < 0x80 {
		return true
	}
	_, err := enc.String(string(r))
	return err == nil
}

// encoderForCharset returns an Encoder for the named IANA charset, or
// nil when the name is empty / UTF-8 / unknown. The writer uses this
// for the okapi-compat encoding-conditional escape: a non-nil encoder
// means "escape chars this encoder cannot handle".
func encoderForCharset(name string) *encoding.Encoder {
	if name == "" || strings.EqualFold(name, "UTF-8") || strings.EqualFold(name, "UTF8") {
		return nil
	}
	enc, err := ianaindex.IANA.Encoding(name)
	if err != nil || enc == nil {
		return nil
	}
	return enc.NewEncoder()
}

// stripCDataCREntities removes &#xD; (carriage return numeric refs)
// from text content. okapi's TextFragment normalizes CR to LF
// internally and never re-emits the CR entity even when the source
// preserved it. Both `&#xD;` and `&#xd;` (XML allows both cases for
// hex digits) are matched.
//
// Used by OkapiCompatConfig.StripCDataCREntities.
func stripCDataCREntities(s string) string {
	if s == "" || !strings.Contains(s, "&#x") {
		return s
	}
	out := strings.ReplaceAll(s, "&#xD;", "")
	out = strings.ReplaceAll(out, "&#xd;", "")
	return out
}

// simulateBrokenWindows1252 replaces every non-ASCII rune in s with
// the U+FFFD REPLACEMENT CHARACTER. okapi's xliff filter has a bug
// where windows-1252 single-byte chars for accented Latin letters end
// up as U+FFFD in the output. This helper applies the same loss to
// neokapi's output so the byte-equivalent comparison passes.
//
// Used by OkapiCompatConfig.SimulateBrokenWindows1252Read at the
// reader level (applied to text content originating from a non-UTF-8
// source file before runs reach the writer).
func simulateBrokenWindows1252(s string) string {
	if s == "" {
		return s
	}
	hasNonASCII := false
	for i := range len(s) {
		if s[i] >= 0x80 {
			hasNonASCII = true
			break
		}
	}
	if !hasNonASCII {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r < 0x80 {
			b.WriteRune(r)
		} else {
			b.WriteRune('�')
		}
	}
	return b.String()
}

// utf8Valid reports whether b is a valid UTF-8 sequence — used to
// short-circuit transcoding helpers when the input is already valid
// UTF-8.
func utf8Valid(b []byte) bool { return utf8.Valid(b) }

// applySkeletonAttrStripping mutates skeleton text bytes to drop
// attributes that the configured OkapiCompat flags say should be
// stripped. The skeleton text contains start tags verbatim from the
// source file (e.g. `<trans-unit id="2" approved="no" ...>`); we
// regex-match the relevant element start tags and strip the named
// attributes from inside them.
//
// This is fragile in the general case (regex-on-XML) but fine for
// parity comparison on the well-formed XLIFF inputs in our fixture
// set. Both single- and double-quoted attribute values are handled.
func applySkeletonAttrStripping(b []byte, compat OkapiCompatConfig) []byte {
	out := b
	if compat.StripTransUnitApprovedAttr {
		out = stripAttrInTag(out, "trans-unit", "approved")
	}
	if compat.StripPhaseDateAttr {
		out = stripAttrInTag(out, "phase", "date")
	}
	return out
}

// stripAttrInTag finds every `<tag …>` start tag in b and removes
// occurrences of the named attribute from inside the start tag. Other
// content is left untouched. Both quoting forms (`attr="…"`,
// `attr='…'`) are matched. The leading whitespace before the
// attribute is consumed so the surviving attributes still parse.
func stripAttrInTag(b []byte, tag, attr string) []byte {
	tagRE := tagStartRE(tag)
	attrRE := innerAttrRE(attr)
	return tagRE.ReplaceAllFunc(b, func(match []byte) []byte {
		return attrRE.ReplaceAll(match, nil)
	})
}

// reCacheMu guards the two regexp caches below. Without it, two
// concurrent writer goroutines (e.g. a flow processing several XLIFF
// files in parallel) compiling the same tag/attr regex for the first
// time would race on the map write, which the Go race detector flags
// and which can corrupt the map. A single mutex is enough: regex
// compilation is rare (once per distinct tag/attr name) and cheap to
// serialize.
var (
	reCacheMu    sync.Mutex
	tagStartREs  = map[string]*regexp.Regexp{}
	innerAttrREs = map[string]*regexp.Regexp{}
)

func tagStartRE(tag string) *regexp.Regexp {
	reCacheMu.Lock()
	defer reCacheMu.Unlock()
	if re, ok := tagStartREs[tag]; ok {
		return re
	}
	// Allows optional namespace prefix (e.g. `<xlf:trans-unit>`).
	// Start-tag form is `<tag>` (no attrs) or `<tag attr=…>` (with attrs);
	// the `[\s>][^>]*>` form would accidentally extend past the start
	// tag's `>` when there are no attrs (consuming through the next `>`),
	// so split into two alternatives.
	re := regexp.MustCompile(`<(?:[A-Za-z_][\w.-]*:)?` + regexp.QuoteMeta(tag) + `(?:\s[^>]*)?>`)
	tagStartREs[tag] = re
	return re
}

func innerAttrRE(attr string) *regexp.Regexp {
	reCacheMu.Lock()
	defer reCacheMu.Unlock()
	if re, ok := innerAttrREs[attr]; ok {
		return re
	}
	// `\s+attr\s*=\s*"…"` or `…='…'`. Quote contents are everything
	// that isn't the same quote char.
	re := regexp.MustCompile(`\s+` + regexp.QuoteMeta(attr) + `\s*=\s*("[^"]*"|'[^']*')`)
	innerAttrREs[attr] = re
	return re
}

// hoistAltTransNotes mirrors okapi's NOTEMARKER skeleton-placement rule
// (XLIFFFilter.java:2682, addNoteMarker called from addTargetIfNeeded).
// For each `<trans-unit>` span containing one or more `<alt-trans>`
// children, it collects every `<note>` element in the trans-unit
// (including notes nested inside alt-trans), strips them from their
// source positions, and re-emits them all immediately before the FIRST
// `<alt-trans>` start tag — exactly where okapi's NOTEMARKER lands.
//
// This loses the "which note belongs to which alt-trans alternate"
// relationship (which okapi loses too — all notes go into the trans-unit
// note bag and re-emerge at one position). XLIFF 1.2 §2.5 places
// `<note>` at trans-unit, group, or alt-trans level — putting them all
// at trans-unit level is spec-valid.
//
// trans-units without `<alt-trans>` children are left alone — okapi's
// NOTEMARKER positioning logic only fires when alt-trans is present.
//
// Implementation walks `<trans-unit>` spans via a depth-tracking byte
// scan (regex alone can't handle nested elements safely). Inside each
// trans-unit, regex matches notes/alt-trans positions.
func hoistAltTransNotes(b []byte) []byte {
	return rewriteTransUnitSpans(b, hoistNotesInTransUnit)
}

// hoistNotesInTransUnit returns the trans-unit body with all <note>
// elements relocated to right before the first <alt-trans>, or the
// body unchanged when no <alt-trans> is present.
func hoistNotesInTransUnit(tu []byte) []byte {
	altLoc := altTransStartTagRE.FindIndex(tu)
	if altLoc == nil {
		return tu
	}
	notes := noteElemRE.FindAll(tu, -1)
	if len(notes) == 0 {
		return tu
	}
	// Track positions to strip notes by index, then recompute alt-trans
	// position after stripping.
	stripped := noteElemRE.ReplaceAll(tu, nil)
	altLoc = altTransStartTagRE.FindIndex(stripped)
	if altLoc == nil {
		return tu
	}
	var out []byte
	out = append(out, stripped[:altLoc[0]]...)
	for _, n := range notes {
		out = append(out, n...)
	}
	out = append(out, stripped[altLoc[0]:]...)
	return out
}

// altTransStartTagRE matches the OPENING tag of <alt-trans> (with or
// without namespace prefix). Used to locate the position where notes
// should be inserted.
var altTransStartTagRE = regexp.MustCompile(`<(?:[A-Za-z_][\w.-]*:)?alt-trans[\s>]`)

// stripAltTransSegSource removes any `<seg-source>…</seg-source>`
// element nested inside an `<alt-trans>` body. okapi's XLIFFFilter
// treats alt-trans as a flat (source, target) translation-memory
// match and never round-trips an inner seg-source. Trans-units
// without alt-trans are unaffected.
//
// Implementation walks each <alt-trans> span (depth-tracked, to
// safely skip the alt-trans's own nested <source>/<target>) and
// strips a single seg-source element if present. Surrounding
// whitespace is preserved as-is.
func stripAltTransSegSource(b []byte) []byte {
	return rewriteAltTransSpans(b, func(at []byte) []byte {
		return altTransSegSourceRE.ReplaceAll(at, nil)
	})
}

// altTransSegSourceRE matches a `<seg-source …>…</seg-source>` element
// (single line or multi-line) along with the line-break that follows
// it, so removing it doesn't leave a stray blank line behind.
var altTransSegSourceRE = regexp.MustCompile(`(?s)<(?:[A-Za-z_][\w.-]*:)?seg-source\b[^>]*>.*?</(?:[A-Za-z_][\w.-]*:)?seg-source>\s*\n?`)

// altTransEndTagRE matches the CLOSING tag of <alt-trans>.
var altTransEndTagRE = regexp.MustCompile(`</(?:[A-Za-z_][\w.-]*:)?alt-trans>`)

// rewriteAltTransSpans applies fn to each <alt-trans>…</alt-trans>
// span. Mirrors rewriteTransUnitSpans but for alt-trans. Depth
// tracking is needed because alt-trans can contain its own
// <source>/<target>/<seg-source>.
func rewriteAltTransSpans(b []byte, fn func([]byte) []byte) []byte {
	var out []byte
	cursor := 0
	for cursor < len(b) {
		startLoc := altTransStartTagRE.FindIndex(b[cursor:])
		if startLoc == nil {
			out = append(out, b[cursor:]...)
			break
		}
		startAbs := cursor + startLoc[0]
		out = append(out, b[cursor:startAbs]...)
		endAbs := findAltTransEnd(b, startAbs)
		if endAbs < 0 {
			out = append(out, b[startAbs:]...)
			break
		}
		span := b[startAbs:endAbs]
		out = append(out, fn(span)...)
		cursor = endAbs
	}
	return out
}

// findAltTransEnd returns the absolute byte offset just past
// `</alt-trans>` for the element starting at `start`, or -1 on
// malformed input. alt-trans can contain its own nested elements but
// not nested alt-trans, so a simple depth-1 close scan suffices.
func findAltTransEnd(b []byte, start int) int {
	openEnd := bytes.IndexByte(b[start:], '>')
	if openEnd < 0 {
		return -1
	}
	cursor := start + openEnd + 1
	loc := altTransEndTagRE.FindIndex(b[cursor:])
	if loc == nil {
		return -1
	}
	return cursor + loc[1]
}

// unwrapSingleSegMrkWhenSourceDiffers drops <seg-source>…</seg-source>
// and unwraps a single `<mrk mid mtype="seg">…</mrk>` wrapper from
// `<target>` content WHEN the reader flagged this trans-unit's
// seg-source as divergent from its source.
//
// Mirrors okapi's XLIFFFilter.java:2278 logic: when the
// `<seg-source>` content disagrees with the `<source>` content
// (CODE_DATA_ONLY compare with the `unwrap()` whitespace pre-pass
// that respects xml:space), okapi falls back to the un-segmented
// source — the segmentation markers are effectively discarded. The
// reader applies the same comparison (with the per-unit xml:space
// context this writer post-pass lacks) and stores the decision as a
// `xliff:divergent-segsource` annotation on each affected Block; the
// `divergent` bitmask here is indexed in document order matching
// w.blocks.
//
// Only applied when OkapiCompatConfig.UnwrapSingleSegMrk is enabled.
func unwrapSingleSegMrkWhenSourceDiffers(b []byte, divergent []bool) []byte {
	idx := 0
	return rewriteTransUnitSpans(b, func(tu []byte) []byte {
		i := idx
		idx++
		if i >= len(divergent) || !divergent[i] {
			return tu
		}
		return unwrapSingleSegMrkInTransUnit(tu)
	})
}

func unwrapSingleSegMrkInTransUnit(tu []byte) []byte {
	if extractElementText(tu, "seg-source") == "" {
		return tu
	}
	// Carve <alt-trans> spans out before regex passes — okapi's
	// segmentation drop fires per trans-unit and never touches
	// alt-trans alternatives, which carry their own (source, target)
	// pair and are allowed to keep their mrk segmentation. Process
	// only the non-alt-trans portions, then re-stitch the alt-trans
	// spans back verbatim so their inner targets stay intact.
	var processed []byte
	cursor := 0
	for cursor < len(tu) {
		startLoc := altTransStartTagRE.FindIndex(tu[cursor:])
		if startLoc == nil {
			processed = append(processed, applyUnwrap(tu[cursor:])...)
			break
		}
		startAbs := cursor + startLoc[0]
		processed = append(processed, applyUnwrap(tu[cursor:startAbs])...)
		endAbs := findAltTransEnd(tu, startAbs)
		if endAbs < 0 {
			processed = append(processed, tu[startAbs:]...)
			break
		}
		processed = append(processed, tu[startAbs:endAbs]...)
		cursor = endAbs
	}
	return processed
}

// applyUnwrap performs the source!=seg-source segmentation drop on a
// chunk that is guaranteed not to contain any <alt-trans> span: drop
// any <seg-source>…</seg-source>, unwrap single mrk-seg wrappers in
// <target>.
func applyUnwrap(b []byte) []byte {
	out := segSourceElemRE.ReplaceAll(b, nil)
	out = targetMrkUnwrapRE.ReplaceAllFunc(out, func(targetMatch []byte) []byte {
		return mrkSegUnwrapRE.ReplaceAll(targetMatch, []byte("$1"))
	})
	return out
}

// segSourceElemRE matches <seg-source>…</seg-source> with optional ns
// prefix, including any leading whitespace.
var segSourceElemRE = regexp.MustCompile(`(?s)\s*<(?:[A-Za-z_][\w.-]*:)?seg-source(?:\s[^>]*)?>.*?</(?:[A-Za-z_][\w.-]*:)?seg-source>`)

// targetMrkUnwrapRE matches a complete <target>…</target> span so we
// can unwrap any single mrk-seg wrapper inside it.
var targetMrkUnwrapRE = regexp.MustCompile(`(?s)<(?:[A-Za-z_][\w.-]*:)?target(?:\s[^>]*)?>.*?</(?:[A-Za-z_][\w.-]*:)?target>`)

// mrkSegUnwrapRE matches `<mrk mid="…" mtype="seg">CONTENT</mrk>` and
// captures CONTENT in group 1 — used to strip the wrapper while
// keeping inner content.
var mrkSegUnwrapRE = regexp.MustCompile(`(?s)<(?:[A-Za-z_][\w.-]*:)?mrk[^>]*\bmtype="seg"[^>]*>(.*?)</(?:[A-Za-z_][\w.-]*:)?mrk>`)

// extractElementText returns the inner text content of the FIRST
// element with the given local name in b, with all child element tags
// stripped (text-only). Returns "" when not found.
//
// Used to compare source vs seg-source content for okapi-compat
// unwrap detection.
func extractElementText(b []byte, localName string) string {
	startRE := regexp.MustCompile(`<(?:[A-Za-z_][\w.-]*:)?` + regexp.QuoteMeta(localName) + `(?:\s[^>]*)?>`)
	endRE := regexp.MustCompile(`</(?:[A-Za-z_][\w.-]*:)?` + regexp.QuoteMeta(localName) + `>`)
	startLoc := startRE.FindIndex(b)
	if startLoc == nil {
		return ""
	}
	rest := b[startLoc[1]:]
	endLoc := endRE.FindIndex(rest)
	if endLoc == nil {
		return ""
	}
	inner := rest[:endLoc[0]]
	// Strip all element tags to get text only.
	stripped := tagStripRE.ReplaceAll(inner, nil)
	return string(stripped)
}

var tagStripRE = regexp.MustCompile(`<[^>]+>`)

// rewriteTransUnitSpans applies fn to each <trans-unit>…</trans-unit>
// span in b and returns the rewritten buffer. Walks element boundaries
// while tracking depth so nested elements (e.g. <source>/<target>
// inside the trans-unit, or alt-trans within) don't trip the matcher.
//
// Why not regex alone: trans-units can contain multi-level nested
// content, and a non-greedy regex match on `<trans-unit>...</trans-unit>`
// can mis-pair start/end when alt-trans contains its own source/target.
func rewriteTransUnitSpans(b []byte, fn func([]byte) []byte) []byte {
	var out []byte
	cursor := 0
	for cursor < len(b) {
		startLoc := transUnitStartTagRE.FindIndex(b[cursor:])
		if startLoc == nil {
			out = append(out, b[cursor:]...)
			break
		}
		startAbs := cursor + startLoc[0]
		out = append(out, b[cursor:startAbs]...)
		// Find matching </trans-unit> by depth-tracking.
		endAbs := findTransUnitEnd(b, startAbs)
		if endAbs < 0 {
			out = append(out, b[startAbs:]...)
			break
		}
		span := b[startAbs:endAbs]
		out = append(out, fn(span)...)
		cursor = endAbs
	}
	return out
}

// transUnitStartTagRE matches the OPENING tag of <trans-unit>.
var transUnitStartTagRE = regexp.MustCompile(`<(?:[A-Za-z_][\w.-]*:)?trans-unit[\s>]`)

// findTransUnitEnd returns the absolute byte offset of the position
// just past `</trans-unit>` for the trans-unit starting at `start`,
// or -1 if not found / malformed.
func findTransUnitEnd(b []byte, start int) int {
	openEnd := bytes.IndexByte(b[start:], '>')
	if openEnd < 0 {
		return -1
	}
	cursor := start + openEnd + 1
	depth := 1
	for cursor < len(b) {
		lt := bytes.IndexByte(b[cursor:], '<')
		if lt < 0 {
			return -1
		}
		pos := cursor + lt
		// </trans-unit>: decrement.
		if bytes.HasPrefix(b[pos:], []byte("</")) {
			end := bytes.IndexByte(b[pos:], '>')
			if end < 0 {
				return -1
			}
			tag := b[pos : pos+end+1]
			if isTransUnitEndTag(tag) {
				depth--
				if depth == 0 {
					return pos + end + 1
				}
			}
			cursor = pos + end + 1
			continue
		}
		// Comments / PIs / DTDs.
		if pos+1 < len(b) && (b[pos+1] == '!' || b[pos+1] == '?') {
			if bytes.HasPrefix(b[pos:], []byte("<!--")) {
				end := bytes.Index(b[pos:], []byte("-->"))
				if end < 0 {
					return -1
				}
				cursor = pos + end + 3
				continue
			}
			end := bytes.IndexByte(b[pos:], '>')
			if end < 0 {
				return -1
			}
			cursor = pos + end + 1
			continue
		}
		// Start element: track nested trans-unit depth.
		end := bytes.IndexByte(b[pos:], '>')
		if end < 0 {
			return -1
		}
		tag := b[pos : pos+end+1]
		selfClosing := end >= 1 && tag[end-1] == '/'
		if isTransUnitStartTag(tag) && !selfClosing {
			depth++
		}
		cursor = pos + end + 1
	}
	return -1
}

func isTransUnitStartTag(tag []byte) bool {
	m := startElementRE.FindSubmatch(tag)
	return m != nil && string(m[1]) == "trans-unit"
}

func isTransUnitEndTag(tag []byte) bool {
	// </…trans-unit>
	stripped := bytes.TrimPrefix(tag, []byte("</"))
	stripped = bytes.TrimSuffix(stripped, []byte(">"))
	stripped = bytes.TrimSpace(stripped)
	if i := bytes.IndexByte(stripped, ':'); i >= 0 {
		stripped = stripped[i+1:]
	}
	return string(stripped) == "trans-unit"
}

// noteElemRE matches a complete <note …>…</note> span (allows optional
// namespace prefix). The start-tag clause is `<note(?:\s[^>]*)?>` so
// it accepts both attribute-less `<note>` and attribute-bearing
// `<note from="x">`. Inner content is non-greedy so adjacent notes
// don't merge into one match.
var noteElemRE = regexp.MustCompile(`(?s)<(?:[A-Za-z_][\w.-]*:)?note(?:\s[^>]*)?>.*?</(?:[A-Za-z_][\w.-]*:)?note>`)

// reorderHeaderToolToEnd moves <tool>…</tool> elements within the
// <header>…</header> region to mimic okapi's tool-placeholder
// insertion logic in XLIFFFilter.java:893-905. okapi inserts the
// tool-placeholder skeleton entry at the position of the FIRST start
// element inside <header> that is NOT one of:
//
//	skl, glossary, reference, count-group, prop-group, note
//
// (phase-group, phase, and tool itself are extracted into typed bags
// before reaching this branch, so they're not "unknown" elements.)
//
// If every header start element is in the known set, the tool
// placeholder lands at the end of the header (just before </header>).
//
// Spec note: XLIFF 1.2 §2.3 doesn't mandate a strict header child
// order, so both source and rewritten forms are spec-valid.
func reorderHeaderToolToEnd(b []byte) []byte {
	return headerSpanRE.ReplaceAllFunc(b, func(headerMatch []byte) []byte {
		tools := toolElemRE.FindAll(headerMatch, -1)
		if len(tools) == 0 {
			return headerMatch
		}
		stripped := toolElemRE.ReplaceAll(headerMatch, nil)
		insertAt := findFirstUnknownHeaderChild(stripped)
		if insertAt < 0 {
			closeIdx := headerCloseRE.FindIndex(stripped)
			if closeIdx == nil {
				return headerMatch
			}
			insertAt = closeIdx[0]
		}
		var out []byte
		out = append(out, stripped[:insertAt]...)
		for _, t := range tools {
			out = append(out, t...)
		}
		out = append(out, stripped[insertAt:]...)
		return out
	})
}

var headerCloseRE = regexp.MustCompile(`</(?:[A-Za-z_][\w.-]*:)?header>`)

// findFirstUnknownHeaderChild returns the byte offset of the first
// DIRECT-CHILD start element inside the header span (b) whose local
// name is NOT a known XLIFF header child. Walks element boundaries
// while tracking nesting depth so nested children (e.g. <external-file>
// inside <glossary>) don't trip the unknown check.
//
// Returns -1 when every direct-child start element is known.
//
// "Known" mirrors okapi's check in XLIFFFilter.java:893-898, plus
// phase-group/phase/tool themselves (which are handled by separate
// branches before reaching the unknown-element branch).
func findFirstUnknownHeaderChild(b []byte) int {
	knownHeaderChildren := map[string]bool{
		"skl":         true,
		"glossary":    true,
		"reference":   true,
		"count-group": true,
		"prop-group":  true,
		"note":        true,
		"phase-group": true,
		"phase":       true,
		"tool":        true,
	}
	depth := 0 // depth relative to <header>: 0 means direct child
	// Skip the leading <header...> open tag so we start at the first
	// child position. The header span always begins with <header...>.
	openEnd := bytes.IndexByte(b, '>')
	if openEnd < 0 {
		return -1
	}
	cursor := openEnd + 1
	for cursor < len(b) {
		// Find next `<`. If it's a closing/comment/PI, advance past it.
		lt := bytes.IndexByte(b[cursor:], '<')
		if lt < 0 {
			return -1
		}
		pos := cursor + lt
		// </…>: close tag, decrement depth.
		if pos+1 < len(b) && b[pos+1] == '/' {
			end := bytes.IndexByte(b[pos:], '>')
			if end < 0 {
				return -1
			}
			depth--
			cursor = pos + end + 1
			continue
		}
		// <!--…-->, <!…>, <?…?>: skip without changing depth.
		if pos+1 < len(b) && (b[pos+1] == '!' || b[pos+1] == '?') {
			// Comments end at -->, others at the next >.
			if bytes.HasPrefix(b[pos:], []byte("<!--")) {
				end := bytes.Index(b[pos:], []byte("-->"))
				if end < 0 {
					return -1
				}
				cursor = pos + end + 3
				continue
			}
			end := bytes.IndexByte(b[pos:], '>')
			if end < 0 {
				return -1
			}
			cursor = pos + end + 1
			continue
		}
		// Start element: extract local name.
		end := bytes.IndexByte(b[pos:], '>')
		if end < 0 {
			return -1
		}
		tag := b[pos : pos+end+1]
		selfClosing := end >= 1 && tag[end-1] == '/'
		nameMatch := startElementRE.FindSubmatchIndex(tag)
		if nameMatch == nil {
			cursor = pos + end + 1
			continue
		}
		local := string(tag[nameMatch[2]:nameMatch[3]])
		if depth == 0 && !knownHeaderChildren[local] {
			return pos
		}
		if !selfClosing {
			depth++
		}
		cursor = pos + end + 1
	}
	return -1
}

// startElementRE captures the local name of any XML start element
// (with or without namespace prefix). Group 1 is the local name.
var startElementRE = regexp.MustCompile(`<(?:[A-Za-z_][\w.-]*:)?([A-Za-z_][\w.-]*)[\s/>]`)

var (
	// Match a complete <header …>…</header> span. Allows optional
	// namespace prefix. Start-tag clause is `<header(?:\s[^>]*)?>` so
	// it accepts both attribute-less and attribute-bearing forms
	// without accidentally extending past the start tag.
	headerSpanRE = regexp.MustCompile(`(?s)<(?:[A-Za-z_][\w.-]*:)?header(?:\s[^>]*)?>.*?</(?:[A-Za-z_][\w.-]*:)?header>`)
	// Match a complete <tool …>…</tool> or self-closing <tool …/>.
	// Allows optional namespace prefix.
	toolElemRE = regexp.MustCompile(`(?s)<(?:[A-Za-z_][\w.-]*:)?tool(?:\s[^>]*?)?(/>|>.*?</(?:[A-Za-z_][\w.-]*:)?tool>)`)
)

// stripApprovedFromTransUnits removes the `approved="…"` attribute from
// each `<trans-unit>` start tag whose document-order index is true in
// noTargetByIndex. The bitmask must be in the same order the trans-units
// appear in `b` (which is the same order the writer collected blocks).
// Indexing by position rather than by id is required because XLIFF
// allows duplicate trans-unit ids (e.g. SF-12-Test03 has two with
// id="1"), and the strip rule is per-occurrence based on each unit's
// own source target presence.
//
// Walks each trans-unit span via rewriteTransUnitSpans (depth-tracked)
// and rewrites only the start tag — body content (including any
// `approved` attribute that might appear on nested elements) is left
// alone. Trans-units past the end of noTargetByIndex are left alone
// (defensive — should never happen since the writer builds the mask
// from the same w.blocks list whose trans-unit start tags appear in b).
//
// Used by OkapiCompatConfig.StripApprovedWhenNoSourceTarget.
func stripApprovedFromTransUnits(b []byte, noTargetByIndex []bool) []byte {
	if len(noTargetByIndex) == 0 {
		return b
	}
	approvedAttrRE := innerAttrRE("approved")
	idx := 0
	return rewriteTransUnitSpans(b, func(span []byte) []byte {
		thisIdx := idx
		idx++
		if thisIdx >= len(noTargetByIndex) || !noTargetByIndex[thisIdx] {
			return span
		}
		end := bytes.IndexByte(span, '>')
		if end < 0 {
			return span
		}
		startTag := span[:end+1]
		newStart := approvedAttrRE.ReplaceAll(startTag, nil)
		out := make([]byte, 0, len(span)-len(startTag)+len(newStart))
		out = append(out, newStart...)
		out = append(out, span[end+1:]...)
		return out
	})
}
