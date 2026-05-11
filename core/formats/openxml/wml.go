package openxml

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// drawingNameAttrRE matches a name="..." attribute on either a
// non-visual drawing object property element (<wp:docPr>) or a
// non-visual canvas property element (<pic:cNvPr>, <wps:cNvPr>, …).
// Both elements are translatable per Okapi's
// XMLEventHelpers.isDrawingProperty (line 292 of okapi/filters/openxml
// /src/main/java/net/sf/okapi/filters/openxml/XMLEventHelpers.java)
// when ConditionalParameters.getTranslateWordGraphicName() is true
// (default true; ConditionalParameters.java line ~setTranslate-
// WordGraphicName(true) in the constructor). The submatch ordering is:
//
//	[1] open tag prefix up to the name= attribute (incl. the leading
//	    "<docPr " or "<cNvPr " plus any preceding attributes)
//	[2] quote character (' or ")
//	[3] attribute value
//	[4] tail of the open tag (closing '>' or '/>')
//
// Conservative: only matches docPr and cNvPr when they appear in a
// drawing context. We don't try to disambiguate against unrelated
// elements named docPr/cNvPr because none exist in the OOXML schema.
// Multiline/indented forms tolerated via [^>]* segments.
var drawingNameAttrRE = regexp.MustCompile(
	`(<(?:[A-Za-z_][\w-]*:)?(?:docPr|cNvPr)\b[^>]*?\s+name=)(["'])([^"']*)(["'][^>]*?/?>)`,
)

// wmlNamespace is the WordprocessingML namespace.
const wmlNamespace = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"

// textRun holds a single run's text and formatting within a paragraph.
type textRun struct {
	text  string
	props runProps
	// data carries raw XML payload for sentinel runs (drawing, pict,
	// object, oMath, oMathPara, mc:AlternateContent). Empty for plain
	// text and zero-data sentinels (tab, break).
	data string
}

// complexFieldState tracks the state machine for complex field (fldChar) parsing.
type complexFieldState struct {
	active       bool   // inside a complex field (between begin and end)
	fieldCode    string // field instruction name (e.g., "HYPERLINK", "TOC")
	extractable  bool   // whether the field's display text should be extracted
	atResult     bool   // past the "separate" marker (in display text area)
	nestingLevel int    // nesting depth for nested complex fields
}

// wmlParser parses WordprocessingML XML parts (document.xml, headers, footers, etc.).
type wmlParser struct {
	cfg           *Config
	blockCounter  *int
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer
	rels          map[string]relationship // hyperlink rels for this part
	codeFinder    *codeFinder             // regex-based inline code detection
	styles        *styleMap               // resolved style inheritance (nil if not enabled)
}

// parsePart streams through a WordprocessingML XML part, emitting Blocks.
func (p *wmlParser) parsePart(data []byte, partPath string, emitBlock func(*model.Block), emitData func()) error {
	// When AutomaticallyAcceptRevisions is true, pre-process the bytes
	// to drop any <w:tr>...</w:tr> rows whose content carries a
	// <w:moveFrom> revision-tracking wrapper (ECMA-376 Part 1 §17.13.5.17).
	// Mirrors upstream Okapi's MoveFromRevisionCrossStructure +
	// StyledTextPart.process row-removal path: when a
	// moveFromRange.skip() crosses a table-row boundary,
	// extraStructureCrossed() returns LOCAL_TABLE_ROW and
	// StyledTextPart lines 299-305 invoke
	// delayedTableMarkup.removeComponentsFromLastWith(LOCAL_TABLE_ROW)
	// so the row never reaches the writer (SkippableElements.java
	// lines 371-450; StyledTextPart.java lines 580-593).
	//
	// The byte-level pre-pass keeps the streaming parser's hot loop
	// untouched; the alternative — capturing the row body and
	// re-decoding to peek at the moveFrom signal — is invasive,
	// changes namespace resolution semantics for the row's children
	// (encoding/xml binds prefixes per-decoder, our namespace
	// registry is global), and breaks raw-payload capture for VML
	// shapes inside the row. Doing the strip up front sidesteps both.
	if p.cfg != nil && p.cfg.AutomaticallyAcceptRevisions {
		// Cross-structure moveFromRange spans
		// (<w:moveFromRangeStart w:id="N"/>...<w:moveFromRangeEnd
		// w:id="N"/>) consume EVERY event between the start and end
		// markers via SkippableElements.MoveFromRevisionCrossStructure
		// (lines 371-450 of SkippableElements.java). When the skip
		// crosses a paragraph boundary (parentStructureCrossed), the
		// enclosing block is marked skipped via
		// builder.skipped(true) in BlockParser (lines 267-274 of
		// BlockParser.java) and the StyledTextPart outer loop drops
		// it (lines 299-305). Any paragraphs WHOLLY inside the range
		// are also consumed wholesale — including their text content,
		// even untracked — because the cross-structure skip walks
		// past the </w:p>/<w:p> boundaries inside the skip loop.
		//
		// The byte-level pre-pass mirrors this: we identify each
		// matched (moveFromRangeStart id, moveFromRangeEnd id) pair
		// and drop from the start tag of the <w:p> containing
		// moveFromRangeStart through the end tag of the <w:p>
		// containing moveFromRangeEnd. Subsequent rangefulness on
		// table-row-level moveFrom (the <w:moveFrom> wrapper on a
		// row) is handled by dropMoveFromTableRows below.
		data = dropMoveFromRanges(data)
		data = dropMoveFromTableRows(data)
		// After moveFrom rows are removed, a table whose every row
		// was a moveFrom-row becomes structurally empty (only
		// <w:tblPr>/<w:tblGrid> remain). Upstream Okapi cleans this
		// up via StyledTextPart.process lines 410-424 (TableEnd
		// handler): if delayedTableMarkup has no translatable block
		// for the last <w:tbl>, removeComponentsFromLastWith
		// (LOCAL_TABLE) drops the entire table. We mirror that
		// post-pass at the byte level here.
		data = dropEmptyTables(data)
	}
	d := xml.NewDecoder(bytes.NewReader(data))

	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("wml: parsing %s: %w", partPath, err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "p":
				if isWML(t) || isWMLNoNS(t) {
					if err := p.parseParagraph(d, partPath, emitBlock); err != nil {
						return err
					}
				} else {
					p.skelWriteStartElement(t)
				}
			case "sdt":
				// Structured document tag — recurse into content
				if err := p.parseSDT(d, partPath, emitBlock, emitData); err != nil {
					return err
				}
			case "tbl":
				// Table — recurse to find paragraphs inside cells
				p.skelWriteStartElement(t)
			case "tr":
				// Table row — inspect <w:trPr> for the row-deletion
				// marker <w:trPr><w:del .../></w:trPr> (revision tracking,
				// ECMA-376 Part 1 §17.13.5.13 Deleted Table Row). When
				// AutomaticallyAcceptRevisions is true (Okapi default —
				// ConditionalParameters.java line 813), the entire row
				// (start tag, content, end tag) is dropped from the
				// output. Mirrors upstream Okapi
				// StyledTextPart.process() lines 530-551, which calls
				// revisionPropertyTableRowDeletedSkippableElements.skip
				// and then removes the queued row markup via
				// delayedTableMarkup.componentsIteratorAtLastWith(
				// LOCAL_TABLE_ROW); iterator.remove();
				// removeComponentsWith(iterator).
				//
				// The row-INSERTION marker
				// <w:trPr><w:ins .../></w:trPr> (ECMA-376 §17.13.5.16)
				// is ALSO accepted: the inserted row stays, the <w:ins>
				// marker inside trPr is dropped at write time by
				// wmlRevisionParagraphMarkRE. Mirrors upstream
				// revisionPropertyTableRowInsertedSkippableElements.skip
				// at StyledTextPart.java lines 515-528, which drains the
				// <w:ins> element without removing the row.
				if (isWML(t) || isWMLNoNS(t)) && p.cfg != nil && p.cfg.AutomaticallyAcceptRevisions {
					if err := p.handleTableRow(d, t); err != nil {
						return err
					}
					continue
				}
				p.skelWriteStartElement(t)
			case "footnote", "endnote":
				// Skip the auto-generated separator footnotes (id 0 and 1)
				id := attrVal(t, "id")
				if id == "0" || id == "1" || id == "-1" {
					p.skelWriteStartElement(t)
					if err := p.skipAndSkel(d); err != nil {
						return err
					}
					continue
				}
				p.skelWriteStartElement(t)
			case "pPr", "sectPr", "tblPr", "tblGrid", "trPr", "tcPr":
				// Non-translatable properties — skeleton only
				raw, err := captureRawElement(d, t)
				if err != nil {
					return err
				}
				p.skelText(raw)
			default:
				p.skelWriteStartElement(t)
			}

		case xml.EndElement:
			p.skelWriteEndElement(t)

		case xml.CharData:
			p.skelText(xmlEscape(string(t)))

		case xml.ProcInst:
			p.skelText("<?" + t.Target + " " + string(t.Inst) + "?>")

		case xml.Directive:
			p.skelText("<!" + string(t) + ">")

		case xml.Comment:
			p.skelText("<!--" + string(t) + "-->")
		}
	}
	return nil
}

// dropMoveFromTableRows removes every <w:tr ...>...</w:tr> region from
// data whose body contains a <w:moveFrom> revision-tracking content
// wrapper (ECMA-376 Part 1 §17.13.5.17 Move From Run Content). When
// AutomaticallyAcceptRevisions is true such a row's translatable
// content has been moved out — upstream Okapi removes the row markup
// via MoveFromRevisionCrossStructure (lines 371-450 of okapi/filters/
// openxml/src/main/java/net/sf/okapi/filters/openxml/SkippableElements.java)
// tracking tableRowStructureCrossed during the moveFromRange skip,
// then triggering
// delayedTableMarkup.removeComponentsFromLastWith(LOCAL_TABLE_ROW)
// in StyledTextPart (lines 299-305 of StyledTextPart.java).
//
// The detector matches <w:moveFrom (with trailing space or `>`) — the
// content-wrapper form, distinct from <w:moveFromRangeStart and
// <w:moveFromRangeEnd which never carry inner content. Pre-stripping
// at the byte level (rather than mid-parse via lookahead) keeps the
// streaming xml.Decoder loop unchanged and avoids the namespace-
// resolution issues that arise when re-decoding a captured row body
// without its document-scoped xmlns declarations.
//
// Nested rows (legal per the schema — a <w:tc> may contain another
// <w:tbl>) are handled correctly by tracking depth on <w:tr balanced
// open/close pairs.
func dropMoveFromTableRows(data []byte) []byte {
	const trOpen = "<w:tr"
	const trClose = "</w:tr>"
	const moveFromOpen = "<w:moveFrom" // shared prefix; we disambiguate after
	if !bytes.Contains(data, []byte(moveFromOpen)) {
		// Fast path: no moveFrom anywhere in the part, nothing to do.
		return data
	}
	out := make([]byte, 0, len(data))
	for {
		// Find the next <w:tr boundary.
		idx := bytes.Index(data, []byte(trOpen))
		if idx < 0 {
			out = append(out, data...)
			break
		}
		// Validate the element-name boundary: next byte must be `>`,
		// `/`, or whitespace so we don't match `<w:trPr`/`<w:trHeight`.
		j := idx + len(trOpen)
		if j >= len(data) {
			out = append(out, data...)
			break
		}
		b := data[j]
		if b != '>' && b != '/' && b != ' ' && b != '\t' && b != '\n' && b != '\r' {
			// Not <w:tr; advance past this position and keep scanning.
			out = append(out, data[:j+1]...)
			data = data[j+1:]
			continue
		}
		// Find the end of the <w:tr ...> open tag.
		k := bytes.IndexByte(data[j:], '>')
		if k < 0 {
			out = append(out, data...)
			break
		}
		startEnd := j + k // position of the `>` closing the open tag
		// Self-closing <w:tr/> form: empty row, never a moveFrom row.
		if startEnd > 0 && data[startEnd-1] == '/' {
			out = append(out, data[:startEnd+1]...)
			data = data[startEnd+1:]
			continue
		}
		// Find the matching </w:tr> respecting nested rows.
		bodyStart := startEnd + 1
		depth := 1
		cursor := bodyStart
		for depth > 0 {
			nextOpen := bytes.Index(data[cursor:], []byte(trOpen))
			nextClose := bytes.Index(data[cursor:], []byte(trClose))
			if nextClose < 0 {
				// Unbalanced — bail, append remainder unchanged.
				out = append(out, data...)
				return out
			}
			// If the next nested <w:tr open beats the next </w:tr> close,
			// step into it (incrementing depth). Otherwise, step out of
			// the current row (decrementing depth).
			if nextOpen >= 0 && nextOpen < nextClose {
				// Re-validate the nested open's element-name boundary.
				absOpen := cursor + nextOpen
				jj := absOpen + len(trOpen)
				if jj < len(data) {
					bb := data[jj]
					if bb == '>' || bb == '/' || bb == ' ' || bb == '\t' || bb == '\n' || bb == '\r' {
						// Skip past this nested open.
						kk := bytes.IndexByte(data[jj:], '>')
						if kk < 0 {
							out = append(out, data...)
							return out
						}
						nestedOpenEnd := jj + kk
						if nestedOpenEnd > 0 && data[nestedOpenEnd-1] != '/' {
							depth++
						}
						cursor = nestedOpenEnd + 1
						continue
					}
				}
				// Misleading prefix (e.g. <w:trPr inside the body).
				cursor = cursor + nextOpen + len(trOpen)
				continue
			}
			// Step out via the close.
			cursor = cursor + nextClose + len(trClose)
			depth--
		}
		rowEnd := cursor // one past the last byte of </w:tr>
		body := data[bodyStart : rowEnd-len(trClose)]
		if rowBodyHasMoveFromContent(body) {
			// Drop the entire <w:tr>...</w:tr> region (including the
			// open and close tags). Skeleton bytes are written via the
			// streaming pass that follows; removing the bytes here
			// means the streaming pass simply never sees the row.
			out = append(out, data[:idx]...)
			data = data[rowEnd:]
			continue
		}
		// Keep the row verbatim; advance past it.
		out = append(out, data[:rowEnd]...)
		data = data[rowEnd:]
	}
	return out
}

