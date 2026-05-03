package xliff

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
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

// escapeNonASCIIAsEntities replaces every non-ASCII rune in s with its
// XML numeric character reference `&#xNNNN;` (lowercase hex, no
// padding above 4 digits). ASCII bytes pass through unchanged. This
// mirrors okapi's XLIFFWriter behavior of writing all chars > U+007F
// as numeric entities regardless of declared output encoding.
//
// Used by OkapiCompatConfig.EscapeNonASCIIAsEntities. Applied AFTER
// the standard XML attribute/text escaping (it operates on the
// rendered string, replacing UTF-8 sequences with entities). Pre-
// existing `&...;` entities are left alone since they're already
// ASCII-only.
func escapeNonASCIIAsEntities(s string) string {
	if s == "" {
		return s
	}
	allASCII := true
	for i := 0; i < len(s); i++ {
		if s[i] >= 0x80 {
			allASCII = false
			break
		}
	}
	if allASCII {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r < 0x80 {
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
	for i := 0; i < len(s); i++ {
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

var (
	tagStartREs  = map[string]*regexp.Regexp{}
	innerAttrREs = map[string]*regexp.Regexp{}
)

func tagStartRE(tag string) *regexp.Regexp {
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

// unwrapSingleSegMrkWhenSourceDiffers drops <seg-source>…</seg-source>
// and unwraps a single `<mrk mid mtype="seg">…</mrk>` wrapper from
// `<target>` content WHEN the source content differs from the
// seg-source content (text-only comparison).
//
// This mirrors okapi's XLIFFFilter.java:2278 logic: when the
// `<seg-source>` content disagrees with the `<source>` content
// (CODE_DATA_ONLY compare), okapi falls back to the un-segmented
// source — the segmentation markers are effectively discarded.
//
// Only applied when OkapiCompatConfig.UnwrapSingleSegMrk is enabled.
// Implementation walks each <trans-unit> and rewrites its body when
// the divergence is detected.
func unwrapSingleSegMrkWhenSourceDiffers(b []byte) []byte {
	return rewriteTransUnitSpans(b, unwrapSingleSegMrkInTransUnit)
}

func unwrapSingleSegMrkInTransUnit(tu []byte) []byte {
	srcText := extractElementText(tu, "source")
	segSrcText := extractElementText(tu, "seg-source")
	if segSrcText == "" {
		return tu
	}
	// Compare normalized text (whitespace-insensitive). When source !=
	// seg-source, okapi drops the segmentation.
	if normalizeForCompare(srcText) == normalizeForCompare(segSrcText) {
		return tu
	}
	// Drop <seg-source>…</seg-source>.
	out := segSourceElemRE.ReplaceAll(tu, nil)
	// Unwrap <mrk mid="…" mtype="seg">…</mrk> in <target>.
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

// normalizeForCompare prepares a source/seg-source text string for
// byte comparison. Mirrors okapi's CODE_DATA_ONLY compare on the
// joined-segments form: each segment's text is character-exact, and
// the inter-segment whitespace is dropped (because it lives between
// `<mrk>` siblings, not inside any segment).
//
// Steps:
//  1. Decode the XML predefined entities (&gt; &lt; &amp; &quot;
//     &apos;). The native writer's two text-emission paths emit `>`
//     differently: xmlEscapeText only escapes `]]>` per XML 1.0 §2.4
//     while the inline encoder escapes every `>`. Source and
//     seg-source can therefore disagree on byte form despite
//     identical semantic content; decoding cancels the asymmetry.
//  2. Trim leading/trailing whitespace (CharData around the element's
//     direct children — non-significant for content).
//  3. NO internal whitespace collapsing: differences like
//     `"About the  Agent"` (two spaces) vs `"About the Agent"` (one
//     space) are real semantic differences that okapi flags as
//     mismatches.
//
// Inter-mrk whitespace in seg-source is naturally dropped by the
// caller's tag-strip pre-pass (only mrk start/end tags are stripped;
// the whitespace between adjacent mrks lives between siblings and
// becomes inter-segment text — which we want to compare against
// source's same-position text).
func normalizeForCompare(s string) string {
	return strings.TrimSpace(decodeBasicEntities(s))
}

// decodeBasicEntities replaces the five XML predefined entities with
// their character equivalents. Ignores numeric character references
// (those rarely differ between source and seg-source in practice).
func decodeBasicEntities(s string) string {
	if !strings.Contains(s, "&") {
		return s
	}
	out := strings.ReplaceAll(s, "&gt;", ">")
	out = strings.ReplaceAll(out, "&lt;", "<")
	out = strings.ReplaceAll(out, "&quot;", "\"")
	out = strings.ReplaceAll(out, "&apos;", "'")
	out = strings.ReplaceAll(out, "&amp;", "&")
	return out
}

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

// bytesLastIndexString returns the last index of s in b, or -1.
// strings.LastIndex would require a string conversion; this is
// equivalent for our regex post-processing context.
func bytesLastIndexString(b []byte, s string) int {
	return strings.LastIndex(string(b), s)
}
