// Table structure in WordprocessingML: the w:tbl / w:tr / w:tc group frames
// the parser pushes and pops, the horizontal (w:gridSpan) and vertical
// (w:vMerge) cell-merge state read from a captured w:tcPr, and the row-level
// element dispatch.

package openxml

import (
	"encoding/xml"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/internal/xmlesc"
	"github.com/neokapi/neokapi/core/model"
)

// gridSpanRe extracts the w:val of a <w:gridSpan> inside a captured <w:tcPr>.
var gridSpanRe = regexp.MustCompile(`<w:gridSpan[^>]*\bw:val="(\d+)"`)

// gridSpanFromTcPr returns the horizontal cell span declared by a captured
// <w:tcPr> (ECMA-376 §17.4.17 CT_TcPrBase/gridSpan), or 0 when unspecified.
func gridSpanFromTcPr(raw string) int {
	if m := gridSpanRe.FindStringSubmatch(raw); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return n
		}
	}
	return 0
}

// vMergeRe matches a <w:vMerge> inside a captured <w:tcPr>, capturing its
// optional w:val. A bare <w:vMerge/> (no val) means "continue".
var vMergeRe = regexp.MustCompile(`<w:vMerge(?:\s+w:val="([a-z]+)")?\s*/?>`)

// vMergeFromTcPr returns the vertical-merge state of a captured <w:tcPr>
// (ECMA-376 §17.4.85 CT_VMerge): "restart" begins a merge, "continue" (the
// default when w:val is absent) extends the one above, "" when no vMerge.
func vMergeFromTcPr(raw string) string {
	m := vMergeRe.FindStringSubmatch(raw)
	if m == nil {
		return ""
	}
	if m[1] == "restart" {
		return "restart"
	}
	return "continue"
}

// structFrame is one open table-structure group on the parser's stack.
type structFrame struct {
	kind string // "table" | "table-row"
	id   string
}

func (p *wmlParser) nextGroupID() string {
	p.groupCounter++
	return fmt.Sprintf("tg%d", p.groupCounter)
}

// openTableStruct emits a table/table-row Group (or bumps cell depth) when the
// parser enters a w:tbl / w:tr / w:tc. Additive — no skeleton bytes, so the
// byte-exact round-trip is unaffected. No-op when emitPart is unset.
func (p *wmlParser) openTableStruct(name string) {
	switch name {
	case "tbl":
		p.path.push("table")
	case "tr":
		p.path.push("row")
	case "tc":
		p.path.push("cell")
	}
	if p.emitPart == nil {
		return
	}
	switch name {
	case "tbl":
		id := p.nextGroupID()
		p.structStack = append(p.structStack, structFrame{kind: "table", id: id})
		p.emitPart(&model.Part{Type: model.PartGroupStart, Resource: &model.GroupStart{ID: id, Name: "table", Type: "table"}})
		p.vmergeOpen = map[int]*model.StructureAnnotation{} // vertical merges are table-scoped
	case "tr":
		id := p.nextGroupID()
		p.structStack = append(p.structStack, structFrame{kind: "table-row", id: id})
		p.emitPart(&model.Part{Type: model.PartGroupStart, Resource: &model.GroupStart{ID: id, Name: "table-row", Type: "table-row"}})
		p.cellCol = 0 // grid column cursor resets each row
	case "tc":
		p.cellDepth++
		p.pendingColSpan = 0 // a fresh cell; its tcPr (if any) sets the span
		p.pendingVMerge = ""
		p.cellSpan = 1
	}
}

// resolveCellVMerge applies the current cell's vertical-merge state (parsed from
// its w:tcPr) at its grid column. A "continue" cell extends the merge begun
// above by bumping that cell's RowSpan; a normal cell ends any open merge in the
// column. A "restart" cell is registered when its first paragraph creates the
// block (registerCellVMerge), since the StructureAnnotation to grow lives there.
func (p *wmlParser) resolveCellVMerge() {
	if p.vmergeOpen == nil {
		return
	}
	switch p.pendingVMerge {
	case "continue":
		if s := p.vmergeOpen[p.cellCol]; s != nil {
			s.RowSpan++
		}
	case "":
		delete(p.vmergeOpen, p.cellCol)
	}
}

// closeTableStruct closes the matching table/table-row Group (or drops cell
// depth) when the parser leaves a w:tbl / w:tr / w:tc. Driven from the single
// main-loop EndElement site, through which every such close flows.
func (p *wmlParser) closeTableStruct(name string) {
	switch name {
	case "tbl":
		p.path.pop("table")
	case "tr":
		p.path.pop("row")
	case "tc":
		p.path.pop("cell")
	}
	if p.emitPart == nil {
		return
	}
	switch name {
	case "tbl", "tr":
		kind := "table"
		if name == "tr" {
			kind = "table-row"
		}
		if n := len(p.structStack); n > 0 && p.structStack[n-1].kind == kind {
			f := p.structStack[n-1]
			p.structStack = p.structStack[:n-1]
			p.emitPart(&model.Part{Type: model.PartGroupEnd, Resource: &model.GroupEnd{ID: f.id}})
		}
		if name == "tbl" {
			p.vmergeOpen = nil // table closed; drop its merge bookkeeping
		}
	case "tc":
		if p.cellDepth > 0 {
			p.cellDepth--
		}
		p.cellCol += p.cellSpan // advance the column cursor past this cell
		p.cellSpan = 1
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
			pending = append(pending, xmlesc.Text(string(tt)))
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
				p.openTableStruct("tr")
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
			p.openTableStruct("tr")
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
	case "tc":
		// First cell of an accept-revisions row: track cell depth so its
		// paragraphs tag RoleTableCell (the matching close flows through the
		// main-loop EndElement site).
		p.skelWriteStartElement(t)
		p.openTableStruct("tc")
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