// dropMoveFromRanges removes the cross-structure spans bracketed by
// <w:moveFromRangeStart w:id="N"/> ... <w:moveFromRangeEnd w:id="N"/>
// markers (ECMA-376 Part 1 §17.13.5.18 / §17.13.5.19) when accepting
// revisions. Mirrors upstream Okapi's
// SkippableElements.MoveFromRevisionCrossStructure (lines 371-450 of
// SkippableElements.java) + BlockParser.parse skipped-block handling
// (lines 267-274 of BlockParser.java) + StyledTextPart.process
// dispatch (lines 580-593 + 299-305 of StyledTextPart.java).
//
// Upstream semantics: when moveFromRangeStart is encountered, an
// event-by-event skip walks through the reader until moveFromRangeEnd
// is consumed (inclusive). EVERY event in between — including the
// </w:p>/<w:p> boundaries of any straddled paragraphs and any
// untracked text in those paragraphs — is dropped wholesale. The
// enclosing block (the <w:p> containing moveFromRangeStart) is marked
// skipped(true) by the BlockParser because parentStructureCrossed
// became true during the skip, and StyledTextPart drops it.
//
// At the byte level we mirror this by, for each (moveFromRangeStart,
// moveFromRangeEnd) pair matched by w:id, removing from the start
// tag of the <w:p> that contains moveFromRangeStart through and
// INCLUDING the </w:p> end tag of the <w:p> that contains
// moveFromRangeEnd. Rationale:
//
//   - The paragraph holding moveFromRangeStart is dropped because the
//     BlockParser returns skipped=true (parentStructureCrossed).
//   - All paragraphs strictly between the two markers are consumed by
//     the cross-structure skip (their start/end tags + content all
//     pass through the skip's event loop).
//   - The paragraph holding moveFromRangeEnd is consumed too: by the
//     time the skip exits, the eventReader is positioned past
//     moveFromRangeEnd inside that paragraph; the trailing events
//     (any content between moveFromRangeEnd and </w:p>, plus the
//     </w:p>) are emitted by the outer loop without a paragraph
//     start. In practice for the 843-3* fixtures upstream produces an
//     empty <w:p></w:p> here (the trailing content is itself
//     revision-tracked <w:del>/<w:ins> that auto-accept-revisions
//     erases). Dropping the wrapper paragraph entirely loses that
//     synthetic empty <w:p> shell — but the difference does not
//     affect translatable content, only document-structural skeleton
//     bytes that the XMLCanonical normalizer compares against the
//     reference. The observed delta on 843-3* is small enough that
//     wrapping the byte-level pass with paragraph-end heuristics
//     (rather than full XML parsing) keeps complexity low.
//
// Pairs are matched by w:id attribute value. Unmatched start markers
// (no corresponding end with matching id, or vice versa) are left
// alone — the writer's stripWMLSkippableElements pass strips the
// stray markers. Self-closing markers (always the schema form for
// these elements per ECMA-376 §CT_MarkupRange) and the explicit
// open+empty-close form are both recognised.
func dropMoveFromRanges(data []byte) []byte {
	const startMarker = "<w:moveFromRangeStart"
	const endMarker = "<w:moveFromRangeEnd"
	if !bytes.Contains(data, []byte(startMarker)) {
		return data
	}
	out := make([]byte, 0, len(data))
	cursor := 0
	for cursor < len(data) {
		startIdx := bytes.Index(data[cursor:], []byte(startMarker))
		if startIdx < 0 {
			out = append(out, data[cursor:]...)
			break
		}
		startIdx += cursor
		// Validate element-name boundary: next byte must be `/`,
		// `>`, or whitespace (rules out e.g. <w:moveFromRangeStartX).
		if !isElementNameBoundary(data, startIdx+len(startMarker)) {
			out = append(out, data[cursor:startIdx+len(startMarker)]...)
			cursor = startIdx + len(startMarker)
			continue
		}
		// Find the closing `>` of the moveFromRangeStart element.
		startTagEnd := bytes.IndexByte(data[startIdx:], '>')
		if startTagEnd < 0 {
			out = append(out, data[cursor:]...)
			break
		}
		startTagEnd += startIdx // absolute position of `>`
		// Extract the w:id="N" value from the start marker.
		id := extractWIDAttr(data[startIdx : startTagEnd+1])
		if id == "" {
			// Malformed start marker — pass through unchanged.
			out = append(out, data[cursor:startTagEnd+1]...)
			cursor = startTagEnd + 1
			continue
		}
		// Find the matching <w:moveFromRangeEnd w:id="N"/> after
		// startTagEnd. Iterate end markers and match by w:id value.
		endStart, endTagEnd := findMoveFromRangeEnd(data, startTagEnd+1, id, endMarker)
		if endStart < 0 {
			// No matching end — leave the start marker in place;
			// the writer strips it. Continue from after the start
			// marker so we don't hunt the same location forever.
			out = append(out, data[cursor:startTagEnd+1]...)
			cursor = startTagEnd + 1
			continue
		}
		// Skip pairs whose span crosses a table cell, row, or table
		// boundary. Upstream Okapi handles those cases via the
		// tableRowStructureCrossed / tableStructureCrossed branches
		// (lines 415-426 of SkippableElements.java +
		// removeComponentsFromLastWith(LOCAL_TABLE/LOCAL_TABLE_ROW)
		// in StyledTextPart). The byte-level pre-pass here only
		// implements the parentStructureCrossed (paragraph) case;
		// the row-level case is partially handled by the existing
		// dropMoveFromTableRows pass for moveFrom CONTENT wrappers.
		// Cross-cell ranges (e.g. 1080-1.docx) are left to the
		// streaming parser which would otherwise produce malformed
		// XML if we ate the row/cell boundary tags.
		if spanCrossesTableBoundary(data[startTagEnd+1 : endStart]) {
			out = append(out, data[cursor:startTagEnd+1]...)
			cursor = startTagEnd + 1
			continue
		}
		// Locate the enclosing <w:p> open tag for the start marker
		// (search backwards from startIdx). If startIdx is at body
		// level (not inside any <w:p>), keep startIdx as-is so we
		// only drop from the start marker forward.
		var dropFrom int
		if isInsideParagraph(data, startIdx) {
			pOpenStart := findEnclosingParagraphOpenStart(data, startIdx)
			if pOpenStart < 0 {
				// Defensive: should not happen if isInsideParagraph
				// said yes, but bail safely.
				out = append(out, data[cursor:endTagEnd+1]...)
				cursor = endTagEnd + 1
				continue
			}
			dropFrom = pOpenStart
		} else {
			dropFrom = startIdx
		}
		// Drop endpoint depends on where the end marker sits.
		//
		//   * INSIDE a paragraph: extend the drop through the
		//     enclosing </w:p> end tag, then re-emit a single
		//     synthetic empty <w:p/> in its place. Upstream
		//     BlockParser collapses the cross-structure span into a
		//     single skipped block whose closing tag is the </w:p>
		//     of the last straddled paragraph (lines 267-274 of
		//     BlockParser.java); the empty <w:p/> shell that
		//     remains at the boundary mirrors what upstream emits
		//     verbatim (observed on 843-31/-32 fixtures: a single
		//     `<w:p/>` precedes the trailing <w:sectPr>).
		//
		//   * AT BODY LEVEL (between sibling <w:p> elements, e.g.
		//     843-33/-34 fixtures): drop through the end marker
		//     only so any subsequent sibling paragraph survives
		//     unchanged.
		var dropTo int
		var insertEmptyP bool
		if isInsideParagraph(data, endStart) {
			pCloseEnd := findEnclosingParagraphCloseEnd(data, endTagEnd+1)
			if pCloseEnd < 0 {
				out = append(out, data[cursor:endTagEnd+1]...)
				cursor = endTagEnd + 1
				continue
			}
			dropTo = pCloseEnd
			insertEmptyP = true
		} else {
			dropTo = endTagEnd + 1
		}
		// Drop everything in [dropFrom, dropTo); inject a synthetic
		// empty paragraph if the boundary needs one.
		out = append(out, data[cursor:dropFrom]...)
		if insertEmptyP {
			out = append(out, []byte("<w:p/>")...)
		}
		cursor = dropTo
	}
	return out
}

// isInsideParagraph reports whether the position pos in data falls
// inside an open <w:p>...</w:p> region (i.e. between an unmatched
// <w:p> open tag and its eventual </w:p> close). Linear scan from
// the start of data; suitable for the once-per-call check we need.
func isInsideParagraph(data []byte, pos int) bool {
	const pOpen = "<w:p"
	const pClose = "</w:p>"
	depth := 0
	cursor := 0
	for cursor < pos {
		nextOpen := indexValidElement(data[cursor:pos], pOpen)
		nextClose := bytes.Index(data[cursor:pos], []byte(pClose))
		if nextOpen < 0 && nextClose < 0 {
			return depth > 0
		}
		if nextOpen >= 0 && (nextClose < 0 || nextOpen < nextClose) {
			absOpen := cursor + nextOpen
			tagEnd := bytes.IndexByte(data[absOpen:], '>')
			if tagEnd < 0 {
				return depth > 0
			}
			absOpenEnd := absOpen + tagEnd
			if absOpenEnd > 0 && data[absOpenEnd-1] != '/' {
				depth++
			}
			cursor = absOpenEnd + 1
		} else {
			depth--
			cursor = cursor + nextClose + len(pClose)
		}
	}
	return depth > 0
}

// spanCrossesTableBoundary reports whether the byte slice between a
// moveFromRangeStart and the matching moveFromRangeEnd crosses a
// </w:tc>, </w:tr>, or </w:tbl> end tag without first opening a
// matching <w:tc>, <w:tr>, or <w:tbl> inside the span. A crossing
// would mean my drop would unbalance cell/row/table structure.
func spanCrossesTableBoundary(span []byte) bool {
	for _, name := range []string{"tc", "tr", "tbl"} {
		open := "<w:" + name
		close := "</w:" + name + ">"
		depth := 0
		cursor := 0
		for cursor < len(span) {
			nextOpen := indexValidElement(span[cursor:], open)
			nextClose := bytes.Index(span[cursor:], []byte(close))
			if nextOpen < 0 && nextClose < 0 {
				break
			}
			if nextClose < 0 || (nextOpen >= 0 && nextOpen < nextClose) {
				absOpen := cursor + nextOpen
				tagEnd := bytes.IndexByte(span[absOpen:], '>')
				if tagEnd < 0 {
					break
				}
				absOpenEnd := absOpen + tagEnd
				if absOpenEnd > 0 && span[absOpenEnd-1] != '/' {
					depth++
				}
				cursor = absOpenEnd + 1
				continue
			}
			if depth == 0 {
				return true
			}
			depth--
			cursor = cursor + nextClose + len(close)
		}
	}
	return false
}

