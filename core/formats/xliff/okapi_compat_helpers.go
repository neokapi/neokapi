package xliff

import (
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
	re := regexp.MustCompile(`<` + regexp.QuoteMeta(tag) + `[\s>][^>]*>`)
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

// hoistAltTransNotes finds every `<alt-trans>…<note>…</note>…</alt-trans>`
// span in the buffer and pulls the inner `<note>` elements out, placing
// them immediately before the `<alt-trans>` start tag (preserving their
// original relative order). This mimics okapi's reader-side behavior of
// collapsing alt-trans-scoped notes into the trans-unit's note bag,
// which the writer then emits as a flat note sequence at trans-unit
// level before the alt-trans element.
//
// The post-processed text concatenates: existing trans-unit-level notes
// keep their position; alt-trans inner notes are inserted just before
// the alt-trans element. Spec note: this **loses semantic information**
// (which note belongs to which alt-trans alternate) — okapi-compat only.
//
// Implementation is regex-based; it assumes well-formed `<alt-trans>`
// blocks without nested alt-trans (which the spec doesn't allow anyway).
func hoistAltTransNotes(b []byte) []byte {
	return altTransNoteRE.ReplaceAllFunc(b, func(match []byte) []byte {
		// Extract all <note>…</note> chunks from inside the alt-trans.
		notes := noteElemRE.FindAll(match, -1)
		if len(notes) == 0 {
			return match
		}
		// Strip the notes from inside the alt-trans body.
		stripped := noteElemRE.ReplaceAll(match, nil)
		// Concatenate: notes (in original order) + stripped alt-trans.
		var out []byte
		for _, n := range notes {
			out = append(out, n...)
		}
		out = append(out, stripped...)
		return out
	})
}

var (
	// Match a complete <alt-trans …>…</alt-trans> span. Greedy on the
	// inner `[\s\S]` to consume nested elements; safe because XLIFF
	// 1.2 §2.4.7 disallows nested alt-trans.
	altTransNoteRE = regexp.MustCompile(`(?s)<alt-trans[\s>][^>]*>.*?</alt-trans>`)
	// Match a complete <note …>…</note> span. Inner is non-greedy
	// so adjacent notes don't merge.
	noteElemRE = regexp.MustCompile(`(?s)<note[\s>][^>]*>.*?</note>`)
)

// reorderHeaderToolToEnd moves <tool>…</tool> elements within the
// <header>…</header> region to appear after any <note>…</note>
// siblings. okapi's reader collects header children into typed bags
// and the writer emits them in a fixed order; the source's authored
// order is lost.
//
// This regex post-pass finds the header span, extracts every <tool>
// chunk in order, removes them from their original positions, and
// inserts them at the position of the LAST <note> inside header (or
// at the end of header content if no notes exist).
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
		// Find the last <note> position to insert tools after it.
		noteMatches := noteElemRE.FindAllIndex(stripped, -1)
		insertAt := -1
		if len(noteMatches) > 0 {
			insertAt = noteMatches[len(noteMatches)-1][1]
		}
		if insertAt < 0 {
			// No notes — insert before </header>.
			closeIdx := bytesLastIndexString(stripped, "</header>")
			if closeIdx < 0 {
				return headerMatch
			}
			insertAt = closeIdx
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

var (
	// Match a complete <header …>…</header> span.
	headerSpanRE = regexp.MustCompile(`(?s)<header[\s>][^>]*>.*?</header>`)
	// Match a complete <tool …>…</tool> or self-closing <tool …/>.
	toolElemRE = regexp.MustCompile(`(?s)<tool[\s>][^>]*?(/>|>.*?</tool>)`)
)

// bytesLastIndexString returns the last index of s in b, or -1.
// strings.LastIndex would require a string conversion; this is
// equivalent for our regex post-processing context.
func bytesLastIndexString(b []byte, s string) int {
	return strings.LastIndex(string(b), s)
}
