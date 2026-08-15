// Tracked-revision preprocessing: byte-level passes over a part's raw XML that
// remove deleted rows, moveFrom ranges and the tables they empty before the
// streaming parser sees the bytes, plus the element-span scanning helpers
// those passes share.

package openxml

import (
	"bytes"
)

// dropDeletedRows removes every <w:tr ...>...</w:tr> region whose
// <w:trPr> carries a top-level <w:del> child — the row-deletion
// revision marker per ECMA-376 Part 1 §17.13.5.13 (CT_TrPrBase /
// `del`). The streaming parser's handleTableRow already strips
// these rows, but pre-stripping at the byte level lets dropEmptyTables
// collapse a table whose every row was deleted; otherwise the
// structurally-empty <w:tbl> would survive the round-trip (fixture
// 1080-1.docx table 2 with <w:tblpPr> positioning).
//
// Mirrors upstream Okapi's row-removal path:
// StyledTextPart.process() lines 530-551 (the
// RevisionPropertyTableRowDeletedSkippableElements.skip dispatch)
// removes the queued row markup; the downstream TableEnd branch
// (lines 410-424) then drops the whole table when no translatable
// block reached it. The context-aware `del` → `trPr` mapping is at
// SkippableElements.java lines 528-531
// (CONTEXT_AWARE_REVISION_SKIPPABLE_ELEMENTS).
//
// Nested rows (legal per the schema — a <w:tc> may contain another
// <w:tbl>) are handled correctly by tracking depth on <w:tr balanced
// open/close pairs.
func dropDeletedRows(data []byte) []byte {
	const trOpen = "<w:tr"
	const trClose = "</w:tr>"
	const trPrOpen = "<w:trPr"
	if !bytes.Contains(data, []byte(trPrOpen)) {
		// Fast path: no trPr means no row-deletion markers either.
		return data
	}
	out := make([]byte, 0, len(data))
	for {
		idx := bytes.Index(data, []byte(trOpen))
		if idx < 0 {
			out = append(out, data...)
			break
		}
		j := idx + len(trOpen)
		if j >= len(data) {
			out = append(out, data...)
			break
		}
		b := data[j]
		if b != '>' && b != '/' && b != ' ' && b != '\t' && b != '\n' && b != '\r' {
			// Not <w:tr; advance past this position.
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
		if startEnd > 0 && data[startEnd-1] == '/' {
			// Self-closing <w:tr/>: no <w:trPr>, never deleted.
			out = append(out, data[:startEnd+1]...)
			data = data[startEnd+1:]
			continue
		}
		bodyStart := startEnd + 1
		depth := 1
		cursor := bodyStart
		for depth > 0 {
			nextOpen := bytes.Index(data[cursor:], []byte(trOpen))
			nextClose := bytes.Index(data[cursor:], []byte(trClose))
			if nextClose < 0 {
				out = append(out, data...)
				return out
			}
			if nextOpen >= 0 && nextOpen < nextClose {
				absOpen := cursor + nextOpen
				jj := absOpen + len(trOpen)
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
				// Misleading prefix (e.g. <w:trPr inside the body).
				cursor = cursor + nextOpen + len(trOpen)
				continue
			}
			cursor = cursor + nextClose + len(trClose)
			depth--
		}
		rowEnd := cursor // one past the last byte of </w:tr>
		body := data[bodyStart : rowEnd-len(trClose)]
		if rowBodyHasDeletedTrPr(body) {
			out = append(out, data[:idx]...)
			data = data[rowEnd:]
			continue
		}
		// Recurse into the row body so deleted rows nested inside a
		// retained outer row (a <w:tc> may host another <w:tbl> with
		// its own <w:tr>s) get pruned too. Without this descent the
		// outer row is appended verbatim and the inner deleted row
		// rides along into the merged document.xml — fixture
		// 848-nested-tables-with-revisions.docx is the canonical case
		// where every inner row carries `<w:trPr><w:del/></w:trPr>`
		// and the outer's row-skip pass leaves them in place. Per
		// ECMA-376-1 §17.4.78 (CT_Row) and §17.4.16 (CT_Cell), nested
		// tables are legal cell content; the row-deletion revision
		// (§17.13.5.13) applies independently at every depth.
		bodyCleaned := dropDeletedRows(body)
		out = append(out, data[:bodyStart]...)
		out = append(out, bodyCleaned...)
		out = append(out, data[rowEnd-len(trClose):rowEnd]...)
		data = data[rowEnd:]
	}
	return out
}

// rowBodyHasDeletedTrPr reports whether the captured row body's own
// direct-child <w:trPr> contains a top-level <w:del> element — the
// row-deletion revision marker per ECMA-376 Part 1 §17.13.5.13
// (CT_TrPrBase / `del`). Mirrors upstream Okapi's
// RevisionProperty.TABLE_ROW_DELETED context-aware skip
// (SkippableElements.java lines 528-531 — `del` keyed under parent
// `trPr`).
//
// Per the schema's `tblPrEx? trPr? content*` sequence the row's
// own trPr precedes any cell content. We locate it by finding the
// first <w:trPr> open tag and verifying no <w:tc>, <w:tbl>, or
// nested <w:tr> appears before it — otherwise the matched trPr
// belongs to a deeper nested row, not the outer row we're examining,
// and must be ignored so a deleted nested row doesn't drag its
// outer ancestor with it.
func rowBodyHasDeletedTrPr(body []byte) bool {
	const trPrOpen = "<w:trPr"
	idx := bytes.Index(body, []byte(trPrOpen))
	if idx < 0 {
		return false
	}
	// Validate element-name boundary so <w:trPrChange> doesn't match.
	j := idx + len(trPrOpen)
	if j >= len(body) {
		return false
	}
	b := body[j]
	if b != '>' && b != '/' && b != ' ' && b != '\t' && b != '\n' && b != '\r' {
		return false
	}
	// Reject if any nested container precedes this trPr — the trPr
	// then belongs to a deeper-nested row, not this one.
	prefix := body[:idx]
	for _, name := range [...]string{"<w:tc", "<w:tbl", "<w:tr"} {
		if pIdx := indexValidElement(prefix, name); pIdx >= 0 {
			return false
		}
	}
	// Find the closing `>` of the open tag and read through </w:trPr>.
	k := bytes.IndexByte(body[j:], '>')
	if k < 0 {
		return false
	}
	startEnd := j + k
	if startEnd > 0 && body[startEnd-1] == '/' {
		// Self-closing <w:trPr/> — no children, no row deletion.
		return false
	}
	closeIdx := bytes.Index(body[startEnd+1:], []byte("</w:trPr>"))
	if closeIdx < 0 {
		return false
	}
	raw := body[idx : startEnd+1+closeIdx+len("</w:trPr>")]
	return trPrHasRowDeletion(string(raw))
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
		// Determine which structural boundaries the span between the
		// two markers crosses. Mirrors upstream's table/row/parent
		// crossed flags (SkippableElements.java lines 415-426):
		//
		//   * crossesTable: a </w:tbl> end tag was traversed without
		//     a matching <w:tbl> start inside the span. Drop the whole
		//     enclosing table — upstream's
		//     removeComponentsFromLastWith(LOCAL_TABLE) + the
		//     TableEnd-branch table drop both fire.
		//
		//   * crossesRow: a </w:tr> end tag was traversed without a
		//     matching <w:tr> start. Drop from <w:tr> of the start
		//     marker through end of moveFromRangeEnd (or </w:tr> of
		//     the row containing it, whichever is later). Mirrors
		//     removeComponentsFromLastWith(LOCAL_TABLE_ROW) plus the
		//     consumed events between rows.
		//
		// Cell-only crossings (</w:tc>) without a row crossing collapse
		// to the row-drop case as well: even a same-row cross-cell
		// moveFromRange leaves the row's translatable content in
		// disarray (cells dropped from delayedTableMarkup), and
		// upstream's outer loop drops the row's downstream cells via
		// the skip's event consumption. The simpler byte-level model
		// drops the whole row.
		crossesTable, crossesRow, crossesCell := spanCrossesTableStructure(data[startTagEnd+1 : endStart])
		if crossesTable || crossesRow || crossesCell {
			scope := "tr"
			if crossesTable {
				scope = "tbl"
			}
			dropFrom := findEnclosingElementOpenStart(data, startIdx, scope)
			if dropFrom < 0 {
				// Defensive: marker is supposed to be inside a row or
				// table but we couldn't find the enclosing element.
				// Bail: leave the start marker, skip past it.
				out = append(out, data[cursor:startTagEnd+1]...)
				cursor = startTagEnd + 1
				continue
			}
			// Drop-to endpoint: extend through </w:tr> (or </w:tbl>)
			// of the element containing moveFromRangeEnd when the end
			// marker sits inside one. Otherwise stop after the end
			// marker itself (sibling-position case).
			dropTo := endTagEnd + 1
			if enclosingClose := findEnclosingElementCloseEnd(data, endTagEnd+1, scope); enclosingClose >= 0 {
				dropTo = enclosingClose
			}
			out = append(out, data[cursor:dropFrom]...)
			cursor = dropTo
			continue
		}
		// Locate the enclosing <w:p> open tag for the start marker
		// (search backwards from startIdx). If startIdx is at body
		// level (not inside any <w:p>), keep startIdx as-is so we
		// only drop from the start marker forward.
		var dropFrom int
		startInsideP := isInsideParagraph(data, startIdx)
		pOpenStartForStart := -1
		if startInsideP {
			pOpenStartForStart = findEnclosingParagraphOpenStart(data, startIdx)
			if pOpenStartForStart < 0 {
				// Defensive: should not happen if isInsideParagraph
				// said yes, but bail safely.
				out = append(out, data[cursor:endTagEnd+1]...)
				cursor = endTagEnd + 1
				continue
			}
			dropFrom = pOpenStartForStart
		} else {
			dropFrom = startIdx
		}
		// Drop endpoint depends on where the end marker sits.
		//
		//   * SAME paragraph as the start marker (no parentStructure
		//     crossed): drop only the byte span between (and
		//     including) the two markers. Mirrors upstream Okapi
		//     SkippableElements.MoveFromRevisionCrossStructure.skip
		//     (SkippableElements.java lines 402-434): the event walk
		//     consumes events from moveFromRangeStart through
		//     moveFromRangeEnd; when no parentStructure (<w:p>) end
		//     tag was traversed, parentStructureCrossed stays false
		//     and BlockParser does NOT mark the block as
		//     skipped(true) (BlockParser.java lines 267-274 only
		//     drops the block when the cross-structure skip marked
		//     it). The surrounding paragraph content (text, <w:ins>
		//     wrappers, <w:moveTo> already-accepted runs, sibling
		//     <w:r>s) survives verbatim. 843-1.docx is the canonical
		//     fixture: <w:moveFromRangeStart> and
		//     <w:moveFromRangeEnd> sit in the same paragraph,
		//     wrapping a single <w:moveFrom><w:r>...</w:r></w:moveFrom>
		//     that gets stripped, leaving "Moved text. Text 1. " (the
		//     accepted <w:moveTo> + plain text + accepted <w:ins>
		//     spaces).
		//
		//   * DIFFERENT paragraphs (parentStructure crossed): extend
		//     the drop through the enclosing </w:p> end tag of the
		//     paragraph containing the end marker, then re-emit a
		//     single synthetic empty <w:p/> in its place. Upstream
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
			pOpenStartForEnd := findEnclosingParagraphOpenStart(data, endStart)
			if pOpenStartForEnd < 0 {
				out = append(out, data[cursor:endTagEnd+1]...)
				cursor = endTagEnd + 1
				continue
			}
			if startInsideP && pOpenStartForEnd == pOpenStartForStart {
				// Same paragraph: drop only the marker-to-marker
				// span; the rest of the paragraph survives.
				dropFrom = startIdx
				dropTo = endTagEnd + 1
			} else {
				pCloseEnd := findEnclosingParagraphCloseEnd(data, endTagEnd+1)
				if pCloseEnd < 0 {
					out = append(out, data[cursor:endTagEnd+1]...)
					cursor = endTagEnd + 1
					continue
				}
				dropTo = pCloseEnd
				insertEmptyP = true
			}
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

// spanCrossesTableStructure inspects the byte slice between a
// moveFromRangeStart and the matching moveFromRangeEnd and reports
// which table-structural boundaries it crosses. Mirrors upstream
// Okapi's tableRowStructureCrossed / tableStructureCrossed flag
// bookkeeping in SkippableElements.MoveFromRevisionCrossStructure
// (SkippableElements.java lines 415-426): an end-element of the
// given local name with no matching start-element earlier in the
// span flips the corresponding "crossed" flag on.
//
// Returns (crossesTable, crossesRow, crossesCell). The caller picks
// the outermost crossed scope as the drop scope.
func spanCrossesTableStructure(span []byte) (crossesTable, crossesRow, crossesCell bool) {
	crossesCell = spanCrossesElement(span, "tc")
	crossesRow = spanCrossesElement(span, "tr")
	crossesTable = spanCrossesElement(span, "tbl")
	return
}

// spanCrossesElement reports whether the byte slice between a
// moveFromRangeStart and the matching moveFromRangeEnd crosses a
// </w:NAME> end tag without first opening a matching <w:NAME> inside
// the span. A crossing would mean dropping the span verbatim would
// unbalance the structure.
func spanCrossesElement(span []byte, name string) bool {
	open := "<w:" + name
	close := "</w:" + name + ">"
	depth := 0
	cursor := 0
	for cursor < len(span) {
		nextOpen := indexValidElement(span[cursor:], open)
		nextClose := bytes.Index(span[cursor:], []byte(close))
		if nextOpen < 0 && nextClose < 0 {
			return false
		}
		if nextClose < 0 || (nextOpen >= 0 && nextOpen < nextClose) {
			absOpen := cursor + nextOpen
			tagEnd := bytes.IndexByte(span[absOpen:], '>')
			if tagEnd < 0 {
				return false
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
	return false
}

// findEnclosingElementOpenStart searches backwards from pos for the
// nearest `<w:NAME>` (or `<w:NAME ...>`) start tag whose matching
// `</w:NAME>` lies AFTER pos. Returns the absolute index of the `<`
// byte, or -1 if pos is not inside any such element. The element-
// name boundary check disambiguates from longer-name siblings (e.g.
// `<w:tr` from `<w:trPr`, `<w:tbl` from `<w:tblGrid`).
func findEnclosingElementOpenStart(data []byte, pos int, name string) int {
	open := "<w:" + name
	close := "</w:" + name + ">"
	depth := 0
	cursor := pos
	for cursor > 0 {
		closeIdx := bytes.LastIndex(data[:cursor], []byte(close))
		openIdx := lastIndexValidElement(data[:cursor], open)
		if openIdx < 0 && closeIdx < 0 {
			return -1
		}
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

// findEnclosingElementCloseEnd searches forward from pos for the
// matching `</w:NAME>` end tag of the enclosing element (depth=0 at
// pos, so we want the first `</w:NAME>` not preceded by an unmatched
// `<w:NAME>`). Returns the absolute index ONE PAST the `>` of the
// end tag, or -1 if no match (i.e. pos is NOT inside an element of
// that name).
func findEnclosingElementCloseEnd(data []byte, pos int, name string) int {
	open := "<w:" + name
	close := "</w:" + name + ">"
	depth := 0
	cursor := pos
	for cursor < len(data) {
		nextOpen := indexValidElement(data[cursor:], open)
		nextClose := bytes.Index(data[cursor:], []byte(close))
		if nextClose < 0 {
			return -1
		}
		if nextOpen >= 0 && nextOpen < nextClose {
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
			return cursor + nextClose + len(close)
		}
		depth--
		cursor = cursor + nextClose + len(close)
	}
	return -1
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
// dropDeletedRows and dropMoveFromRanges: when those passes strip
// every row of a table, the structurally-empty <w:tbl> shell would
// otherwise reach the writer. Upstream Okapi removes these via
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
		// Recurse into the body first so any inner <w:tbl> that lost
		// all its rows in earlier passes (or other inner empty
		// tables) is collapsed BEFORE we test whether THIS table has
		// surviving rows. Without this descent, nested tables emptied
		// by dropDeletedRows linger inside an outer cell — the outer
		// `tableBodyHasRow` check looks at the outer's own rows so
		// the empty inner tbl rides along into the writer. Fixture
		// 848-nested-tables-with-revisions.docx is the canonical
		// case: every inner table's rows carry <w:trPr><w:del/></w:trPr>
		// (ECMA-376-1 §17.13.5.13) and after row removal the inner
		// `<w:tbl><w:tblPr/><w:tblGrid/></w:tbl>` shell would survive
		// into the merged document.xml; upstream Okapi drops it via
		// StyledTextPart.process lines 410-424 (the TableEnd branch
		// removing queued delayedTableMarkup when no translatable
		// block reached the writer).
		bodyCleaned := dropEmptyTables(body)
		if !tableBodyHasRow(bodyCleaned) {
			// Empty table — drop the whole region.
			out = append(out, data[:idx]...)
			data = data[tableEnd:]
			continue
		}
		// Splice the cleaned body back into the table region.
		out = append(out, data[:bodyStart]...)
		out = append(out, bodyCleaned...)
		out = append(out, data[tableEnd-len(tblClose):tableEnd]...)
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