// isElementNameBoundary reports whether the byte at position pos in
// data is a valid character that can follow an XML element name (so we
// know we matched the full element name and not a prefix).
func isElementNameBoundary(data []byte, pos int) bool {
	if pos >= len(data) {
		return false
	}
	b := data[pos]
	return b == '>' || b == '/' || b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

// extractWIDAttr extracts the value of the w:id="..." attribute from
// the given element open-tag bytes (including the leading `<` and
// closing `>`). Returns "" if the attribute is absent or malformed.
func extractWIDAttr(tag []byte) string {
	const attr = "w:id="
	idx := bytes.Index(tag, []byte(attr))
	if idx < 0 {
		return ""
	}
	q := idx + len(attr)
	if q >= len(tag) {
		return ""
	}
	quote := tag[q]
	if quote != '"' && quote != '\'' {
		return ""
	}
	end := bytes.IndexByte(tag[q+1:], quote)
	if end < 0 {
		return ""
	}
	return string(tag[q+1 : q+1+end])
}

// findMoveFromRangeEnd searches data from start onward for the next
// <w:moveFromRangeEnd w:id="id" .../> marker. Returns (startIdx,
// endIdx) where startIdx is the position of the `<` and endIdx is the
// position of the closing `>`. Returns (-1, -1) if no matching marker
// is found.
func findMoveFromRangeEnd(data []byte, from int, id, endMarker string) (int, int) {
	cursor := from
	for cursor < len(data) {
		idx := bytes.Index(data[cursor:], []byte(endMarker))
		if idx < 0 {
			return -1, -1
		}
		idx += cursor
		if !isElementNameBoundary(data, idx+len(endMarker)) {
			cursor = idx + len(endMarker)
			continue
		}
		tagEnd := bytes.IndexByte(data[idx:], '>')
		if tagEnd < 0 {
			return -1, -1
		}
		tagEnd += idx
		if extractWIDAttr(data[idx:tagEnd+1]) == id {
			return idx, tagEnd
		}
		cursor = tagEnd + 1
	}
	return -1, -1
}

// findEnclosingParagraphOpenStart searches backwards from pos for the
// nearest `<w:p>` or `<w:p ...>` start tag whose content has not yet
// been closed by a `</w:p>` between the tag and pos. Returns the
// absolute index of the `<` byte, or -1 if pos is not inside any
// paragraph.
func findEnclosingParagraphOpenStart(data []byte, pos int) int {
	const pOpen = "<w:p"
	const pClose = "</w:p>"
	depth := 0
	cursor := pos
	for cursor > 0 {
		// Find the previous occurrence of either <w:p or </w:p>.
		// Search the substring data[:cursor] from the right.
		closeIdx := bytes.LastIndex(data[:cursor], []byte(pClose))
		// For openIdx we need the LAST occurrence of "<w:p" whose
		// boundary char is `>`, `/`, ` `, `\t`, `\n`, `\r` so we
		// don't match <w:pPr or <w:pict, etc.
		openIdx := lastIndexValidElement(data[:cursor], pOpen)
		if openIdx < 0 && closeIdx < 0 {
			return -1
		}
		// Pick the later of the two; that's the next event going
		// backwards.
		if openIdx > closeIdx {
			if depth == 0 {
				return openIdx
			}
			depth--
			cursor = openIdx
		} else {
			depth++
			cursor = closeIdx
		}
	}
	return -1
}

// lastIndexValidElement returns the last index in data where elemName
// appears followed by a valid element-name boundary character. -1 if
// none found.
func lastIndexValidElement(data []byte, elemName string) int {
	cursor := len(data)
	for cursor > 0 {
		idx := bytes.LastIndex(data[:cursor], []byte(elemName))
		if idx < 0 {
			return -1
		}
		if isElementNameBoundary(data, idx+len(elemName)) {
			return idx
		}
		cursor = idx
	}
	return -1
}

// findEnclosingParagraphCloseEnd searches forward from pos for the
// matching `</w:p>` end tag of the enclosing paragraph (depth=0 at
// pos, so we want the first `</w:p>` not preceded by an unmatched
// `<w:p>`). Returns the absolute index ONE PAST the `>` of the end
// tag (so it can be used as a slice upper bound), or -1 if no match.
func findEnclosingParagraphCloseEnd(data []byte, pos int) int {
	const pOpen = "<w:p"
	const pClose = "</w:p>"
	depth := 0
	cursor := pos
	for cursor < len(data) {
		nextOpen := indexValidElement(data[cursor:], pOpen)
		nextClose := bytes.Index(data[cursor:], []byte(pClose))
		if nextClose < 0 {
			return -1
		}
		if nextOpen >= 0 && nextOpen < nextClose {
			// Stepped into a nested paragraph (rare — paragraphs
			// don't nest in document.xml normally, but they can
			// inside textbox/sdt content). Track depth.
			absOpen := cursor + nextOpen
			tagEnd := bytes.IndexByte(data[absOpen:], '>')
			if tagEnd < 0 {
				return -1
			}
			absOpenEnd := absOpen + tagEnd
			if data[absOpenEnd-1] != '/' {
				depth++
			}
			cursor = absOpenEnd + 1
			continue
		}
		if depth == 0 {
			return cursor + nextClose + len(pClose)
		}
		depth--
		cursor = cursor + nextClose + len(pClose)
	}
	return -1
}

// indexValidElement returns the first index in data where elemName
// appears followed by a valid element-name boundary character. -1 if
// none found.
func indexValidElement(data []byte, elemName string) int {
	cursor := 0
	for cursor < len(data) {
		idx := bytes.Index(data[cursor:], []byte(elemName))
		if idx < 0 {
			return -1
		}
		idx += cursor
		if isElementNameBoundary(data, idx+len(elemName)) {
			return idx
		}
		cursor = idx + len(elemName)
	}
	return -1
}

// dropEmptyTables removes every <w:tbl ...>...</w:tbl> region from data
// whose body contains no <w:tr> child element. This complements
// dropMoveFromTableRows: when every row of a table was wrapped in a
// moveFrom revision, removing the rows leaves a structurally empty
// table behind. Upstream Okapi removes these via
// StyledTextPart.process lines 410-424 (the TableEnd branch): if
// delayedTableMarkup has accumulated no translatable block since the
// last <w:tbl>, the entire table-markup component chain is dropped
// via removeComponentsFromLastWith(LOCAL_TABLE).
//
// The pass iterates until fixed-point so that nested tables collapsed
// by an outer-level removal also disappear (a <w:tc> may contain
// another <w:tbl>; if that inner table becomes empty after row drops,
// the outer cell may itself become empty — but cell/row dropping is
// not addressed here, only the strictly-empty table case Okapi
// directly handles).
func dropEmptyTables(data []byte) []byte {
	const tblOpen = "<w:tbl"
	const tblClose = "</w:tbl>"
	if !bytes.Contains(data, []byte(tblOpen)) {
		return data
	}
	out := make([]byte, 0, len(data))
	for {
		idx := bytes.Index(data, []byte(tblOpen))
		if idx < 0 {
			out = append(out, data...)
			break
		}
		// Validate element-name boundary so we don't match <w:tblPr,
		// <w:tblGrid, <w:tblBorders, etc.
		j := idx + len(tblOpen)
		if j >= len(data) {
			out = append(out, data...)
			break
		}
		b := data[j]
		if b != '>' && b != '/' && b != ' ' && b != '\t' && b != '\n' && b != '\r' {
			out = append(out, data[:j+1]...)
			data = data[j+1:]
			continue
		}
		k := bytes.IndexByte(data[j:], '>')
		if k < 0 {
			out = append(out, data...)
			break
		}
		startEnd := j + k
		// Self-closing <w:tbl/> is already empty — drop.
		if startEnd > 0 && data[startEnd-1] == '/' {
			out = append(out, data[:idx]...)
			data = data[startEnd+1:]
			continue
		}
		// Find matching </w:tbl> respecting nested tables.
		bodyStart := startEnd + 1
		depth := 1
		cursor := bodyStart
		for depth > 0 {
			nextOpen := bytes.Index(data[cursor:], []byte(tblOpen))
			nextClose := bytes.Index(data[cursor:], []byte(tblClose))
			if nextClose < 0 {
				out = append(out, data...)
				return out
			}
			if nextOpen >= 0 && nextOpen < nextClose {
				absOpen := cursor + nextOpen
				jj := absOpen + len(tblOpen)
				if jj < len(data) {
					bb := data[jj]
					if bb == '>' || bb == '/' || bb == ' ' || bb == '\t' || bb == '\n' || bb == '\r' {
						kk := bytes.IndexByte(data[jj:], '>')
						if kk < 0 {
							out = append(out, data...)
							return out
						}
						nestedOpenEnd := jj + kk
						if nestedOpenEnd > 0 && data[nestedOpenEnd-1] != '/' {
							depth++
						}
						cursor = nestedOpenEnd + 1
						continue
					}
				}
				cursor = cursor + nextOpen + len(tblOpen)
				continue
			}
			cursor = cursor + nextClose + len(tblClose)
			depth--
		}
		tableEnd := cursor
		body := data[bodyStart : tableEnd-len(tblClose)]
		if !tableBodyHasRow(body) {
			// Empty table — drop the whole region.
			out = append(out, data[:idx]...)
			data = data[tableEnd:]
			continue
		}
		out = append(out, data[:tableEnd]...)
		data = data[tableEnd:]
	}
	return out
}

// tableBodyHasRow reports whether the captured table body contains at
// least one <w:tr> element. The boundary check disambiguates <w:tr from
// <w:trPr/<w:trHeight/<w:trCantSplit etc.
func tableBodyHasRow(body []byte) bool {
	const marker = "<w:tr"
	cursor := 0
	for {
		idx := bytes.Index(body[cursor:], []byte(marker))
		if idx < 0 {
			return false
		}
		j := cursor + idx + len(marker)
		if j >= len(body) {
			return false
		}
		b := body[j]
		if b == '>' || b == '/' || b == ' ' || b == '\t' || b == '\n' || b == '\r' {
			return true
		}
		cursor = j
	}
}

// rowBodyHasMoveFromContent reports whether the captured row body
// contains a <w:moveFrom> revision-tracking content wrapper (ECMA-376
// Part 1 §17.13.5.17 Move From Run Content). The detector explicitly
// disambiguates from <w:moveFromRangeStart and <w:moveFromRangeEnd
// (different element local names) by requiring the next byte after
// `<w:moveFrom` to be a space (attributes follow) or `>`; the wrapper
// form always carries id/author/date attributes per the schema.
func rowBodyHasMoveFromContent(body []byte) bool {
	const marker = "<w:moveFrom"
	cursor := 0
	for {
		idx := bytes.Index(body[cursor:], []byte(marker))
		if idx < 0 {
			return false
		}
		j := cursor + idx + len(marker)
		if j >= len(body) {
			return false
		}
		b := body[j]
		if b == ' ' || b == '\t' || b == '\n' || b == '\r' || b == '>' {
			return true
		}
		cursor = j
	}
}

// handleTableRow processes a <w:tr> start element, deciding whether the
// entire row should be dropped because <w:trPr> carries a <w:del> child
// (revision tracking, ECMA-376 Part 1 §17.13.5.13). When a row-deletion
// marker is found AND AutomaticallyAcceptRevisions is true, the helper
// drains tokens through the matching </w:tr> end and emits no skeleton.
//
// If the row is NOT a deletion candidate, the helper emits the <w:tr>
// start element, any whitespace/comments seen before the first child,
// and then either the <w:trPr> raw bytes (if present) or the first
// non-trPr child (re-dispatched). The caller's outer loop continues
// reading the rest of the row's cell content.
//
// Mirrors upstream Okapi StyledTextPart.process() lines 530-551
// (revisionPropertyTableRowDeletedSkippableElements + delayedTableMarkup
// removal) and lines 515-528
// (revisionPropertyTableRowInsertedSkippableElements drain-only).
func (p *wmlParser) handleTableRow(d *xml.Decoder, start xml.StartElement) error {
	// Peek at the first child token. Per ECMA-376 §17.4.79 (CT_Row),
	// the row's child sequence is tblPrEx? trPr? content* — so trPr
	// is at most the second child. We tolerate an optional tblPrEx
	// preceding it. Whitespace between elements is preserved in the
	// skeleton so we capture it as we go.
	var pending []string // serialised whitespace / comments seen before first child

	emitPending := func() {
		for _, s := range pending {
			p.skelText(s)
		}
	}

	// Drain to matching </w:tr> end without emitting anything.
	skipRowToEnd := func() error {
		depth := 1
		for depth > 0 {
			tok, err := d.Token()
			if err != nil {
				return err
			}
			switch tt := tok.(type) {
			case xml.StartElement:
				if tt.Name.Local == "tr" {
					depth++
				}
			case xml.EndElement:
				if tt.Name.Local == "tr" {
					depth--
				}
			}
		}
		return nil
	}

	for {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		switch tt := tok.(type) {
		case xml.CharData:
			// xml.CharData backing slice is reused by the decoder; copy via string().
			pending = append(pending, xmlEscape(string(tt)))
		case xml.Comment:
			// xml.Comment backing slice is reused by the decoder; copy via string().
			pending = append(pending, "<!--"+string(tt)+"-->")
		case xml.StartElement:
			// Found the first child element.
			if tt.Name.Local == "trPr" {
				// Capture raw and inspect for a top-level <w:del> child.
				raw, err := captureRawElement(d, tt)
				if err != nil {
					return err
				}
				if trPrHasRowDeletion(raw) {
					// Drain the rest of the row and emit nothing.
					return skipRowToEnd()
				}
				// Not a deleted row — emit row start, any pending
				// whitespace/comments, then the trPr raw. Caller
				// continues normal processing for the rest of the row.
				p.skelWriteStartElement(start)
				emitPending()
				p.skelText(raw)
				return nil
			}
			// First child wasn't trPr — could be tblPrEx or a content
			// cell (no row-property block at all). Either way, the
			// row carries no row-revision marker; emit row start, any
			// pending whitespace, the child start element, then
			// hand back to the outer loop.
			p.skelWriteStartElement(start)
			emitPending()
			return p.dispatchInRow(d, tt)
		case xml.EndElement:
			// Empty row (no children at all). Emit row start and
			// row end, return — caller continues.
			p.skelWriteStartElement(start)
			emitPending()
			p.skelWriteEndElement(tt)
			return nil
		}
	}
}

// dispatchInRow forwards a start element seen as the first non-trPr
// child of <w:tr> to the appropriate parsePart handler. Mirrors the
// switch in parsePart for the elements that legitimately appear inside
// a row (typically <w:tc> via the default branch, or another
// <w:trPr>-less child).
func (p *wmlParser) dispatchInRow(d *xml.Decoder, t xml.StartElement) error {
	switch t.Name.Local {
	case "tcPr":
		raw, err := captureRawElement(d, t)
		if err != nil {
			return err
		}
		p.skelText(raw)
	default:
		p.skelWriteStartElement(t)
	}
	return nil
}

// trPrHasRowDeletion reports whether raw (the captured XML of a
// <w:trPr> element) contains a top-level <w:del> child — the row
// deletion revision marker per ECMA-376 Part 1 §17.13.5.13. Top-level
// is determined by a single-element-deep scan: the marker appears as
// a direct child of <w:trPr>, not inside any nested element. The
// scan tolerates whitespace, attribute variations, and self-closing
// or open/close empty forms.
//
// Mirrors upstream Okapi's
// SkippableElement.RevisionProperty.TABLE_ROW_DELETED entry
// (SkippableElement.java line 245) keyed on QName "del" with
// parent QName "trPr" via
// SkippableElements.RevisionProperty.CONTEXT_AWARE_REVISION_SKIPPABLE_ELEMENTS
// (SkippableElements.java line 528-531).
func trPrHasRowDeletion(raw string) bool {
	// Strip the outer <w:trPr ...> and </w:trPr> wrapper, then scan
	// only the immediate-child layer for <w:del. We use a simple
	// depth tracker since the trPr content is small (revision
	// markers, height, cantSplit, etc.) and rarely deeply nested.
	dec := xml.NewDecoder(strings.NewReader(raw))
	depth := 0
	for {
		tok, err := dec.Token()
		if err != nil {
			return false
		}
		switch tt := tok.(type) {
		case xml.StartElement:
			depth++
			if depth == 2 && tt.Name.Local == "del" {
				return true
			}
		case xml.EndElement:
			depth--
			if depth == 0 {
				return false
			}
		}
	}
}

// parseParagraph parses a <w:p> element and emits a Block if it contains text.
func (p *wmlParser) parseParagraph(d *xml.Decoder, partPath string, emitBlock func(*model.Block)) error {
	var runs []textRun
	var hyperlinkRuns []textRun
	var inHyperlink bool
	var hyperlinkID string
	var paraProps string
	var paraStyleID string
	var cfs complexFieldState
	var bms bookmarkSkipState

	for {
		tok, err := d.Token()
		if err != nil {
			return err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "pPr":
				// Capture paragraph properties for skeleton, extracting pStyle if present
				raw, styleID, err := captureParaProps(d, t)
				if err != nil {
					return err
				}
				paraProps = raw
				paraStyleID = styleID

			case "r":
				// Text run — may contain fldChar/instrText for complex
				// fields. parseRunWithFieldState collapses such runs to
				// a single SubTypeFieldChar sentinel carrying the raw
				// <w:r>...</w:r>; surface them through the field-aware
				// keep/drop logic below.
				rawStart := startElementToRaw(t)
				run, err := p.parseRunWithFieldState(d, &cfs, rawStart)
				if err != nil {
					return err
				}
				run = filterFieldRuns(run, &cfs)
				// If we're inside a non-extractable complex field, drop
				// any plain text runs (the field-markup sentinel runs
				// have already been retained by filterFieldRuns); only
				// the cached display text from non-extractable fields is
				// suppressed per upstream Okapi
				// (RunParser.parseComplexField, lines 501-506).
				if cfs.active && !cfs.extractable {
					run = dropTextRuns(run)
				}
				// If we're inside an extractable field but before the
				// separator, drop translatable text but keep field
				// markup (begin / instrText / separate sentinels).
				if cfs.active && cfs.extractable && !cfs.atResult {
					run = dropTextRuns(run)
				}
				if len(run) == 0 {
					continue
				}
				if inHyperlink {
					hyperlinkRuns = append(hyperlinkRuns, run...)
				} else {
					runs = append(runs, run...)
				}

			case "hyperlink":
				inHyperlink = true
				hyperlinkID = attrVal(t, "id")
				hyperlinkRuns = nil

			case "bookmarkStart", "bookmarkEnd":
				// Bookmarks are direct children of <w:p> per ECMA-376
				// Part 1 §17.13.6 (Bookmarks). They are cross-structure
				// markers that delimit a named range; the markers can
				// span runs, paragraphs, tables, and even sections, so
				// they must be preserved verbatim at the position they
				// appear in the source.
				//
				// Mirrors upstream Okapi
				// SkippableElements.BookmarkCrossStructure
				// (SkippableElements.java lines 300-331) and
				// BlockSkippableElements.skip (BlockSkippableElements.java
				// lines 116-121): the `_GoBack` bookmark — Word's auto-
				// generated "return-to-last-edit" bookmark — is
				// silently skipped (start AND its matching end by id),
				// every other bookmark falls through to be added as
				// inline markup on the block.
				bookmark, captured, err := p.captureBookmark(d, t, &bms)
				if err != nil {
					return err
				}
				if !captured {
					continue
				}
				if inHyperlink {
					hyperlinkRuns = append(hyperlinkRuns, bookmark)
				} else {
					runs = append(runs, bookmark)
				}

			case "proofErr", "commentRangeStart", "commentRangeEnd",
				"permStart", "permEnd":
				if err := skipElement(d); err != nil {
					return err
				}

			case "sdt":
				// Inline structured document tag — recurse
				sdtRuns, err := p.parseInlineSDT(d)
				if err != nil {
					return err
				}
				runs = append(runs, sdtRuns...)

			case "ins", "moveTo":
				// Revision-tracking content wrapper: insertion / move-to.
				// Mirrors okapi's SkippableElements.RevisionInline.skip
				// (lines 209-212 of okapi/filters/openxml/src/main/java/
				// net/sf/okapi/filters/openxml/SkippableElements.java)
				// which returns early without skipping for INSERTED_CONTENT
				// and MOVED_CONTENT_TO — i.e. the wrapper is unwrapped and
				// its child runs are kept (the auto-accept-revisions
				// default semantics: insertions are accepted into the
				// final document).
				//
				// Process child <w:r> runs as if they were direct
				// children of <w:p> by handing them off to the run
				// parser inline.
				if err := p.parseRevisionInsertion(d, t.Name.Local, &runs, &cfs); err != nil {
					return err
				}

			case "del", "moveFrom":
				// Revision-tracking content wrapper: deletion / move-from.
				// Auto-accept-revisions drops the entire subtree (deleted
				// content is removed from the final document). Per
				// SkippableElements.RevisionInline at lines 213-214 of
				// SkippableElements.java this falls through to the default
				// skip path. The skipElement walker discards the subtree
				// entirely, including any nested <w:r><w:delText>...
				// </w:delText></w:r> runs.
				if err := skipElement(d); err != nil {
					return err
				}

			case "oMathPara", "oMath":
				// Math content (Office Math Markup Language, OMML —
				// ECMA-376 Part 1 §22.1). Word may emit <m:oMathPara>
				// or <m:oMath> as a direct child of <w:p>, not wrapped
				// in <w:r>. Okapi's MathSymbol / MathBlock parsers
				// preserve the entire OMML subtree opaquely — text
				// inside m:t is mathematical typography, not natural
				// language — so we capture the raw XML as a sentinel
				// run (TypeImage) so the writer round-trips the
				// equation byte-for-byte. equation.docx is the
				// canonical fixture.
				raw, err := captureRawElement(d, t)
				if err != nil {
					return err
				}
				runs = append(runs, textRun{text: "", data: raw})

			case "AlternateContent":
				// Paragraph-level mc:AlternateContent (rare but legal:
				// some authoring tools emit it as a <w:p> child rather
				// than a <w:r> child). Same MCE semantics as the
				// run-level handler — keep the wrapper + selected
				// Choice, drop Fallback. ECMA-376 Part 3 §10. See
				// captureAlternateContent for citations. Tagged with the
				// paragraph-level sentinel  so runToXML emits it
				// without wrapping in <w:r>.
				raw, err := captureAlternateContent(d, t)
				if err != nil {
					return err
				}
				runs = append(runs, textRun{text: "", data: raw})

			case "fldSimple":
				// Simple field — `<w:fldSimple w:instr="...">...</
				// w:fldSimple>` per ECMA-376 Part 1 §17.16.6. Per
				// upstream Okapi the entire fldSimple element is
				// gathered and flushed as a single opaque markup chunk
				// (BlockParser.parse lines 242-250 of okapi/filters/
				// openxml/src/main/java/net/sf/okapi/filters/openxml/
				// BlockParser.java); nothing inside is treated as
				// translatable. Mirror that here: capture the whole
				// element raw and hand it to the block as a
				// SubTypeFieldSimple sentinel so the writer emits it
				// verbatim with no modifications.
				raw, err := captureRawElement(d, t)
				if err != nil {
					return err
				}
				// Protect every nested <w:rPr> inside the captured
				// payload from the writer's stripWMLSkippableElements
				// pass: Okapi's BlockParser routes fldSimple through
				// the gather-events-into-markup path (lines 242-250 of
				// okapi/filters/openxml/src/main/java/net/sf/okapi/
				// filters/openxml/BlockParser.java) which preserves the
				// inner runs verbatim — no skippable-element stripping
				// applied. So inner rPrs that carry only `<w:noProof/>`
				// (e.g. AUTHOR cached-result run in Document-with-
				// formula-and-tabs.docx) need to round-trip with the
				// noProof intact, not stripped + empty-rPr-collapsed.
				raw = protectFieldPayloadFromStripping(raw)
				if inHyperlink {
					hyperlinkRuns = append(hyperlinkRuns, textRun{text: ":fldSimple", data: raw})
				} else {
					runs = append(runs, textRun{text: ":fldSimple", data: raw})
				}

			default:
				if err := skipElement(d); err != nil {
					return err
				}
			}

		case xml.EndElement:
			if t.Name.Local == "hyperlink" {
				if inHyperlink && len(hyperlinkRuns) > 0 {
					runs = append(runs, p.wrapHyperlinkRuns(hyperlinkRuns, hyperlinkID)...)
				}
				inHyperlink = false
				hyperlinkID = ""
				continue
			}

			if t.Name.Local == "p" {
				// Apply style optimization: subtract inherited properties
				if p.styles != nil && paraStyleID != "" {
					styleProps := p.styles.resolveProps(paraStyleID)
					for i := range runs {
						if !isSentinel(runs[i].text) {
							subtractProps(&runs[i].props, styleProps)
						}
					}
				}

				// Apply font mapping: normalize font names to script groups for merging
				if len(p.cfg.FontMappings) > 0 {
					for i := range runs {
						if runs[i].props.fontName != "" {
							if group, ok := p.cfg.FontMappings[runs[i].props.fontName]; ok {
								runs[i].props.fontName = group
							}
						}
					}
				}

				// Compute the per-paragraph common rPr children BEFORE
				// mergeRuns collapses adjacent runs. mergeRuns drops the
				// rPrChildren of merged-away neighbours (it only keeps
				// the first run's props), so the intersection must be
				// taken across the original source runs.
				//
				// commonRPrChildren mirrors upstream Okapi
				// StyleOptimisation.commonRunPropertiesOf
				// (StyleOptimisation.java lines 204-237) — the set of
				// rPr child elements present and equal across every
				// translatable text run in the paragraph. The writer
				// emits these on every <w:r> for the block (#592), and
				// the WSO post-pass then lifts them into a synthesised
				// paragraph style when the threshold conditions are
				// met (#589 / style_optimization.go).
				commonRPr := commonRPrChildren(runs)
				commonRPrXML := joinRPrChildren(commonRPr)
				// Capture per-text-run rPr fragments BEFORE
				// mergeRuns collapses adjacent runs (mergeRuns
				// drops the rPrChildren of the merged-away
				// neighbours by keeping only the first run's
				// props). Phase 1 only stashes the sidecar on the
				// block; the writer wire-up that consumes it
				// (Phase 2) lands separately. See PARITY_NOTES.md
				// "1083-*" per-run rPr investigation.
				perRunRPrXML := perRunRPrFragments(runs)

				// Merge adjacent runs with same formatting
				merged := mergeRuns(runs)

				// Pre-extract translatable bits from any drawing
				// sentinel runs in this paragraph so they reach
				// the translation pipeline regardless of which
				// writer path handles the run later (the empty-
				// paragraph skeleton flush in writeDrawingXMLToSkel
				// already extracted, but the build-block path
				// below dumps Ph.Data verbatim through the
				// renderBlock TypeImage handler — without this
				// pre-extraction step, drawings inside paragraphs
				// that ALSO contain translatable text never get
				// their textbox/textpath content translated, e.g.
				// TextBoxes.docx and OutOfTheTextBox.docx).
				for i := range merged {
					if isDrawingSentinel(merged[i].text) && merged[i].data != "" {
						merged[i].data = p.extractDrawingTranslations(merged[i].data, partPath, emitBlock)
					}
				}

				// Skip empty paragraphs. A "non-translatable but
				// non-empty" paragraph (one whose only runs are
				// drawing/pict/object sentinels) still needs its
				// runs flushed to the skeleton so the embedded
				// markup survives the round-trip — losing
				// <w:drawing> here is the bug fixed in #590.
				if isEmptyRuns(merged) {
					p.skelWriteString("<w:p>")
					if paraProps != "" {
						p.skelText(paraProps)
					}
					for _, r := range merged {
						p.writeRunToSkel(r, partPath, emitBlock)
					}
					p.skelWriteString("</w:p>")
					return nil
				}

				// Skip hidden text unless configured
				if !p.cfg.TranslateHiddenText && allHidden(merged) {
					p.skelWriteString("<w:p>")
					if paraProps != "" {
						p.skelText(paraProps)
					}
					// Write runs as skeleton text
					for _, r := range merged {
						p.skelText(runToXML(r))
					}
					p.skelWriteString("</w:p>")
					return nil
				}

				// Build block
				*p.blockCounter++
				blockID := fmt.Sprintf("tu%d", *p.blockCounter)

				// Skeleton: write paragraph open, props, ref, close
				p.skelWriteString("<w:p>")
				if paraProps != "" {
					p.skelText(paraProps)
				}
				p.skelRef(blockID)
				p.skelWriteString("</w:p>")

				block := p.buildBlock(blockID, merged, partPath, commonRPrXML, perRunRPrXML)
				emitBlock(block)
				return nil
			}
		}
	}
}

