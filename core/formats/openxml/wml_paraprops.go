// Paragraph properties: capturing a w:pPr blob, the paragraph style it names,
// the rPr children inside it that survive stripping, and the deleted
// paragraph-mark tests that drive mergeable-block handling.

package openxml

import (
	"encoding/xml"
	"strings"
)

// captureParaProps captures paragraph properties as raw XML and extracts the pStyle value.
func captureParaProps(d *rawDecoder, start xml.StartElement) (string, string, error) {
	raw, err := captureRawElement(d, start)
	if err != nil {
		return "", "", err
	}
	// Extract pStyle value from the raw XML
	styleID := extractPStyle(raw)
	return raw, styleID, nil
}

// paragraphHasDeletedMark reports whether the raw `<w:pPr>` payload
// contains a `<w:rPr>` direct child that itself carries a `<w:del>` or
// `<w:moveFrom>` start element — the "deleted paragraph mark" /
// "moved-from paragraph mark" tracked-change markers introduced by
// ECMA-376 Part 1 §17.13.5.13 (CT_ParaRPr) and §17.13.5.14
// (CT_ParaRPrChange).
//
// In ECMA-376 these markers indicate that the paragraph mark (¶) itself
// is part of a tracked deletion / move-from. Under auto-accept-revisions
// the paragraph break is removed, which collapses the paragraph into the
// following one. Upstream Okapi mirrors this via
// `ParagraphBlockProperties.containsRunPropertyDeletedParagraphMark()`
// (ParagraphBlockProperties.java lines 576-586) — keyed on
// `SkippableElement.RevisionProperty.RUN_PROPERTY_DELETED_PARAGRAPH_MARK`
// (`w:del`) and `RUN_PROPERTY_MOVED_PARAGRAPH_FROM` (`w:moveFrom`) per
// SkippableElement.java lines 232 and 234. `BlockParser.parse` lines
// 207-213 then sets `builder.mergeable(true)` when this marker is
// present so `StyledTextPart.process` (lines 312-319) can absorb the
// paragraph into the next block.
//
// We use the xml.Decoder for safety rather than substring search so
// nested `<w:pPrChange>` history (which can itself contain a
// `<w:rPr><w:del/></w:rPr>` re-stating the pre-change state) does not
// produce a false positive — we only consider the immediate
// `<w:pPr><w:rPr>` direct-child path.
func paragraphHasDeletedMark(raw string) bool {
	if raw == "" {
		return false
	}
	if !strings.Contains(raw, "<w:del") && !strings.Contains(raw, "<w:moveFrom") {
		return false
	}
	dec := newRawDecoderString(raw)
	var depth int
	// Path stack of element local names from the root <w:pPr>.
	var path []string
	for {
		tok, err := dec.Token()
		if err != nil {
			return false
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			path = append(path, t.Name.Local)
			// We want the chain <pPr> (depth 1) -> <rPr> (depth 2) ->
			// <del>|<moveFrom> (depth 3). pPrChange / rPrChange history
			// blocks live one level deeper, so this check excludes them.
			if depth == 3 && len(path) >= 3 &&
				path[0] == "pPr" && path[1] == "rPr" &&
				(t.Name.Local == "del" || t.Name.Local == "moveFrom") {
				return true
			}
		case xml.EndElement:
			depth--
			if len(path) > 0 {
				path = path[:len(path)-1]
			}
		}
	}
}

// stripPPrIfDeletedMark returns an empty string when the captured
// paragraph-properties XML carries a `<w:del>` or `<w:moveFrom>` paragraph-
// mark revision marker inside `<w:pPr>/<w:rPr>`. Otherwise it returns the
// input unchanged.
//
// Mirrors upstream Okapi BlockParser.parse (BlockParser.java:207-213):
// when `ParagraphBlockProperties.containsRunPropertyDeletedParagraphMark()`
// returns true (ParagraphBlockProperties.java:576-586), the parser sets
// `builder.mergeable(true)` and SKIPS adding the blockProperties to the
// RunBuilder's markup. The pPr never reaches the emitted block — only the
// `mergeable` flag is set, and `StyledTextPart.process` either absorbs
// the block into the next paragraph (`block.mergeWith(mergeableBlock)`,
// Block.java:139-166, which copies chunks 1..N-1, NOT chunk 0 which
// carries the paragraph open + pPr) or emits the dangling block at EOF
// without ever materialising the suppressed pPr.
//
// The native parser already handles the partMergeable absorption path
// (lines 2398-2404 + 2495-2502 below). But absorption is GATED off when
// an extractable complex field is open across the paragraph boundary
// (the `!(cfs.active && cfs.extractable)` guard at 2495) — in that
// state Okapi's `RunParser.parseComplexField` (RunParser.java:516-528 +
// 594-609) routes the inner `<w:pPr>` through `deferredEvents`, the
// pPr arrives at `BlockParser.parse` later, and BlockParser still
// applies the `containsRunPropertyDeletedParagraphMark` gate then —
// dropping the pPr exactly the same way.
//
// To mirror that final emit, the native skeleton write paths funnel
// paraProps through this helper so the pPr disappears whenever its
// rPr carries a `<w:del>` / `<w:moveFrom>` paragraph-mark marker —
// regardless of whether the merge actually absorbed the runs. The
// paragraph still emits as a structural shell (`<w:p>` / `<w:p/>`),
// but without the suppressed pPr.
//
// Fixture 1102.docx: P2 (content + del-marked pPr inside open HYPERLINK
// field) and P3 (empty + del-marked pPr, still inside the open field)
// both lose their pPr in the reference output; native previously
// preserved the source pPr (including a `<w:rStyle w:val="Hyperlink"/>`
// child that's invisible-but-real after `<w:ins>`/`<w:del>` revision
// markers are stripped).
//
// References:
//   - ECMA-376-1 §17.13.5.13 (CT_ParaRPr) — defines `<w:del>` /
//     `<w:moveFrom>` inside `<w:pPr>/<w:rPr>` as paragraph-mark
//     revisions, the same shape this helper detects.
//   - Okapi BlockParser.java:207-213 — the suppression site.
//   - Okapi ParagraphBlockProperties.java:576-586 —
//     containsRunPropertyDeletedParagraphMark logic this mirrors.
func stripPPrIfDeletedMark(raw string) string {
	if !paragraphHasDeletedMark(raw) {
		return raw
	}
	return ""
}

// extractPStyle extracts the w:val attribute from <w:pStyle> in raw paragraph properties XML.
func extractPStyle(raw string) string {
	idx := strings.Index(raw, "<w:pStyle")
	if idx < 0 {
		// Try without namespace prefix
		idx = strings.Index(raw, "<pStyle")
		if idx < 0 {
			return ""
		}
	}
	// Find w:val="..." or val="..."
	valIdx := strings.Index(raw[idx:], `val="`)
	if valIdx < 0 {
		return ""
	}
	start := idx + valIdx + 5
	end := strings.Index(raw[start:], `"`)
	if end < 0 {
		return ""
	}
	return raw[start : start+end]
}