// parseRevisionInsertion drains the children of a <w:ins> or <w:moveTo>
// content wrapper that appears at paragraph level, appending any <w:r>
// runs found inside to the caller's run list. The wrapper element is
// effectively unwrapped — children are kept, the wrapper itself is
// dropped — to mirror okapi's auto-accept-revisions semantics for
// inserted/moved-in content.
//
// The local name passed in (`ins` or `moveTo`) lets the function know
// when to stop draining (matching close tag).
//
// Nested <w:ins>/<w:moveTo> inside the wrapper are handled recursively.
// Nested <w:del>/<w:moveFrom> inside the wrapper are skipped (their
// content is "deletion-of-an-insertion", which auto-accept treats as
// removal — same end state as if the deletion was direct).
func (p *wmlParser) parseRevisionInsertion(d *xml.Decoder, wrapperName string, runs *[]textRun, cfs *complexFieldState) error {
	for {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "r":
				rawStart := startElementToRaw(t)
				run, err := p.parseRunWithFieldState(d, cfs, rawStart)
				if err != nil {
					return err
				}
				run = filterFieldRuns(run, cfs)
				if cfs.active && !cfs.extractable {
					run = dropTextRuns(run)
				}
				if cfs.active && cfs.extractable && !cfs.atResult {
					run = dropTextRuns(run)
				}
				if len(run) == 0 {
					continue
				}
				*runs = append(*runs, run...)
			case "ins", "moveTo":
				if err := p.parseRevisionInsertion(d, t.Name.Local, runs, cfs); err != nil {
					return err
				}
			case "del", "moveFrom":
				if err := skipElement(d); err != nil {
					return err
				}
			default:
				// Unknown content (bookmarks, sdt, hyperlinks, etc. —
				// rare inside revision wrappers in practice). Skip the
				// subtree to mirror parseParagraph's default fallback;
				// future fixtures can extend this case if needed.
				if err := skipElement(d); err != nil {
					return err
				}
			}
		case xml.EndElement:
			if t.Name.Local == wrapperName {
				return nil
			}
		}
	}
}

// parseRunWithFieldState parses a <w:r> element while tracking complex field state.
// It delegates to parseRun for content extraction, but handles fldChar and instrText
// to maintain the field state machine across runs within a paragraph.
//
// When the run carries field markup (fldChar begin/separate/end or
// instrText), the *entire* <w:r> — rPr, all children, end tag — is also
// captured raw and returned as a SubTypeFieldChar sentinel run so the
// writer can round-trip the markup verbatim. This mirrors upstream
// Okapi's RunParser.parseComplexField behaviour (lines 461-542 of
// okapi/filters/openxml/src/main/java/net/sf/okapi/filters/openxml/
// RunParser.java) which routes fldChar/instrText runs through
// runBuilder.addToMarkup so they survive on the block as opaque markup
// chunks regardless of whether the field code is in
// ConditionalParameters.tsComplexFieldDefinitionsToExtract.
//
// rawStart is the raw XML form of the <w:r> start tag (including the
// open angle bracket and attributes) produced by the caller via
// startElementToString. The function appends children verbatim to a
// raw buffer alongside parsing them for content; if any child triggers
// the field-markup path, the assembled raw block is returned as the
// sentinel run's data field. Otherwise the raw buffer is discarded.
func (p *wmlParser) parseRunWithFieldState(d *xml.Decoder, cfs *complexFieldState, rawStart string) ([]textRun, error) {
	var props runProps
	var runs []textRun
	hasProps := false

	// rawBuf accumulates the verbatim XML serialisation of the run as
	// we decode it, so we can hand back an opaque copy when fldChar /
	// instrText is detected. Initialised lazily on first need; backLog
	// holds any post-<w:r> content already consumed before raw capture
	// engaged (e.g. an rPr that precedes the fldChar in document order
	// — `<w:r><w:rPr><w:b/></w:rPr><w:fldChar .../></w:r>` is the
	// canonical shape in 768.docx). Without backLog the rPr would be
	// dropped from the captured payload and the field-marker run would
	// emit without its source rPr.
	var rawBuf strings.Builder
	var rawCaptured bool
	var hasFieldMarkup bool
	var backLog strings.Builder
	startRawCapture := func() {
		if rawCaptured {
			return
		}
		rawBuf.WriteString(rawStart)
		if backLog.Len() > 0 {
			rawBuf.WriteString(backLog.String())
			backLog.Reset()
		}
		rawCaptured = true
	}
	// emitRaw appends s to rawBuf when raw capture is active, otherwise
	// holds it in backLog so a later startRawCapture() can replay any
	// pre-trigger content (rPr that precedes the field marker, etc.).
	emitRaw := func(s string) {
		if rawCaptured {
			rawBuf.WriteString(s)
		} else {
			backLog.WriteString(s)
		}
	}
	// When the caller is already inside an active complex field whose
	// content is being preserved verbatim — i.e. between begin and end
	// for any non-extractable field, or between begin and separate for
	// any field — every run in that span is opaque markup per upstream
	// Okapi (RunParser.parseComplexField, lines 501-506: events route
	// to runBuilder.addToMarkup unless extractable && atResult). Engage
	// raw capture eagerly so display-text runs lacking fldChar /
	// instrText (e.g. the cached `<w:r><w:rPr><w:noProof/></w:rPr>
	// <w:t>I am a textfield.</w:t></w:r>` between separate and end in
	// Textfield.docx) survive the round-trip with their rPr intact.
	if cfs.active && (!cfs.extractable || !cfs.atResult) {
		startRawCapture()
		hasFieldMarkup = true
	}

	for {
		tok, err := d.Token()
		if err != nil {
			return nil, err
		}

		// When raw capture is active, mirror the token verbatim into
		// rawBuf alongside whatever specialised handling the switch
		// performs below. The handlers themselves call into helpers
		// (readCharData, parseRunProps, skipElement, captureRawElement)
		// that consume tokens from d *without* re-emitting them, so the
		// raw mirror has to be set up before each consumer call.
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "rPr":
				hasProps = true
				// Capture rPr raw before consuming its tokens so we can
				// preserve the run's run-properties verbatim on opaque
				// emission. parseRunProps drains through the matching
				// </w:rPr> via skipElement, so without pre-capture the
				// raw buffer would lose the rPr subtree entirely.
				rPrRaw, err := captureRawElement(d, t)
				if err != nil {
					return nil, err
				}
				// Pre-strip noProof / lang / rPrChange / etc. from the
				// captured rPr to mirror upstream Okapi
				// RunSkippableElements (lines 50-62 of okapi/filters/
				// openxml/src/main/java/net/sf/okapi/filters/openxml/
				// RunSkippableElements.java).
				stripped := stripFieldRPrSkippables(rPrRaw)
				// rPr policy on the field-markup capture path mirrors
				// the upstream RunParser flow:
				//   - When raw capture is already engaged (i.e. this
				//     run is an interior field-content run, e.g. a
				//     <w:rPr><w:noProof/></w:rPr> on a cached display
				//     text run inside an active complex field) the
				//     stripped rPr — even if empty — is included in
				//     the opaque payload. Okapi's RunParser drops the
				//     containing run into runBuilder.addToMarkup
				//     verbatim (RunParser.parseComplexField lines
				//     501-506) so the empty <w:rPr/> survives the
				//     round-trip (Textfield.docx is the canonical
				//     fixture).
				//   - When raw capture has not yet engaged (this run
				//     is the entry-point of the field, i.e. carries
				//     the begin / instrText / separate / end marker
				//     and the rPr appears in document order BEFORE
				//     the marker), only stash the rPr in backLog if
				//     stripping leaves a non-empty body. Okapi's
				//     RunParser routes the entry-point run's rPr
				//     through parseRunPropertiesAndRunStyle (line
				//     159) and ultimately through
				//     RunProperties.Default.getEvents (line 580 of
				//     RunProperties.java) which returns an empty
				//     event list for empty properties — so the rPr
				//     wrapper is dropped from the output entirely
				//     when nothing remains after stripping. The
				//     768.docx HYPERLINK fixtures rely on the
				//     non-empty branch (rPr carries <w:b/>); the
				//     ComplexTextfield.docx IF-begin run relies on
				//     the empty branch (rPr only had <w:lang/>).
				if rawCaptured {
					emitRaw(stripped)
				} else if !isStrippedRPrEmpty(stripped) {
					emitRaw(stripped)
				}
				// Re-parse the captured rPr for typed properties.
				props, err = parseRunPropsFromRaw(rPrRaw, p.cfg.AggressiveCleanup)
				if err != nil {
					return nil, err
				}

			case "fldChar":
				hasFieldMarkup = true
				startRawCapture()
				// Mirror the fldChar element raw (including its ffData
				// subtree if present, e.g. Textfield.docx) into the
				// buffer.
				fldRaw, err := captureRawElement(d, t)
				if err != nil {
					return nil, err
				}
				rawBuf.WriteString(fldRaw)
				// Complex field state machine transition
				fldCharType := attrVal(t, "fldCharType")
				switch fldCharType {
				case "begin":
					cfs.nestingLevel++
					if cfs.nestingLevel == 1 {
						cfs.active = true
						cfs.fieldCode = ""
						cfs.extractable = false
						cfs.atResult = false
					}
				case "separate":
					if cfs.nestingLevel == 1 {
						cfs.atResult = true
					}
				case "end":
					cfs.nestingLevel--
					if cfs.nestingLevel <= 0 {
						cfs.active = false
						cfs.fieldCode = ""
						cfs.extractable = false
						cfs.atResult = false
						cfs.nestingLevel = 0
					}
				}

			case "instrText":
				hasFieldMarkup = true
				startRawCapture()
				// Mirror the instrText element raw, preserving the
				// xml:space="preserve" attribute that field codes
				// commonly carry (e.g. ` PAGE \* MERGEFORMAT `).
				rawBuf.WriteString("<")
				writeElementName(&rawBuf, t.Name)
				for _, a := range t.Attr {
					rawBuf.WriteString(" ")
					writeAttrName(&rawBuf, a.Name)
					rawBuf.WriteString(`="`)
					rawBuf.WriteString(xmlEscapeAttr(a.Value))
					rawBuf.WriteString(`"`)
				}
				rawBuf.WriteString(">")
				// Field instruction text — extract the field code name
				text, err := readCharData(d)
				if err != nil {
					return nil, err
				}
				rawBuf.WriteString(xmlEscape(text))
				rawBuf.WriteString("</")
				writeElementName(&rawBuf, t.Name)
				rawBuf.WriteString(">")
				if cfs.active && cfs.nestingLevel == 1 && cfs.fieldCode == "" {
					cfs.fieldCode = complexFieldCodeName(text)
					cfs.extractable = p.isExtractableField(cfs.fieldCode)
				}

			case "t":
				// Capture <w:t ...> open tag verbatim into rawBuf
				// before draining its char data, so opaque emission
				// preserves the text exactly as authored (including
				// xml:space="preserve" when present).
				if rawCaptured {
					rawBuf.WriteString("<")
					writeElementName(&rawBuf, t.Name)
					for _, a := range t.Attr {
						rawBuf.WriteString(" ")
						writeAttrName(&rawBuf, a.Name)
						rawBuf.WriteString(`="`)
						rawBuf.WriteString(xmlEscapeAttr(a.Value))
						rawBuf.WriteString(`"`)
					}
					rawBuf.WriteString(">")
				}
				text, err := readCharData(d)
				if err != nil {
					return nil, err
				}
				if rawCaptured {
					rawBuf.WriteString(xmlEscape(text))
					rawBuf.WriteString("</")
					writeElementName(&rawBuf, t.Name)
					rawBuf.WriteString(">")
				}
				_ = hasProps
				runs = append(runs, textRun{text: text, props: props})

			case "br":
				if rawCaptured {
					rawBuf.WriteString("<")
					writeElementName(&rawBuf, t.Name)
					for _, a := range t.Attr {
						rawBuf.WriteString(" ")
						writeAttrName(&rawBuf, a.Name)
						rawBuf.WriteString(`="`)
						rawBuf.WriteString(xmlEscapeAttr(a.Value))
						rawBuf.WriteString(`"`)
					}
					rawBuf.WriteString("/>")
				}
				runs = append(runs, textRun{
					text:  "\n",
					props: runProps{}, // break has no formatting
				})
				if err := skipElement(d); err != nil {
					return nil, err
				}

			case "tab":
				if rawCaptured {
					rawBuf.WriteString("<")
					writeElementName(&rawBuf, t.Name)
					rawBuf.WriteString("/>")
				}
				if p.cfg.TabAsCharacter {
					runs = append(runs, textRun{text: "\t", props: props})
				} else {
					runs = append(runs, textRun{text: "\uE100", props: props}) // sentinel
				}
				if err := skipElement(d); err != nil {
					return nil, err
				}

			case "drawing", "pict", "object":
				// Capture the full element verbatim so the writer can
				// restore the original markup (drawings, OLE objects,
				// pictures with VML/DrawingML are opaque to the
				// translator but must round-trip byte-equivalently).
				raw, err := captureRawElement(d, t)
				if err != nil {
					return nil, err
				}
				if rawCaptured {
					rawBuf.WriteString(raw)
				}
				runs = append(runs, textRun{text: "\uE101", props: props, data: raw}) // image sentinel

			case "AlternateContent":
				// Markup Compatibility (ECMA-376 Part 3 / ISO/IEC
				// 29500-3 \u00A710): mc:AlternateContent wraps one or more
				// mc:Choice branches plus an optional mc:Fallback.
				// The processor selects the first Choice whose
				// Requires namespaces are all understood, otherwise
				// the Fallback. Okapi unconditionally selects Choice
				// and drops Fallback \u2014 see
				// SkippableElement.GeneralInline.ALTERNATE_CONTENT_FALLBACK
				// (line 56 of okapi/filters/openxml/src/main/java/
				// net/sf/okapi/filters/openxml/SkippableElement.java)
				// wired into RunSkippableElements (lines 45-49 and
				// 93-105 of okapi/filters/openxml/src/main/java/
				// net/sf/okapi/filters/openxml/RunSkippableElements.java).
				// The wrapper itself (mc:AlternateContent + mc:Choice)
				// stays in the output verbatim; the gold fixture
				// gold/parts/block/document-alternate-content.xml
				// shows mc:AlternateContent>mc:Choice surviving
				// round-trip with Fallback stripped. Mirror that here.
				raw, err := captureAlternateContent(d, t)
				if err != nil {
					return nil, err
				}
				if rawCaptured {
					rawBuf.WriteString(raw)
				}
				runs = append(runs, textRun{text: "\uE101", props: props, data: raw})

			case "footnoteReference", "endnoteReference":
				noteID := attrVal(t, "id")
				if rawCaptured {
					rawBuf.WriteString("<")
					writeElementName(&rawBuf, t.Name)
					for _, a := range t.Attr {
						rawBuf.WriteString(" ")
						writeAttrName(&rawBuf, a.Name)
						rawBuf.WriteString(`="`)
						rawBuf.WriteString(xmlEscapeAttr(a.Value))
						rawBuf.WriteString(`"`)
					}
					rawBuf.WriteString("/>")
				}
				runs = append(runs, textRun{text: "\uE102:" + noteID, props: props}) // footnote sentinel
				if err := skipElement(d); err != nil {
					return nil, err
				}

			case "sym":
				char := attrVal(t, "char")
				if rawCaptured {
					rawBuf.WriteString("<")
					writeElementName(&rawBuf, t.Name)
					for _, a := range t.Attr {
						rawBuf.WriteString(" ")
						writeAttrName(&rawBuf, a.Name)
						rawBuf.WriteString(`="`)
						rawBuf.WriteString(xmlEscapeAttr(a.Value))
						rawBuf.WriteString(`"`)
					}
					rawBuf.WriteString("/>")
				}
				if char != "" {
					runs = append(runs, textRun{text: "[sym:" + char + "]", props: props})
				}
				if err := skipElement(d); err != nil {
					return nil, err
				}

			default:
				// Unknown / unsupported child element. Mirror raw if
				// we're already capturing \u2014 losing it on the opaque
				// path would corrupt the field markup.
				if rawCaptured {
					raw, err := captureRawElement(d, t)
					if err != nil {
						return nil, err
					}
					rawBuf.WriteString(raw)
				} else {
					if err := skipElement(d); err != nil {
						return nil, err
					}
				}
			}

		case xml.EndElement:
			if t.Name.Local == "r" {
				if hasFieldMarkup {
					rawBuf.WriteString("</")
					writeElementName(&rawBuf, t.Name)
					rawBuf.WriteString(">")
					// Replace any decoded child-runs with a single
					// SubTypeFieldChar sentinel carrying the verbatim
					// <w:r>...</w:r> payload so the writer can emit it
					// untouched. parseRunPropsFromRaw still populated
					// `props` so the run participates correctly in
					// downstream merging / common-rPr computation, but
					// the payload itself is opaque.
					return []textRun{{
						text:  "\uE108:fldChar",
						props: props,
						data:  rawBuf.String(),
					}}, nil
				}
				return runs, nil
			}
		}
	}
}

// complexFieldCodeName extracts the field code name (first word) from instrText content.
// e.g., ` HYPERLINK "http://example.com" \t "_blank" ` → "HYPERLINK"
func complexFieldCodeName(instrText string) string {
	s := strings.TrimSpace(instrText)
	if idx := strings.IndexByte(s, ' '); idx > 0 {
		return s[:idx]
	}
	return s
}

// isExtractableField returns true if the field code is in the configured extract list.
func (p *wmlParser) isExtractableField(fieldCode string) bool {
	for _, prefix := range p.cfg.ComplexFieldDefinitionsToExtract {
		if strings.EqualFold(fieldCode, prefix) {
			return true
		}
	}
	return false
}

// parseSDT parses a structured document tag, extracting its content.
func (p *wmlParser) parseSDT(d *xml.Decoder, partPath string, emitBlock func(*model.Block), emitData func()) error {
	depth := 1
	inContent := false

	for depth > 0 {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			switch t.Name.Local {
			case "sdtContent":
				inContent = true
			case "sdtPr":
				// Skip SDT properties
				if err := skipElement(d); err != nil {
					return err
				}
				depth--
			case "p":
				if inContent {
					if err := p.parseParagraph(d, partPath, emitBlock); err != nil {
						return err
					}
					depth--
				}
			}
		case xml.EndElement:
			depth--
			if t.Name.Local == "sdtContent" {
				inContent = false
			}
		}
	}
	return nil
}

// parseInlineSDT parses an inline SDT and returns its text runs.
func (p *wmlParser) parseInlineSDT(d *xml.Decoder) ([]textRun, error) {
	var runs []textRun
	depth := 1

	for depth > 0 {
		tok, err := d.Token()
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			switch t.Name.Local {
			case "sdtPr":
				if err := skipElement(d); err != nil {
					return nil, err
				}
				depth--
			case "r":
				// SDT runs don't track complex field state — use a throwaway state
				var cfs complexFieldState
				rawStart := startElementToRaw(t)
				r, err := p.parseRunWithFieldState(d, &cfs, rawStart)
				if err != nil {
					return nil, err
				}
				runs = append(runs, r...)
				depth--
			}
		case xml.EndElement:
			depth--
		}
	}
	return runs, nil
}

// wrapHyperlinkRuns wraps runs in hyperlink opening/closing markers.
func (p *wmlParser) wrapHyperlinkRuns(runs []textRun, relID string) []textRun {
	// Resolve the hyperlink URL from relationships
	url := ""
	if rel, ok := p.rels[relID]; ok {
		url = rel.Target
	}

	data := "<w:hyperlink>"
	if url != "" {
		data = fmt.Sprintf(`<w:hyperlink r:id="%s" href="%s">`, xmlEscapeAttr(relID), xmlEscapeAttr(url))
	}

	// Create wrapper with sentinel markers
	var result []textRun
	result = append(result, textRun{text: "\uE103:" + data, props: runProps{}}) // hyperlink open sentinel
	result = append(result, runs...)
	result = append(result, textRun{text: "\uE104:" + data, props: runProps{}}) // hyperlink close sentinel
	return result
}

// buildBlock builds a model.Block from a list of merged text runs.
//
// commonRPrXML is the children-only serialisation of the rPr elements
// that are present and identical across every translatable source run
// in the paragraph (computed by commonRPrChildren BEFORE mergeRuns
// collapsed adjacent same-toggle runs). When non-empty it is stored as
// the openxmlSourceRPrAnnotation on the block so the writer can
// reapply it on every emitted <w:r>. This is the per-run rPr
// preservation path required by Bowrain Issue #592.
//
// perRunRPrXML is the per-text-run rPr fragments sidecar (Phase 1 of
// the per-run rPr work — see PARITY_NOTES.md "1083-*" cluster).
// When non-empty it is stashed as the openxmlPerRunRPrAnnotation on
// the block; the writer wire-up that consumes it lands in Phase 2.
// Until then this annotation is read-only sidecar data and does not
// change writer behaviour.
func (p *wmlParser) buildBlock(id string, runs []textRun, partPath, commonRPrXML string, perRunRPrXML []string) *model.Block {
	b := &runBuilder{}
	spanCounter := 0

	var activeProps *runProps

	for _, run := range runs {
		// Handle sentinel markers for special content
		if strings.HasPrefix(run.text, "\uE100") {
			// Tab placeholder
			spanCounter++
			b.AddPh(fmt.Sprintf("c%d", spanCounter),
				TypeTab, SubTypeTab,
				"<w:tab/>", "\t", "",
				false, false, false)
			continue
		}
		if strings.HasPrefix(run.text, "\uE101") {
			// Image/drawing/pict/object/oMath/AlternateContent
			// placeholder. The original element's full XML is in
			// run.data so the writer can restore it byte-for-byte.
			// Fall back to a self-closing <w:drawing/> if data was
			// never populated (legacy callers).
			spanCounter++
			data := run.data
			if data == "" {
				data = "<w:drawing/>"
			}
			b.AddPh(fmt.Sprintf("c%d", spanCounter),
				TypeImage, SubTypeImage,
				data, "", "",
				false, false, false)
			continue
		}
		if strings.HasPrefix(run.text, "\uE102:") {
			// Footnote/endnote reference
			noteID := strings.TrimPrefix(run.text, "\uE102:")
			spanCounter++
			b.AddPh(fmt.Sprintf("c%d", spanCounter),
				TypeFootnoteRef, SubTypeFootnoteRef,
				fmt.Sprintf(`<w:footnoteReference w:id="%s"/>`, noteID),
				"",
				fmt.Sprintf("[%s]", noteID),
				false, false, false)
			continue
		}
		if strings.HasPrefix(run.text, "\uE103:") {
			// Hyperlink open
			data := strings.TrimPrefix(run.text, "\uE103:")
			spanCounter++
			b.AddPcOpen(fmt.Sprintf("c%d", spanCounter),
				TypeHyperlink, SubTypeHyperlink,
				data, "", "",
				true, true, true)
			continue
		}
		if strings.HasPrefix(run.text, "\uE104:") {
			// Hyperlink close
			if activeProps != nil && !activeProps.isEmpty() {
				// Close formatting before hyperlink close
				activeProps.appendClosingRuns(b, &spanCounter)
				activeProps = nil
			}
			spanCounter++
			b.AddPcClose(fmt.Sprintf("c%d", spanCounter),
				TypeHyperlink, SubTypeHyperlink,
				"</w:hyperlink>", "")
			continue
		}
		if strings.HasPrefix(run.text, "\uE106:") || strings.HasPrefix(run.text, "\uE107:") {
			// Bookmark start/end placeholder. Per ECMA-376 Part 1
			// \u00A717.13.6 these are direct children of <w:p> rather
			// than <w:r>. The writer's `default` Ph branch emits
			// Ph.Data verbatim with no <w:r> wrapper, mirroring
			// upstream Okapi which adds non-_GoBack bookmarks as
			// inline Markup chunks on the Block (see
			// BlockSkippableElements.skip / BlockParser line 294).
			//
			// Close any active formatting first so the bookmark
			// doesn't sit between the open <w:r>...rPr and the
			// next text run when re-rendered.
			if activeProps != nil && !activeProps.isEmpty() {
				activeProps.appendClosingRuns(b, &spanCounter)
				activeProps = nil
			}
			subType := SubTypeBookmarkStart
			if strings.HasPrefix(run.text, "\uE107:") {
				subType = SubTypeBookmarkEnd
			}
			spanCounter++
			b.AddPh(fmt.Sprintf("c%d", spanCounter),
				TypeBookmark, subType,
				run.data, "", "",
				false, false, false)
			continue
		}
		if isFieldSentinel(run.text) {
			// Complex-field markup chunk. Per upstream Okapi
			// RunParser.parseComplexField (lines 461-542 of
			// okapi/filters/openxml/src/main/java/net/sf/okapi/filters/
			// openxml/RunParser.java) every fldChar (begin/separate/
			// end) and instrText event flows through
			// runBuilder.addToMarkup so the original markup survives
			// the round-trip even when the field code is not in
			// tsComplexFieldDefinitionsToExtract. Same shape applies to
			// fldSimple per BlockParser.parse lines 242-250.
			//
			// Close any active formatting first so the field markup
			// doesn't get trapped inside an <w:r>...rPr wrapper meant
			// for the surrounding translatable text. The captured
			// payload already carries its own <w:r>...</w:r> (or
			// <w:fldSimple>...</w:fldSimple>) wrapper.
			if activeProps != nil && !activeProps.isEmpty() {
				activeProps.appendClosingRuns(b, &spanCounter)
				activeProps = nil
			}
			subType := SubTypeFieldChar
			if strings.HasPrefix(run.text, "\uE108:fldSimple") {
				subType = SubTypeFieldSimple
			}
			spanCounter++
			b.AddPh(fmt.Sprintf("c%d", spanCounter),
				TypeField, subType,
				run.data, "", "",
				false, false, false)
			continue
		}

		// Handle line break
		if run.text == "\n" {
			spanCounter++
			b.AddPh(fmt.Sprintf("c%d", spanCounter),
				TypeBreak, SubTypeBreak,
				"<w:br/>", "\n", "",
				false, false, false)
			continue
		}

		// Handle formatting changes
		if activeProps == nil || !activeProps.equal(run.props) {
			// Close previous formatting
			if activeProps != nil && !activeProps.isEmpty() {
				activeProps.appendClosingRuns(b, &spanCounter)
			}
			// Open new formatting
			if !run.props.isEmpty() {
				run.props.appendOpeningRuns(b, &spanCounter)
			}
			propsCopy := run.props
			activeProps = &propsCopy
		} else if !activeProps.equalIncludingChildren(run.props) {
			// Toggles match (so no PcOpen/PcClose break was emitted)
			// but the non-toggle rPrChildren differ between adjacent
			// source runs (e.g. different <w:color>, <w:sz>, or
			// <w:rStyle>). Force a model.Run boundary so the per-
			// source-run rPr sidecar (#592 Phase 1) stays slot-
			// aligned with the model.Run population — otherwise the
			// writer's alignment guard (renderWMLBlock) nils the
			// sidecar and per-run rPr emission (Phase 2) silently
			// regresses to common-rPr-only output.
			//
			// Mirrors upstream Okapi RunBuilder.java lines 73-188 +
			// RunMerger.canRunPropertiesBeMerged (RunMerger.java
			// lines 156-229): heterogeneous RunProperties (toggle OR
			// non-toggle) keep runs distinct on the way to the
			// writer. Per ECMA-376-1 §17.3.2.
			b.Break()
			propsCopy := run.props
			activeProps = &propsCopy
		}

		b.AddText(run.text)
	}

	// Close any remaining open formatting
	if activeProps != nil && !activeProps.isEmpty() {
		activeProps.appendClosingRuns(b, &spanCounter)
	}

	// Apply code finder before block construction so the placeholder
	// runs it inserts land in the builder's run sequence alongside the
	// formatting runs.
	blockRuns := b.Runs()
	if p.codeFinder != nil {
		blockRuns = p.codeFinder.applyToRuns(blockRuns, &spanCounter)
	}

	block := &model.Block{
		ID:           id,
		Type:         "paragraph",
		Translatable: true,
		Source:       []*model.Segment{model.NewRunsSegment("s1", blockRuns)},
		Targets:      make(map[model.LocaleID][]*model.Segment),
		Properties:   map[string]string{"partPath": partPath},
		Annotations:  make(map[string]model.Annotation),
	}

	// Collect font info if configured
	if p.cfg.ExtractRunFontsInfo {
		fonts := collectFonts(runs)
		if fonts != "" {
			block.Annotations["fonts"] = &model.GenericAnnotation{
				Kind:   "fonts",
				Fields: map[string]any{"names": fonts},
			}
		}
	}

	// Stash the common per-source-run rPr children for the writer (#592).
	// The writer prepends this XML to every emitted <w:r>'s <w:rPr>; the
	// WSO post-pass then lifts it into a synthesised paragraph style when
	// the optimisation conditions are met (mirroring upstream Okapi
	// StyleOptimisation.Default.applyTo, see StyleOptimisation.java
	// lines 96-129 of okapi-filter-openxml).
	if commonRPrXML != "" {
		block.Annotations[openxmlSourceRPrAnnotationKey] = &model.GenericAnnotation{
			Kind:   openxmlSourceRPrAnnotationKey,
			Fields: map[string]any{"xml": commonRPrXML},
		}
	}

	// Stash the per-text-run rPr fragments sidecar (Phase 1 of the
	// per-run rPr work — see PARITY_NOTES.md "1083-*" cluster). The
	// writer wire-up (Phase 2) consumes this annotation; until then it
	// is read-only sidecar data and does not change writer behaviour.
	if len(perRunRPrXML) > 0 {
		block.Annotations[openxmlPerRunRPrAnnotationKey] = &model.GenericAnnotation{
			Kind:   openxmlPerRunRPrAnnotationKey,
			Fields: map[string]any{"fragments": perRunRPrXML},
		}
	}

	return block
}

// mergeRuns merges adjacent runs with identical formatting.
//
// The equality test gates fusion on the FULL rPr (toggles + non-toggle
// children — rStyle, color, sz, lang, highlight, …) via
// equalIncludingChildren, mirroring upstream Okapi
// RunMerger.canRunPropertiesBeMerged (RunMerger.java:156-229). Per
// ECMA-376-1 §17.3.2, heterogeneous rPr means heterogeneous runs:
// runs that differ on ANY rPr property must remain distinct merged
// runs so the per-source-run rPr sidecar (#592, Phase 1) stays aligned
// with the merged-run count for the writer (Phase 2).
func mergeRuns(runs []textRun) []textRun {
	if len(runs) <= 1 {
		return runs
	}

	var merged []textRun
	current := runs[0]

	for i := 1; i < len(runs); i++ {
		r := runs[i]
		// Don't merge sentinel markers or line breaks
		if isSentinel(current.text) || isSentinel(r.text) ||
			current.text == "\n" || r.text == "\n" {
			merged = append(merged, current)
			current = r
			continue
		}
		if current.props.equalIncludingChildren(r.props) {
			current.text += r.text
		} else {
			merged = append(merged, current)
			current = r
		}
	}
	merged = append(merged, current)
	return merged
}

// isSentinel returns true if the text is a special marker.
func isSentinel(s string) bool {
	r := []rune(s)
	if len(r) == 0 {
		return false
	}
	if r[0] < '\uE100' || r[0] > '\uE108' {
		return false
	}
	// Single-char sentinels (tab \uE100, image \uE101, paragraph
	// opaque \uE105). Note: \uE105 wraps math (m:oMathPara/m:oMath)
	// or paragraph-level mc:AlternateContent \u2014 content that is a
	// direct <w:p> child rather than a <w:r> child, so the writer
	// must not wrap it in <w:r> when re-emitting.
	if len(r) == 1 {
		return true
	}
	// Multi-char sentinels must have ':' separator
	// (\uE102:id, \uE103:data, \uE104:data, \uE106:id, \uE107:id,
	// \uE108:fldChar / \uE108:fldSimple)
	return len(r) >= 2 && r[1] == ':'
}

// isFieldSentinel reports whether a textRun's text marker indicates
// captured complex-field markup: a <w:r> wrapping fldChar / instrText
// (subtype suffix `fldChar`) or a <w:fldSimple>...</w:fldSimple>
// (subtype suffix `fldSimple`). Carrier sentinel is U+E108. Per
// upstream Okapi (RunParser.parseComplexField, lines 461-542 of
// okapi/filters/openxml/src/main/java/net/sf/okapi/filters/openxml/
// RunParser.java; BlockParser.parse for fldSimple, lines 242-250 of
// BlockParser.java) such markup is preserved as opaque chunks on the
// block irrespective of whether the field code is in
// tsComplexFieldDefinitionsToExtract \u2014 the writer dumps Ph.Data
// verbatim with no <w:r> wrapper because the <w:r> open/close (or
// <w:fldSimple> open/close) is part of the captured payload.
func isFieldSentinel(text string) bool {
	if text == "" {
		return false
	}
	r := []rune(text)
	if len(r) == 0 {
		return false
	}
	return r[0] == '\uE108'
}

// filterFieldRuns is currently a pass-through that documents the run
// shape coming out of parseRunWithFieldState: when a field-marker
// child was seen the returned slice is exactly one SubTypeFieldChar
// sentinel run carrying the raw <w:r>...</w:r> payload; otherwise
// it's a regular slice of translatable text runs. The function exists
// as a future extension point if per-run policy needs to evolve (e.g.
// dropping field markup inside hidden text). At present we always
// keep the captured field markup so it survives the round-trip.
func filterFieldRuns(runs []textRun, _ *complexFieldState) []textRun {
	return runs
}

// dropTextRuns removes plain translatable runs from a slice while
// keeping every sentinel run (field markup, drawings, bookmarks, \u2026).
// Mirrors upstream Okapi's parseComplexField branching at lines 501-
// 506 of RunParser.java where, when the field is non-extractable or
// the reader is still before the separator, content events are routed
// to runBuilder.addToMarkup (preserved as opaque markup) rather than
// to the run text. Translatable text alongside the field markup never
// reaches the block, but the field markup itself does.
func dropTextRuns(runs []textRun) []textRun {
	out := runs[:0]
	for _, r := range runs {
		if isSentinel(r.text) {
			out = append(out, r)
		}
	}
	return out
}

// isBookmarkSentinel reports whether a textRun's text marker
// indicates a captured `<w:bookmarkStart>` (\uE106) or
// `<w:bookmarkEnd>` (\uE107). Bookmarks are direct children of
// `<w:p>` per ECMA-376 \u00A717.13.6, NOT children of `<w:r>`, so the
// writer must NOT wrap the captured XML in `<w:r>...</w:r>`.
func isBookmarkSentinel(text string) bool {
	if text == "" {
		return false
	}
	r := []rune(text)
	if len(r) == 0 {
		return false
	}
	return r[0] == '' || r[0] == ''
}

// isDrawingSentinel reports whether a textRun's text marker
// indicates an opaque drawing/pict/object/AlternateContent payload
// (run-level "" or paragraph-level ""). Used by
// parseParagraph to scope drawing-XML pre-extraction to the runs
// that actually carry captured payloads.
func isDrawingSentinel(text string) bool {
	if text == "" {
		return false
	}
	r := []rune(text)
	if len(r) == 0 {
		return false
	}
	return r[0] == '' || r[0] == ''
}

// isEmptyRuns returns true if all runs have no visible text content.
func isEmptyRuns(runs []textRun) bool {
	for _, r := range runs {
		if isSentinel(r.text) {
			continue
		}
		if strings.TrimSpace(r.text) != "" {
			return false
		}
	}
	return true
}

// allHidden returns true if all runs have the vanish property.
func allHidden(runs []textRun) bool {
	for _, r := range runs {
		if isSentinel(r.text) {
			continue
		}
		if !r.props.vanish && strings.TrimSpace(r.text) != "" {
			return false
		}
	}
	return true
}

// runToXML converts a text run back to XML for skeleton output. The
// run is wrapped in <w:r>...</w:r>; the body is either an opaque
// payload (drawing, pict, AlternateContent — preserved verbatim from
// run.data) or a <w:t> text element. Empty drawings (no captured data)
// fall back to a self-closing <w:drawing/>.
func runToXML(r textRun) string {
	// Paragraph-level opaque sentinel (\uE105): emit captured raw
	// XML directly with no <w:r> wrapper. Used for math (m:oMathPara,
	// m:oMath) and paragraph-level mc:AlternateContent that appear
	// as direct children of <w:p>.
	if strings.HasPrefix(r.text, "\uE105") {
		if r.data != "" {
			return r.data
		}
		return ""
	}
	// Bookmark sentinels (\uE106 / \uE107) \u2014 emit the captured raw
	// XML verbatim with no <w:r> wrapper. ECMA-376 Part 1
	// \u00A717.13.6.1 / \u00A717.13.6.2 specify <w:bookmarkStart> /
	// <w:bookmarkEnd> as direct children of <w:p>, not <w:r>.
	if isBookmarkSentinel(r.text) {
		return r.data
	}
	// Field-markup sentinel (\uE108) \u2014 captured payload already
	// carries the full <w:r>...</w:r> (for fldChar / instrText) or
	// <w:fldSimple>...</w:fldSimple> wrapper, so emit verbatim with
	// no additional wrapping. Mirrors the bookmark path above.
	if isFieldSentinel(r.text) {
		return r.data
	}
	var buf strings.Builder
	buf.WriteString("<w:r>")
	if !r.props.isEmpty() {
		buf.WriteString("<w:rPr>")
		if r.props.bold {
			buf.WriteString("<w:b/>")
		}
		if r.props.italic {
			buf.WriteString("<w:i/>")
		}
		if r.props.underline != "" {
			buf.WriteString(`<w:u w:val="` + r.props.underline + `"/>`)
		}
		if r.props.strike {
			buf.WriteString("<w:strike/>")
		}
		if r.props.vertAlign != "" {
			buf.WriteString(`<w:vertAlign w:val="` + r.props.vertAlign + `"/>`)
		}
		if r.props.vanish {
			buf.WriteString("<w:vanish/>")
		}
		buf.WriteString("</w:rPr>")
	}
	switch {
	case strings.HasPrefix(r.text, ""):
		// drawing/pict/object/AlternateContent — emit captured raw XML
		if r.data != "" {
			buf.WriteString(r.data)
		} else {
			buf.WriteString("<w:drawing/>")
		}
	case r.text == "":
		buf.WriteString("<w:tab/>")
	case r.text == "\n":
		buf.WriteString("<w:br/>")
	case strings.HasPrefix(r.text, ":"):
		noteID := strings.TrimPrefix(r.text, ":")
		buf.WriteString(fmt.Sprintf(`<w:footnoteReference w:id="%s"/>`, noteID))
	default:
		buf.WriteString(`<w:t xml:space="preserve">`)
		buf.WriteString(xmlEscape(r.text))
		buf.WriteString("</w:t>")
	}
	buf.WriteString("</w:r>")
	return buf.String()
}

// writeRunToSkel emits a textRun directly into the skeleton stream.
// Mostly delegates to runToXML, but for opaque drawing/pict/object/
// AlternateContent payloads (sentinel "" or paragraph-level
// ""), it scans the captured XML for translatable name=
// attributes on <wp:docPr> / <pic:cNvPr> / <wps:cNvPr> elements and
// emits a separate "property" Block per match — interleaving the raw
// XML between attribute-value substitution points and skeleton refs
// to those blocks. This mirrors Okapi's
// RunParser.processTranslatableAttributes (line ~838 of
// okapi/filters/openxml/src/main/java/net/sf/okapi/filters/openxml/
// RunParser.java) which extracts wp:docPr/@name when
// ConditionalParameters.getTranslateWordGraphicName() is true (the
// default). Without this extraction, drawings round-trip with the
// source-language object name still present (e.g. "Bild 1") while
// Okapi would have translated it ("ßĩĺď 1" under pseudo-translation),
// producing structural-but-semantic divergence.
func (p *wmlParser) writeRunToSkel(r textRun, partPath string, emitBlock func(*model.Block)) {
	// For opaque sentinel runs with captured data, do attribute
	// extraction. Otherwise, fall back to the simple runToXML path.
	isOpaque := strings.HasPrefix(r.text, "") || strings.HasPrefix(r.text, "")
	if !isOpaque || r.data == "" {
		p.skelText(runToXML(r))
		return
	}

	// Wrap opaque payload in <w:r>...</w:r> for run-level sentinels;
	// paragraph-level sentinels () carry no <w:r> wrapper.
	wrap := strings.HasPrefix(r.text, "")
	if wrap {
		// Emit the run open tag (with rPr if needed) via runToXML on
		// a stripped variant — simpler to construct a synthetic
		// run with empty data and slice the inner.
		open, close := splitRunWrapper(r)
		p.skelText(open)
		p.writeDrawingXMLToSkel(r.data, partPath, emitBlock)
		p.skelText(close)
		return
	}
	p.writeDrawingXMLToSkel(r.data, partPath, emitBlock)
}

// splitRunWrapper returns the opening and closing portions of the
// <w:r>...</w:r> wrapper for a sentinel run, with the run's run-
// properties (rPr) included in the opening. Used by writeRunToSkel to
// frame an opaque drawing payload with the original run wrapper while
// emitting the inner XML piecewise to the skeleton.
func splitRunWrapper(r textRun) (open, close string) {
	var buf strings.Builder
	buf.WriteString("<w:r>")
	if !r.props.isEmpty() {
		buf.WriteString("<w:rPr>")
		if r.props.bold {
			buf.WriteString("<w:b/>")
		}
		if r.props.italic {
			buf.WriteString("<w:i/>")
		}
		if r.props.underline != "" {
			buf.WriteString(`<w:u w:val="` + r.props.underline + `"/>`)
		}
		if r.props.strike {
			buf.WriteString("<w:strike/>")
		}
		if r.props.vertAlign != "" {
			buf.WriteString(`<w:vertAlign w:val="` + r.props.vertAlign + `"/>`)
		}
		if r.props.vanish {
			buf.WriteString("<w:vanish/>")
		}
		buf.WriteString("</w:rPr>")
	}
	return buf.String(), "</w:r>"
}

// drawingMarkerProp is the comment marker syntax embedded inside
// captured drawing XML at READ time to flag a translatable
// attribute value (drawing-name, vml-textpath-string). The writer
// expands these markers either into skeleton refs (skeleton path,
// writeDrawingXMLToSkel) or into rendered "property" Block content
// (in-block path, writer.go renderWMLBlock TypeImage handler).
const drawingMarkerPropPrefix = "<!--KAPI-PROP:"

// drawingMarkerPara is the marker syntax for a translatable
// paragraph block — used when a captured drawing contains
// <w:txbxContent><w:p>...</w:p></w:txbxContent> (textbox body
// paragraphs).
const drawingMarkerParaPrefix = "<!--KAPI-PARA:"

const drawingMarkerSuffix = "-->"

// drawingMarkerRE matches either a property marker
// (<!--KAPI-PROP:tu123-->) or a paragraph marker
// (<!--KAPI-PARA:tu123-->) and captures the kind plus block ID.
var drawingMarkerRE = regexp.MustCompile(`<!--KAPI-(PROP|PARA):([a-zA-Z0-9_-]+)-->`)

// extractDrawingTranslations scans a captured drawing XML payload,
// emits "property" / "paragraph" Blocks for every translatable
// site (drawing-name attributes, vml-textpath strings, txbx-
// content paragraph bodies), and returns the XML with each site
// replaced by a comment marker referencing the emitted block.
//
// Both writer paths (skeleton flush + in-block TypeImage handler)
// then expand the markers — the skeleton flush turns them into
// real skel refs (inside writeDrawingXMLToSkel), the TypeImage
// handler resolves them against the blocks map and substitutes
// rendered content. Splitting extraction from emission lets
// drawings inside paragraphs that ALSO contain translatable text
// runs (e.g. TextBoxes.docx where the body paragraph has three
// pict-only runs followed by a "Doggy " text run) participate in
// translation — the buildBlock path stuffs the captured XML into
// a TypeImage placeholder, bypassing the skeleton entirely, so the
// extraction must happen up-front.
//
// Mirrors Okapi's RunParser.processTranslatableAttributes
// (RunParser.java lines 838-858) for attribute extraction and
// wordConfiguration.yml's `'wps:txbx': ruleTypes: [GROUP]` (line
// 141) for textbox descent.
func (p *wmlParser) extractDrawingTranslations(xmlData, partPath string, emitBlock func(*model.Block)) string {
	var out strings.Builder
	out.Grow(len(xmlData))
	wrapped := wrapDrawingXMLForDecode(xmlData)
	dec := xml.NewDecoder(strings.NewReader(wrapped))
	if _, err := dec.Token(); err != nil {
		return xmlData
	}
	if err := p.copyAndExtractDrawing(dec, &out, partPath, emitBlock); err != nil {
		// Decoding failure: fall back to verbatim. Do not corrupt
		// the round-trip.
		return xmlData
	}
	return out.String()
}

// copyAndExtractDrawing serialises tokens from dec into out until
// it consumes the matching end of the synthetic wrapper element
// emitted by wrapDrawingXMLForDecode. Translatable sites are
// replaced with marker comments; everything else round-trips
// verbatim.
func (p *wmlParser) copyAndExtractDrawing(dec *xml.Decoder, out *strings.Builder, partPath string, emitBlock func(*model.Block)) error {
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch {
			case isDrawingPropertyElement(t):
				p.writeStartElementWithTranslatableAttrTo(out, t, "name", "drawing-name", partPath, emitBlock)
			case t.Name.Local == "textpath":
				p.writeStartElementWithTranslatableAttrTo(out, t, "string", "vml-textpath-string", partPath, emitBlock)
			case t.Name.Local == "txbxContent":
				writeRawStartElementTo(out, t)
				if err := p.extractTxbxContent(dec, out, t, partPath, emitBlock); err != nil {
					return err
				}
			default:
				writeRawStartElementTo(out, t)
			}
		case xml.EndElement:
			if t.Name.Local == drawingDecodeWrapperLocal {
				return nil
			}
			writeRawEndElementTo(out, t)
		case xml.CharData:
			out.WriteString(xmlEscape(string(t)))
		case xml.Comment:
			out.WriteString("<!--")
			out.Write(t)
			out.WriteString("-->")
		case xml.ProcInst:
			out.WriteString("<?")
			out.WriteString(t.Target)
			if len(t.Inst) > 0 {
				out.WriteString(" ")
				out.Write(t.Inst)
			}
			out.WriteString("?>")
		}
	}
}

// extractTxbxContent processes children of <w:txbxContent>: emits a
// paragraph Block (and a marker comment in place) per <w:p> with
// translatable runs; copies non-paragraph children verbatim.
//
// When a <w:p> contains a complex field (`<w:fldChar>`), the
// paragraph is preserved verbatim — parseParagraph's existing
// non-extractable-field path drops the field markup along with
// its display runs. Falling back to verbatim keeps round-trip
// safe (TextboxNumber.docx with PAGE \* MERGEFORMAT is the
// canonical fixture for this corner).
func (p *wmlParser) extractTxbxContent(
	dec *xml.Decoder,
	out *strings.Builder,
	start xml.StartElement,
	partPath string,
	emitBlock func(*model.Block),
) error {
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local == "p" {
				rawP, err := captureRawElement(dec, t)
				if err != nil {
					return err
				}
				if containsComplexField(rawP) {
					// Preserve the field-bearing paragraph verbatim
					// (rawP from captureRawElement is the full
					// paragraph including its open and close tags).
					// parseParagraph's existing non-extractable-field
					// path drops both the field display runs AND the
					// fldChar markers themselves, which would lose
					// markup like PAGE \* MERGEFORMAT. Verbatim
					// preservation is round-trip safe for textboxes.
					// TextboxNumber.docx is the canonical fixture.
					out.WriteString(rawP)
					continue
				}
				// Re-decode the captured paragraph through a fresh
				// namespace-aware decoder so extractTxbxParagraph
				// sees the canonical token stream with the same
				// prefix bindings as the outer document.
				inner := wrapDrawingXMLForDecode(rawP)
				idec := xml.NewDecoder(strings.NewReader(inner))
				if _, err := idec.Token(); err != nil {
					return err
				}
				// Advance past the <w:p> start tag so
				// extractTxbxParagraph sees the inside of the
				// paragraph (its pPr / runs / end tag).
				for {
					itok, err := idec.Token()
					if err != nil {
						return err
					}
					if se, ok := itok.(xml.StartElement); ok && se.Name.Local == "p" {
						break
					}
				}
				if err := p.extractTxbxParagraph(idec, out, partPath, emitBlock); err != nil {
					return err
				}
			} else if t.Name.Local == "tbl" || t.Name.Local == "tr" || t.Name.Local == "tc" {
				writeRawStartElementTo(out, t)
				if err := p.extractTxbxContent(dec, out, t, partPath, emitBlock); err != nil {
					return err
				}
			} else {
				raw, err := captureRawElement(dec, t)
				if err != nil {
					return err
				}
				out.WriteString(raw)
			}
		case xml.EndElement:
			writeRawEndElementTo(out, t)
			if t.Name.Local == start.Name.Local {
				return nil
			}
		case xml.CharData:
			out.WriteString(xmlEscape(string(t)))
		case xml.Comment:
			out.WriteString("<!--")
			out.Write(t)
			out.WriteString("-->")
		}
	}
}

// extractTxbxParagraph parses a <w:p> from a textbox body: the
// caller has already positioned the decoder right after the <w:p>
// start tag. We re-implement a minimal subset of parseParagraph's
// behaviour here, capturing pPr verbatim and collecting <w:r>
// runs for blocking, then emit the paragraph block and write a
// `<w:p><pPr/><!--KAPI-PARA:id--></w:p>` to out.
//
// Hyperlinks, sdt, ins/del/moveTo/moveFrom, and AlternateContent
// inside textboxes are rare; we skip them via skipElement to keep
// this scoped. Future fixtures can extend.
func (p *wmlParser) extractTxbxParagraph(dec *xml.Decoder, out *strings.Builder, partPath string, emitBlock func(*model.Block)) error {
	var paraProps string
	var paraStyleID string
	var runs []textRun
	var cfs complexFieldState
	var bms bookmarkSkipState

	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "pPr":
				raw, styleID, err := captureParaProps(dec, t)
				if err != nil {
					return err
				}
				paraProps = raw
				paraStyleID = styleID
			case "r":
				rawStart := startElementToRaw(t)
				rs, err := p.parseRunWithFieldState(dec, &cfs, rawStart)
				if err != nil {
					return err
				}
				rs = filterFieldRuns(rs, &cfs)
				if cfs.active && !cfs.extractable {
					rs = dropTextRuns(rs)
				}
				if cfs.active && cfs.extractable && !cfs.atResult {
					rs = dropTextRuns(rs)
				}
				runs = append(runs, rs...)
			case "bookmarkStart", "bookmarkEnd":
				// See parseParagraph for the bookmark capture rationale.
				bookmark, captured, err := p.captureBookmark(dec, t, &bms)
				if err != nil {
					return err
				}
				if captured {
					runs = append(runs, bookmark)
				}
			case "fldSimple":
				// See parseParagraph for the fldSimple rationale.
				raw, err := captureRawElement(dec, t)
				if err != nil {
					return err
				}
				raw = protectFieldPayloadFromStripping(raw)
				runs = append(runs, textRun{text: ":fldSimple", data: raw})
			case "proofErr", "commentRangeStart", "commentRangeEnd",
				"permStart", "permEnd":
				if err := skipElement(dec); err != nil {
					return err
				}
			default:
				if err := skipElement(dec); err != nil {
					return err
				}
			}
		case xml.EndElement:
			if t.Name.Local != "p" {
				continue
			}
			// Apply style optimisation as parseParagraph does.
			if p.styles != nil && paraStyleID != "" {
				styleProps := p.styles.resolveProps(paraStyleID)
				for i := range runs {
					if !isSentinel(runs[i].text) {
						subtractProps(&runs[i].props, styleProps)
					}
				}
			}
			commonRPr := commonRPrChildren(runs)
			commonRPrXML := joinRPrChildren(commonRPr)
			// Per-run rPr sidecar (Phase 1) — see PARITY_NOTES.md.
			perRunRPrXML := perRunRPrFragments(runs)
			merged := mergeRuns(runs)
			// Recurse extraction into nested drawing/pict
			// payloads so e.g. a docPr name inside an image
			// embedded within a textbox paragraph still reaches
			// the translation pipeline (GraphicInTextBox.docx).
			for i := range merged {
				if isDrawingSentinel(merged[i].text) && merged[i].data != "" {
					merged[i].data = p.extractDrawingTranslations(merged[i].data, partPath, emitBlock)
				}
			}
			// Empty paragraph: emit verbatim wrapper without a
			// translatable block. The pPr (if any) is preserved
			// inside <w:p>...</w:p>.
			if isEmptyRuns(merged) {
				out.WriteString("<w:p>")
				if paraProps != "" {
					out.WriteString(paraProps)
				}
				for _, r := range merged {
					out.WriteString(runToXML(r))
				}
				out.WriteString("</w:p>")
				return nil
			}
			*p.blockCounter++
			blockID := fmt.Sprintf("tu%d", *p.blockCounter)
			out.WriteString("<w:p>")
			if paraProps != "" {
				out.WriteString(paraProps)
			}
			out.WriteString(drawingMarkerParaPrefix)
			out.WriteString(blockID)
			out.WriteString(drawingMarkerSuffix)
			out.WriteString("</w:p>")
			block := p.buildBlock(blockID, merged, partPath, commonRPrXML, perRunRPrXML)
			emitBlock(block)
			return nil
		}
	}
}

// writeRawStartElementTo emits an XML start element to a strings.Builder,
// preserving namespace prefixes via the package nsPrefixMap and
// registering any new xmlns declarations on the element.
func writeRawStartElementTo(out *strings.Builder, t xml.StartElement) {
	registerNamespaces(t.Attr)
	out.WriteString("<")
	writeElementName(out, t.Name)
	for _, a := range t.Attr {
		out.WriteString(" ")
		writeAttrName(out, a.Name)
		out.WriteString(`="`)
		out.WriteString(xmlEscapeAttr(a.Value))
		out.WriteString(`"`)
	}
	out.WriteString(">")
}

// writeRawEndElementTo emits an XML end element to a strings.Builder.
func writeRawEndElementTo(out *strings.Builder, t xml.EndElement) {
	out.WriteString("</")
	writeElementName(out, t.Name)
	out.WriteString(">")
}

// writeStartElementWithTranslatableAttrTo emits a start element to
// the given builder, replacing the named attribute's value with a
// drawingMarkerProp comment marker referencing an emitted block.
func (p *wmlParser) writeStartElementWithTranslatableAttrTo(
	out *strings.Builder,
	t xml.StartElement,
	attrLocal, blockElementTag, partPath string,
	emitBlock func(*model.Block),
) {
	out.WriteString("<")
	writeElementName(out, t.Name)
	emittedRef := false
	for _, a := range t.Attr {
		out.WriteString(" ")
		writeAttrName(out, a.Name)
		out.WriteString(`="`)
		if !emittedRef && a.Name.Local == attrLocal && a.Name.Space == "" && strings.TrimSpace(a.Value) != "" {
			*p.blockCounter++
			refID := fmt.Sprintf("tu%d", *p.blockCounter)
			out.WriteString(drawingMarkerPropPrefix)
			out.WriteString(refID)
			out.WriteString(drawingMarkerSuffix)
			emittedRef = true
			emitBlock(&model.Block{
				ID:           refID,
				Type:         "property",
				Translatable: true,
				Source: []*model.Segment{model.NewRunsSegment(
					"s1",
					[]model.Run{{Text: &model.TextRun{Text: a.Value}}},
				)},
				Targets: make(map[model.LocaleID][]*model.Segment),
				Properties: map[string]string{
					"partPath": partPath,
					"element":  blockElementTag,
				},
				Annotations: make(map[string]model.Annotation),
			})
		} else {
			out.WriteString(xmlEscapeAttr(a.Value))
		}
		out.WriteString(`"`)
	}
	out.WriteString(">")
}

// writeDrawingXMLToSkel emits a drawing's captured raw XML to the
// skeleton, walking the XML token stream to extract translatable
// content at three structural sites:
//
//  1. name= attribute on <wp:docPr> / <pic:cNvPr> / <wps:cNvPr>
//     (drawing object names) — extracted as a "property" Block.
//     Mirrors Okapi's RunParser.processTranslatableAttributes
//     (RunParser.java lines 838-858) gated by
//     ConditionalParameters.getTranslateWordGraphicName() (default
//     true).
//
//  2. string= attribute on <v:textpath> (legacy WordArt text
//     painted along a curve) — extracted as a "property" Block.
//     Mirrors RunParser.processTranslatableAttributes (RunParser.java
//     lines 854-855) which calls processTranslatableAttribute(startEl,
//     "string") whenever XMLEventHelpers.isTextPath(startEl) holds
//     (XMLEventHelpers.java lines 287-289, LOCAL_TEXTPATH = "textpath"
//     at line 77). Per ECMA-376 Part 4 (VML) §6.2.2, the textpath
//     element's string attribute carries the displayed text.
//
//  3. <w:p> paragraphs nested inside <w:txbxContent> (drawing
//     textbox bodies — both the WordprocessingML <wps:txbx> shape
//     wrapper and the legacy VML <v:textbox> wrapper produce a
//     <w:txbxContent> child holding regular WML paragraphs). These
//     are parsed via parseParagraph so the inner text emits as
//     normal "paragraph" Blocks (with inline runs, hyperlinks,
//     fldChars, …). The skeleton stream interleaves the captured
//     drawing/textbox markup with paragraph block refs so the
//     writer reconstructs <w:txbxContent> with translated runs in
//     place. Mirrors Okapi's word-configuration.yml at line 141
//     ('wps:txbx': ruleTypes: [GROUP]) which directs the filter to
//     descend into the textbox content as a structural group rather
//     than treating it as opaque inline content.
//
// Anything else passes through verbatim.
//
// The xmlData has already been processed by
// extractDrawingTranslations (called from parseParagraph before
// the empty-runs path branches into writeRunToSkel) — meaning
// translatable sites are already represented as
// <!--KAPI-PROP:tu123--> / <!--KAPI-PARA:tu123--> markers and the
// corresponding Blocks have been emitted to the part stream. All
// this function does is split the modified XML on markers,
// emitting skeleton refs in their place so the writer's skeleton
// stitching expands them into rendered block content.
func (p *wmlParser) writeDrawingXMLToSkel(xmlData, _partPath string, _emitBlock func(*model.Block)) {
	matches := drawingMarkerRE.FindAllStringSubmatchIndex(xmlData, -1)
	if len(matches) == 0 {
		p.skelText(xmlData)
		return
	}
	pos := 0
	for _, m := range matches {
		// m = [whole_lo, whole_hi, kind_lo, kind_hi, id_lo, id_hi]
		p.skelText(xmlData[pos:m[0]])
		blockID := xmlData[m[4]:m[5]]
		p.skelRef(blockID)
		pos = m[1]
	}
	p.skelText(xmlData[pos:])
}

// drawingDecodeWrapperLocal is the local-name of the synthetic root
// element used to wrap captured drawing XML so encoding/xml can
// resolve prefixes. It only ever exists in the temporary input to
// the decoder and never reaches the skeleton stream.
const drawingDecodeWrapperLocal = "neokapi_drawing_wrapper"

// drawingDecodeWrapperPrefix is the namespace declarations injected
// onto the synthetic wrapper so every known OpenXML prefix resolves
// to its full URI when the decoder reads child elements. Built once
// at package init from nsPrefixMap (skipping the empty prefix and
// the synthetic xmlns/xml prefixes which encoding/xml handles).
var drawingDecodeWrapperPrefix string

func init() {
	var b strings.Builder
	b.WriteString("<")
	b.WriteString(drawingDecodeWrapperLocal)
	for uri, prefix := range nsPrefixMap {
		// xml prefix is implicit; xmlns prefix is reserved.
		if prefix == "" || prefix == "xml" || prefix == "xmlns" {
			continue
		}
		b.WriteString(` xmlns:`)
		b.WriteString(prefix)
		b.WriteString(`="`)
		b.WriteString(xmlEscapeAttr(uri))
		b.WriteString(`"`)
	}
	b.WriteString(">")
	drawingDecodeWrapperPrefix = b.String()
}

// wrapDrawingXMLForDecode wraps captured drawing XML in a synthetic
// root that declares every known OpenXML namespace prefix, so
// encoding/xml's namespace-aware decoder can fully qualify the
// Names of nested elements (`w:drawing`, `v:textpath`, `wps:txbx`,
// …). The wrapper is stripped during re-emission — see
// writeDrawingXMLToSkel.
func wrapDrawingXMLForDecode(xmlData string) string {
	var b strings.Builder
	b.Grow(len(drawingDecodeWrapperPrefix) + len(xmlData) + len(drawingDecodeWrapperLocal) + 4)
	b.WriteString(drawingDecodeWrapperPrefix)
	b.WriteString(xmlData)
	b.WriteString("</")
	b.WriteString(drawingDecodeWrapperLocal)
	b.WriteString(">")
	return b.String()
}

// isDrawingPropertyElement reports whether t is a non-visual drawing
// property carrier (<docPr> on a wp wrapper, or <cNvPr> on any
// pic/wps/dgm wrapper) whose name attribute Okapi treats as
// translatable. Mirrors XMLEventHelpers.isDrawingProperty (lines
// 291-294 of okapi/filters/openxml/src/main/java/net/sf/okapi/
// filters/openxml/XMLEventHelpers.java) which checks two local
// names: LOCAL_NON_VISUAL_OBJECT_PROPERTY ("docPr") and
// LOCAL_NON_VISUAL_CANVAS_PROPERTY ("cNvPr").
func isDrawingPropertyElement(t xml.StartElement) bool {
	return t.Name.Local == "docPr" || t.Name.Local == "cNvPr"
}

// startElementToRaw serialises the open form of an xml.StartElement to
// the same raw XML shape captureRawElement uses — prefixed local name,
// attribute pairs in source order, attributes xml-attr-escaped, no
// closing slash. Used by callers of parseRunWithFieldState that need
// to hand the function the raw <w:r ...> open tag so it can rebuild
// the verbatim run payload when field markup is detected inside.
// fieldRPrKeepEmptyMarker is the comment marker emitted inside an
// otherwise-empty `<w:rPr></w:rPr>` captured from a complex-field run
// so the writer's stripWMLSkippableElements pass leaves the wrapper
// in place. Removed by postWML before the document is written to the
// output zip. Per upstream Okapi (RunParser.parseComplexField, lines
// 461-542 of okapi/filters/openxml/src/main/java/net/sf/okapi/filters/
// openxml/RunParser.java) field-bearing runs flow through
// runBuilder.addToMarkup verbatim, bypassing
// RunProperties.Default.getEvents (RunProperties.java line 580) which
// would otherwise collapse the empty rPr — so the emitted shape is
// `<w:r><w:rPr/><w:t>...</w:t></w:r>` rather than the bare
// `<w:r><w:t>...</w:t></w:r>` Okapi emits for non-field runs.
const fieldRPrKeepEmptyMarker = "<!--KAPI-FIELD-RPR-->"

// fieldRPrStripREs are the per-element regexes used by
// stripFieldRPrSkippables to remove run-property children that Okapi
// strips via RunSkippableElements (RunSkippableElements.java lines
// 50-62 of okapi/filters/openxml/src/main/java/net/sf/okapi/filters/
// openxml/RunSkippableElements.java). The complete list per upstream:
//   - <w:lang>            (RUN_PROPERTY_LANGUAGE)
//   - <w:noProof>         (RUN_PROPERTY_NO_SPELLING_OR_GRAMMAR)
//   - <w:rPrChange>       (RUN_PROPERTIES_CHANGE — revision tracking)
// Each regex matches both self-closing and open/close forms and
// allows attributes / xmlns declarations on the start tag.
var fieldRPrStripREs = []*regexp.Regexp{
	regexp.MustCompile(`<w:lang\b[^>]*/>|<w:lang\b[^>]*>.*?</w:lang>`),
	regexp.MustCompile(`<w:noProof\b[^>]*/>|<w:noProof\b[^>]*>.*?</w:noProof>`),
	regexp.MustCompile(`<w:rPrChange\b[^>]*/>|<w:rPrChange\b[^>]*>.*?</w:rPrChange>`),
}

// fieldRPrEmptyRE matches an `<w:rPr>` that is empty after
// stripFieldRPrSkippables removed every child. Captures the open and
// close tags so the helper can replace the run with the
// fieldRPrKeepEmptyMarker variant.
var fieldRPrEmptyRE = regexp.MustCompile(`<w:rPr>\s*</w:rPr>|<w:rPr\s*/>`)

// isStrippedRPrEmpty reports whether stripFieldRPrSkippables's output
// represents an empty rPr — either the bare `<w:rPr></w:rPr>` /
// `<w:rPr/>` shape OR the keep-empty marker variant
// `<w:rPr><!--KAPI-FIELD-RPR--></w:rPr>` the helper emits when the
// original rPr collapsed to empty after skippable-element stripping.
// Used by the entry-point-run path of parseRunWithFieldState to drop
// the rPr entirely when nothing of substance survives — mirroring
// upstream Okapi's RunProperties.Default.getEvents (line 580 of
// RunProperties.java) which returns no events for empty properties.
func isStrippedRPrEmpty(stripped string) bool {
	if fieldRPrEmptyRE.MatchString(stripped) {
		return true
	}
	return stripped == "<w:rPr>"+fieldRPrKeepEmptyMarker+"</w:rPr>"
}

// protectFieldPayloadFromStripping wraps an opaque field payload (a
// captured <w:fldSimple>...</w:fldSimple> blob, or any future opaque
// field chunk) in element renames so the writer's
// stripWMLSkippableElements pass leaves the payload alone. Per
// upstream Okapi BlockParser.parse
// (lines 242-250 of okapi/filters/openxml/src/main/java/net/sf/okapi/
// filters/openxml/BlockParser.java) the entire <w:fldSimple> element
// is gathered into markup verbatim — so any <w:noProof/> / <w:lang/>
// / <w:rPrChange/> inside it must survive the round-trip with no
// stripping (Document-with-formula-and-tabs.docx is the canonical
// AUTHOR-fldSimple fixture: source has `<w:rPr><w:noProof/></w:rPr>`,
// reference round-trip preserves it). Rename each strippable element's
// open tag (e.g. `w:noProof` → `w:noProofKAPIKEEP`) so the writer's
// stripWMLSkippableElements regex does not match. postWML reverses
// the rename after stripping.
//
// This protect/unprotect dance is the cleanest way to scope a
// document-wide regex strip to "everything except these regions",
// short of refactoring stripWMLSkippableElements to be position-aware
// (which would require an XML parse pass over the full document.xml
// per write, and is overkill for a handful of opaque field payloads).
func protectFieldPayloadFromStripping(payload string) string {
	for _, name := range fieldKeepElementNames {
		// Match `<w:NAME` (open tag, attrs follow) — replace with
		// `<w:NAMEKAPIKEEP`. Match `</w:NAME` (close tag) — same. The
		// body of the element is left untouched. Self-closing forms
		// (`<w:NAME/>`) are also covered by the open-tag rename
		// because the trailing `/>` is part of attribute-territory.
		open := "<w:" + name
		openKeep := "<w:" + name + fieldKeepElementSuffix
		payload = strings.ReplaceAll(payload, open, openKeep)
		closeTag := "</w:" + name + ">"
		closeKeep := "</w:" + name + fieldKeepElementSuffix + ">"
		payload = strings.ReplaceAll(payload, closeTag, closeKeep)
	}
	return payload
}

// fieldKeepElementNames lists the WordprocessingML element local
// names that the writer's stripWMLSkippableElements pass would strip
// from the entire document.xml — protectFieldPayloadFromStripping
// renames each occurrence inside an opaque field payload so the strip
// passes them by. Mirrors stripWMLSkippableElements / wmlNoProofRE /
// wmlStrippableElementRE in writer.go: any element name added there
// also needs to appear here so fldSimple round-trip stays clean.
var fieldKeepElementNames = []string{
	"noProof",
	"lang",
	"bidiVisual",
	"rPrChange",
	"moveToRange",
	"moveFromRange",
	"moveToRangeStart",
	"moveToRangeEnd",
	"moveFromRangeStart",
	"moveFromRangeEnd",
}

// fieldKeepElementSuffix is the rename suffix appended by
// protectFieldPayloadFromStripping. Chosen so the resulting element
// name is well-formed XML, has no chance of colliding with a real
// WordprocessingML element name, and is cheap to scan-and-replace in
// postWML.
const fieldKeepElementSuffix = "KAPIKEEP"

// stripFieldRPrSkippables takes the raw `<w:rPr>...</w:rPr>` blob
// captured from a complex-field run, strips the always-stripped
// children (noProof, lang, rPrChange — the same set
// RunSkippableElements drops upstream), and re-emits the wrapper. If
// the wrapper would collapse to empty, emits
// `<w:rPr>fieldRPrKeepEmptyMarker</w:rPr>` so the writer's empty-
// container regex skips it. Pure string transform — keeps the prefix
// shape (e.g. `w:`) the captureRawElement output uses.
func stripFieldRPrSkippables(rPrXML string) string {
	for _, re := range fieldRPrStripREs {
		rPrXML = re.ReplaceAllString(rPrXML, "")
	}
	if fieldRPrEmptyRE.MatchString(rPrXML) {
		return "<w:rPr>" + fieldRPrKeepEmptyMarker + "</w:rPr>"
	}
	return rPrXML
}

func startElementToRaw(start xml.StartElement) string {
	var b strings.Builder
	b.WriteString("<")
	writeElementName(&b, start.Name)
	for _, a := range start.Attr {
		b.WriteString(" ")
		writeAttrName(&b, a.Name)
		b.WriteString(`="`)
		b.WriteString(xmlEscapeAttr(a.Value))
		b.WriteString(`"`)
	}
	b.WriteString(">")
	return b.String()
}

// containsComplexField reports whether a captured <w:p> XML
// fragment contains an Office complex-field marker (`<w:fldChar`).
// Used by walkTxbxContent to decide between extracting the
// paragraph's text (clean case) and preserving the paragraph
// verbatim (the field-bearing case). String-level scan is
// sufficient — captureRawElement always emits prefixed names via
// the package nsPrefixMap, so the literal `<w:fldChar` substring
// is a stable test for any namespace binding the source used.
func containsComplexField(rawP string) bool {
	return strings.Contains(rawP, "<w:fldChar")
}

// collectFonts returns a comma-separated list of unique font names from runs.
func collectFonts(runs []textRun) string {
	seen := make(map[string]bool)
	var fonts []string
	for _, r := range runs {
		for _, f := range []string{r.props.fontName, r.props.fontNameCS, r.props.fontNameEA} {
			if f != "" && !seen[f] {
				seen[f] = true
				fonts = append(fonts, f)
			}
		}
	}
	return strings.Join(fonts, ", ")
}

// Skeleton helpers

func (p *wmlParser) skelText(s string) {
	if p.skeletonStore != nil {
		p.skelBuf.WriteString(s)
	}
}

func (p *wmlParser) skelRef(id string) {
	if p.skeletonStore != nil {
		if p.skelBuf.Len() > 0 {
			_ = p.skeletonStore.WriteText(p.skelBuf.Bytes())
			p.skelBuf.Reset()
		}
		_ = p.skeletonStore.WriteRef(id)
	}
}

func (p *wmlParser) skelFlush() {
	if p.skeletonStore != nil && p.skelBuf.Len() > 0 {
		_ = p.skeletonStore.WriteText(p.skelBuf.Bytes())
		p.skelBuf.Reset()
	}
}

func (p *wmlParser) skelWriteStartElement(t xml.StartElement) {
	if p.skeletonStore == nil {
		return
	}
	registerNamespaces(t.Attr)
	var buf strings.Builder
	buf.WriteString("<")
	writeElementName(&buf, t.Name)
	for _, a := range t.Attr {
		buf.WriteString(" ")
		writeAttrName(&buf, a.Name)
		buf.WriteString(`="`)
		buf.WriteString(xmlEscapeAttr(a.Value))
		buf.WriteString(`"`)
	}
	buf.WriteString(">")
	p.skelBuf.WriteString(buf.String())
}

func (p *wmlParser) skelWriteEndElement(t xml.EndElement) {
	if p.skeletonStore == nil {
		return
	}
	var buf strings.Builder
	buf.WriteString("</")
	writeElementName(&buf, t.Name)
	buf.WriteString(">")
	p.skelBuf.WriteString(buf.String())
}

func (p *wmlParser) skelWriteString(s string) {
	if p.skeletonStore != nil {
		p.skelBuf.WriteString(s)
	}
}

func (p *wmlParser) skipAndSkel(d *xml.Decoder) error {
	depth := 1
	for depth > 0 {
		tok, err := d.Token()
		if err != nil {
			return err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			p.skelWriteStartElement(t)
		case xml.EndElement:
			depth--
			p.skelWriteEndElement(t)
		case xml.CharData:
			p.skelText(xmlEscape(string(t)))
		}
	}
	return nil
}

// XML helpers

// nsRegistry tracks namespace URI → prefix mappings discovered during parsing.
// It supplements the static nsPrefixMap with dynamic mappings from xmlns: attributes.
var nsRegistry = struct {
	sync.RWMutex
	m map[string]string
}{m: make(map[string]string)}

// registerNamespaces scans an element's attributes for xmlns declarations
// and records the URI → prefix mapping.
func registerNamespaces(attrs []xml.Attr) {
	nsRegistry.Lock()
	for _, a := range attrs {
		if a.Name.Space == "xmlns" {
			// xmlns:prefix="URI" → map URI to prefix
			nsRegistry.m[a.Value] = a.Name.Local
		} else if a.Name.Space == "" && a.Name.Local == "xmlns" {
			// xmlns="URI" (default namespace) → map URI to "" (no prefix)
			nsRegistry.m[a.Value] = ""
		}
	}
	nsRegistry.Unlock()
}

// resolvePrefix returns the namespace prefix for a URI, checking the dynamic
// registry first (which reflects the document's actual declarations), then
// falling back to the static map.
func resolvePrefix(ns string) string {
	nsRegistry.RLock()
	p, ok := nsRegistry.m[ns]
	nsRegistry.RUnlock()
	if ok {
		return p
	}
	if p, ok := nsPrefixMap[ns]; ok {
		return p
	}
	return ""
}

// writeElementName writes an element name with its namespace prefix.
func writeElementName(buf *strings.Builder, name xml.Name) {
	if name.Space != "" {
		prefix := resolvePrefix(name.Space)
		if prefix != "" {
			buf.WriteString(prefix)
			buf.WriteString(":")
		}
		// If no known prefix, write local name only — the namespace is
		// already declared on a parent element via xmlns.
	}
	buf.WriteString(name.Local)
}

// writeAttrName writes an attribute name, handling xmlns declarations.
func writeAttrName(buf *strings.Builder, name xml.Name) {
	if name.Space == "xmlns" {
		// Namespace declaration: xmlns:prefix
		buf.WriteString("xmlns:")
		buf.WriteString(name.Local)
		return
	}
	if name.Space == "" && name.Local == "xmlns" {
		// Default namespace declaration
		buf.WriteString("xmlns")
		return
	}
	if name.Space != "" {
		prefix := resolvePrefix(name.Space)
		if prefix != "" {
			buf.WriteString(prefix)
			buf.WriteString(":")
		}
		// Unknown namespace — omit the prefix. The namespace is
		// already declared on a parent element and the attribute
		// name alone is sufficient for well-formed output.
	}
	buf.WriteString(name.Local)
}

// xmlEscapeAttr escapes a string for use as an XML attribute value.
func xmlEscapeAttr(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

// nsPrefix maps namespace URI → prefix for known OpenXML namespaces.
var nsPrefixMap = map[string]string{
	wmlNamespace: "w",
	dmlNamespace: "a",
	"http://schemas.openxmlformats.org/officeDocument/2006/relationships":       "r",
	"http://schemas.openxmlformats.org/markup-compatibility/2006":               "mc",
	"http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing":    "wp",
	"http://schemas.openxmlformats.org/drawingml/2006/spreadsheetDrawing":       "xdr",
	"http://schemas.openxmlformats.org/drawingml/2006/chart":                    "c",
	"http://schemas.openxmlformats.org/drawingml/2006/diagram":                  "dgm",
	"http://schemas.openxmlformats.org/drawingml/2006/picture":                  "pic",
	"http://schemas.openxmlformats.org/officeDocument/2006/math":                "m",
	"http://schemas.openxmlformats.org/officeDocument/2006/extended-properties": "ep",
	"http://schemas.openxmlformats.org/officeDocument/2006/custom-properties":   "cp",
	"http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes":      "vt",
	"http://schemas.openxmlformats.org/spreadsheetml/2006/main":                 "x",
	"http://schemas.openxmlformats.org/presentationml/2006/main":                "p",
	"http://schemas.openxmlformats.org/package/2006/relationships":              "pr",
	"http://schemas.openxmlformats.org/package/2006/content-types":              "ct",
	"http://schemas.openxmlformats.org/package/2006/metadata/core-properties":   "coreProperties",
	"http://schemas.microsoft.com/office/word/2010/wordml":                      "w14",
	"http://schemas.microsoft.com/office/word/2012/wordml":                      "w15",
	"http://schemas.microsoft.com/office/word/2015/wordml/symex":                "w16se",
	"http://schemas.microsoft.com/office/spreadsheetml/2009/9/main":             "x14",
	"http://schemas.microsoft.com/office/spreadsheetml/2010/11/main":            "x15",
	"http://schemas.microsoft.com/office/powerpoint/2010/main":                  "p14",
	"http://schemas.microsoft.com/office/powerpoint/2012/main":                  "p15",
	"http://schemas.microsoft.com/office/drawing/2010/main":                     "a14",
	"http://schemas.microsoft.com/office/drawing/2014/main":                     "a16",
	"http://purl.org/dc/elements/1.1/":                                          "dc",
	"http://purl.org/dc/terms/":                                                 "dcterms",
	"http://schemas.openxmlformats.org/officeDocument/2006/customXml":           "ds",
	"urn:schemas-microsoft-com:vml":                                             "v",
	"urn:schemas-microsoft-com:office:office":                                   "o",
	"urn:schemas-microsoft-com:office:word":                                     "w10",
	"http://www.w3.org/2001/XMLSchema-instance":                                 "xsi",
	"http://www.w3.org/2001/XMLSchema":                                          "xsd",
	"http://www.w3.org/XML/1998/namespace":                                      "xml",
	// Microsoft Office extension namespaces
	"http://schemas.microsoft.com/office/word/2010/wordprocessingCanvas":  "wpc",
	"http://schemas.microsoft.com/office/word/2010/wordprocessingDrawing": "wp14",
	"http://schemas.microsoft.com/office/word/2010/wordprocessingGroup":   "wpg",
	"http://schemas.microsoft.com/office/word/2010/wordprocessingInk":     "wpi",
	"http://schemas.microsoft.com/office/word/2010/wordprocessingShape":   "wps",
	"http://schemas.microsoft.com/office/word/2006/wordml":                "wne",
	"http://schemas.microsoft.com/office/mac/office/2008/main":            "mo",
	"urn:schemas-microsoft-com:mac:vml":                                   "mv",
	"http://schemas.microsoft.com/office/drawing/2012/chart":              "c15",
	"http://schemas.microsoft.com/office/drawing/2014/chartex":            "cx",
	"http://schemas.openxmlformats.org/drawingml/2006/lockedCanvas":       "lc",
	"http://schemas.microsoft.com/office/drawing/2008/diagram":            "dsp",
	"http://schemas.microsoft.com/office/drawing/2010/diagram":            "dgm14",
	"http://schemas.microsoft.com/office/thememl/2012/main":               "thm15",
	"http://schemas.microsoft.com/office/drawing/2017/decorative":         "adec",
	"http://schemas.microsoft.com/office/drawing/2018/hyperlinkcolor":     "ahlc",
	"http://schemas.microsoft.com/office/word/2016/wordml/cid":            "w16cid",
	"http://schemas.microsoft.com/office/word/2018/wordml":                "w16",
	"http://schemas.microsoft.com/office/word/2018/wordml/cex":            "w16cex",
	"http://schemas.microsoft.com/office/word/2020/wordml/sdtdatahash":    "w16sdtdh",
}

func isWML(el xml.StartElement) bool {
	return el.Name.Space == wmlNamespace
}

func isWMLNoNS(el xml.StartElement) bool {
	return el.Name.Space == ""
}

// readCharData reads character data content of a simple element and consumes its end tag.
func readCharData(d *xml.Decoder) (string, error) {
	var text strings.Builder
	for {
		tok, err := d.Token()
		if err != nil {
			return "", err
		}
		switch t := tok.(type) {
		case xml.CharData:
			text.Write(t)
		case xml.EndElement:
			return text.String(), nil
		case xml.StartElement:
			// Unexpected nested element — skip it
			if err := skipElement(d); err != nil {
				return "", err
			}
		}
	}
}

// captureParaProps captures paragraph properties as raw XML and extracts the pStyle value.
func captureParaProps(d *xml.Decoder, start xml.StartElement) (string, string, error) {
	raw, err := captureRawElement(d, start)
	if err != nil {
		return "", "", err
	}
	// Extract pStyle value from the raw XML
	styleID := extractPStyle(raw)
	return raw, styleID, nil
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

// skippableBookmarkName is the well-known Word internal bookmark
// generated to track the user's last edit position. ECMA-376 doesn't
// reserve the name, but every modern Word build emits it on save (and
// expects it to round-trip as a no-op). Upstream Okapi's
// SkippableElements.BookmarkCrossStructure.SKIPPABLE_BOOKMARK_NAME
// (SkippableElements.java line 304) hard-codes it to `_GoBack` and
// drops both the start and the matching end (by id) silently — we
// mirror that policy exactly. The matching is by id, not by name,
// because the end element only carries an id attribute (ECMA-376
// Part 1 §17.13.6.2 — `<w:bookmarkEnd>` has only the `w:id` attribute).
const skippableBookmarkName = "_GoBack"

// bookmarkSkipState tracks the id of the most recent skipped
// bookmarkStart so the matching bookmarkEnd can also be dropped.
// Mirrors the `identifier` field on
// SkippableElements.CrossStructure (SkippableElements.java line 231)
// and the conditional id check on canBeSkipped (lines 277-281).
type bookmarkSkipState struct {
	skippedID string // id of the last skipped bookmarkStart, "" when no pending skip
}

// captureBookmark serializes a `<w:bookmarkStart>` or `<w:bookmarkEnd>`
// element verbatim (preserving every attribute and namespace prefix)
// and returns it as a sentinel textRun. The boolean second result is
// false when the bookmark should be silently dropped (matching upstream
// Okapi's `_GoBack` skip policy — see skippableBookmarkName for the
// citation). The decoder is advanced past the matching end token in
// every case so the caller can continue draining sibling tokens.
//
// ECMA-376 Part 1 §17.13.6.1 — `<w:bookmarkStart>` has `w:id`,
// `w:name`, plus optional revision-tracking attributes (`w:colFirst`,
// `w:colLast`, `w:displacedByCustomXml`). We preserve ALL of them.
//
// ECMA-376 Part 1 §17.13.6.2 — `<w:bookmarkEnd>` has only `w:id` plus
// the optional `w:displacedByCustomXml`.
func (p *wmlParser) captureBookmark(d *xml.Decoder, start xml.StartElement, bms *bookmarkSkipState) (textRun, bool, error) {
	id := attrVal(start, "id")
	if start.Name.Local == "bookmarkStart" {
		name := attrVal(start, "name")
		if name == skippableBookmarkName {
			bms.skippedID = id
			if err := skipElement(d); err != nil {
				return textRun{}, false, err
			}
			return textRun{}, false, nil
		}
	} else if start.Name.Local == "bookmarkEnd" {
		// A bookmarkEnd whose id matches the last skipped start is
		// the closing half of a skipped `_GoBack` and is dropped
		// silently; once consumed the tracking id is cleared so a
		// later bookmarkEnd with the same id (uncommon but legal
		// when ids are recycled) is preserved.
		if bms.skippedID != "" && bms.skippedID == id {
			bms.skippedID = ""
			if err := skipElement(d); err != nil {
				return textRun{}, false, err
			}
			return textRun{}, false, nil
		}
	}

	raw, err := captureRawElement(d, start)
	if err != nil {
		return textRun{}, false, err
	}

	var sentinel string
	if start.Name.Local == "bookmarkStart" {
		sentinel = ":" + id
	} else {
		sentinel = ":" + id
	}
	return textRun{text: sentinel, data: raw}, true, nil
}

// captureRawElement captures an entire element (start to end) as raw XML.
func captureRawElement(d *xml.Decoder, start xml.StartElement) (string, error) {
	var buf strings.Builder
	buf.WriteString("<")
	writeElementName(&buf, start.Name)
	for _, a := range start.Attr {
		buf.WriteString(" ")
		writeAttrName(&buf, a.Name)
		buf.WriteString(`="`)
		buf.WriteString(xmlEscapeAttr(a.Value))
		buf.WriteString(`"`)
	}
	buf.WriteString(">")

	depth := 1
	for depth > 0 {
		tok, err := d.Token()
		if err != nil {
			return "", err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			depth++
			buf.WriteString("<")
			writeElementName(&buf, t.Name)
			for _, a := range t.Attr {
				buf.WriteString(" ")
				writeAttrName(&buf, a.Name)
				buf.WriteString(`="`)
				buf.WriteString(xmlEscapeAttr(a.Value))
				buf.WriteString(`"`)
			}
			buf.WriteString(">")
		case xml.EndElement:
			depth--
			buf.WriteString("</")
			writeElementName(&buf, t.Name)
			buf.WriteString(">")
		case xml.CharData:
			buf.WriteString(xmlEscape(string(t)))
		case xml.Comment:
			buf.WriteString("<!--")
			buf.Write(t)
			buf.WriteString("-->")
		}
	}
	return buf.String(), nil
}

// captureAlternateContent serializes an <mc:AlternateContent> element,
// preserving the wrapper plus the selected branch but dropping
// <mc:Fallback>. Per ECMA-376 Part 3 / ISO/IEC 29500-3 §10 (Markup
// Compatibility and Extensibility) the consumer must select the first
// <mc:Choice Requires="..."> whose required namespaces are all
// supported, otherwise the <mc:Fallback>. Okapi's reference filter
// always selects the first Choice and unconditionally strips Fallback
// (SkippableElement.GeneralInline.ALTERNATE_CONTENT_FALLBACK at line
// 56 of okapi/filters/openxml/src/main/java/net/sf/okapi/filters/
// openxml/SkippableElement.java; gold fixture
// gold/parts/block/document-alternate-content.xml shows
// <mc:AlternateContent><mc:Choice Requires="wps">...</mc:Choice></
// mc:AlternateContent> surviving the round-trip with Fallback gone).
// We mirror that policy: keep the wrapper, keep every Choice, drop
// every Fallback. The wrapper element name (mc:AlternateContent) and
// child Choice/Fallback names are matched by local-name regardless of
// prefix so documents that bind the markup-compatibility namespace to
// a non-default prefix still work.
func captureAlternateContent(d *xml.Decoder, start xml.StartElement) (string, error) {
	var buf strings.Builder
	buf.WriteString("<")
	writeElementName(&buf, start.Name)
	for _, a := range start.Attr {
		buf.WriteString(" ")
		writeAttrName(&buf, a.Name)
		buf.WriteString(`="`)
		buf.WriteString(xmlEscapeAttr(a.Value))
		buf.WriteString(`"`)
	}
	buf.WriteString(">")

	for {
		tok, err := d.Token()
		if err != nil {
			return "", err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "Fallback":
				// Drop the Fallback subtree entirely. Skip without
				// emitting anything — matches Okapi's
				// SkippableElement.GeneralInline.ALTERNATE_CONTENT_FALLBACK
				// behaviour described above.
				if err := skipElement(d); err != nil {
					return "", err
				}
			case "Choice":
				// Keep the Choice element verbatim, including its
				// Requires attribute and full subtree. Per the MCE
				// spec a Choice consumer MAY select the first
				// supported Choice — Okapi simply preserves every
				// Choice and lets the rendering pipeline decide,
				// which is byte-faithful to the source for any
				// document that already had its wrapper survive a
				// Word save/load round-trip.
				raw, err := captureRawElement(d, t)
				if err != nil {
					return "", err
				}
				buf.WriteString(raw)
			default:
				// Defensive: unknown child of mc:AlternateContent
				// (the schema only allows Choice and Fallback).
				// Preserve it verbatim so unusual documents don't
				// regress silently.
				raw, err := captureRawElement(d, t)
				if err != nil {
					return "", err
				}
				buf.WriteString(raw)
			}
		case xml.EndElement:
			if t.Name.Local == start.Name.Local {
				buf.WriteString("</")
				writeElementName(&buf, t.Name)
				buf.WriteString(">")
				return buf.String(), nil
			}
			// Should not happen for a well-formed document, but
			// emit the close tag defensively.
			buf.WriteString("</")
			writeElementName(&buf, t.Name)
			buf.WriteString(">")
		case xml.CharData:
			buf.WriteString(xmlEscape(string(t)))
		case xml.Comment:
			buf.WriteString("<!--")
			buf.Write(t)
			buf.WriteString("-->")
		}
	}
}

func xmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// xmlEscapeRune writes a single rune to a string builder, XML-escaping if needed.
func xmlEscapeRune(buf *strings.Builder, r rune) {
	switch r {
	case '&':
		buf.WriteString("&amp;")
	case '<':
		buf.WriteString("&lt;")
	case '>':
		buf.WriteString("&gt;")
	case '"':
		buf.WriteString("&quot;")
	default:
		buf.WriteRune(r)
	}
}
