package structure

import (
	"fmt"
	"strconv"

	"github.com/neokapi/neokapi/core/model"
)

// cellGridOrigin is the GeometryAnnotation.Origin a spreadsheet reader stamps on
// a cell block: the BBox X/Y are the zero-based column/row and W/H are 1 (one
// cell). See core/formats/openxml/sml.go.
const cellGridOrigin = "cell-grid"

type gridCell struct {
	b              *model.Block
	col, row, page int
}

func gridCellOf(b *model.Block) (gridCell, bool) {
	g, ok := b.Geometry()
	if !ok || g == nil || g.Origin != cellGridOrigin {
		return gridCell{}, false
	}
	return gridCell{b: b, col: int(g.BBox.X), row: int(g.BBox.Y), page: g.Page}, true
}

func blockResource(p *model.Part) *model.Block {
	if p == nil || p.Type != model.PartBlock {
		return nil
	}
	b, _ := p.Resource.(*model.Block)
	return b
}

// SpreadsheetGridToTables rewrites a materialized part stream so flat spreadsheet
// cells become canonical table groups (GroupStart "table"/"table-row" plus
// RoleTableCell / RoleTableHeader), letting a structural writer (markdown, html,
// asciidoc, doclang) render a real table on cross-format export instead of a
// flat list of cell values.
//
// Cells are recognized purely by their cell-grid geometry (Origin "cell-grid";
// BBox X/Y = col/row), so the transform is format-agnostic — any reader that
// stamps that geometry benefits. Cells are grouped into one table per geometry
// Page (worksheet), the first row is marked as the header, and every cell gets a
// "column" property so column-aware writers align sparse rows while sequential
// writers still see a filled rectangle.
//
// A cell that references a deduplicated shared block via the "siIndex" property
// is the canonical position+text for that string; the now-redundant standalone
// shared block (same siIndex, no cell geometry) is dropped so it is not also
// rendered as a loose paragraph. Streams without any cell-grid blocks are
// returned unchanged, so non-spreadsheet exports are unaffected.
func SpreadsheetGridToTables(parts []*model.Part, groupCounter *int) []*model.Part {
	// Pass 1: which shared-string indices are represented by a grid cell, so the
	// deduplicated source blocks they came from can be dropped.
	claimed := map[string]bool{}
	hasGrid := false
	for _, p := range parts {
		b := blockResource(p)
		if b == nil {
			continue
		}
		if _, ok := gridCellOf(b); ok {
			hasGrid = true
			if si := b.Properties["siIndex"]; si != "" {
				claimed[si] = true
			}
		}
	}
	if !hasGrid {
		return parts
	}

	// Pass 2: replace each contiguous run of grid cells (a worksheet's cells sit
	// between its layer markers) with table groups; drop the now-redundant
	// shared-string source blocks; pass everything else through in order.
	out := make([]*model.Part, 0, len(parts))
	var run []*model.Block
	flush := func() {
		if len(run) == 0 {
			return
		}
		out = append(out, gridTableParts(run, groupCounter)...)
		run = nil
	}
	for _, p := range parts {
		b := blockResource(p)
		switch {
		case b != nil && isGridCell(b):
			run = append(run, b)
		case b != nil && isRedundantForGrid(b, claimed):
			// Represented in the grid already — drop.
		default:
			flush()
			out = append(out, p)
		}
	}
	flush()
	return out
}

func isGridCell(b *model.Block) bool {
	_, ok := gridCellOf(b)
	return ok
}

// isRedundantForGrid reports whether a non-grid block merely duplicates content
// the grid already carries, so it should not also render as a loose paragraph:
//   - a deduplicated shared-string source block whose index a grid cell claims;
//   - an Excel table-column name block, which exists only to keep the worksheet
//     valid on round-trip and repeats the header row's text.
func isRedundantForGrid(b *model.Block, claimed map[string]bool) bool {
	if isGridCell(b) {
		return false
	}
	if b.Type == "table-column" {
		return true
	}
	si := b.Properties["siIndex"]
	return si != "" && claimed[si]
}

// gridTableParts emits one table per geometry Page, in first-seen page order.
func gridTableParts(cells []*model.Block, groupCounter *int) []*model.Part {
	var pageOrder []int
	byPage := map[int][]gridCell{}
	for _, b := range cells {
		gc, ok := gridCellOf(b)
		if !ok {
			continue
		}
		if _, seen := byPage[gc.page]; !seen {
			pageOrder = append(pageOrder, gc.page)
		}
		byPage[gc.page] = append(byPage[gc.page], gc)
	}

	var parts []*model.Part
	for _, page := range pageOrder {
		parts = append(parts, oneTableParts(byPage[page], groupCounter)...)
	}
	return parts
}

// oneTableParts renders a single worksheet's cells as a dense table: a rectangle
// spanning the populated extent, row 0 as the header, each cell tagged with its
// column index. Missing cells become empty placeholders so sequential renderers
// keep alignment.
func oneTableParts(cells []gridCell, groupCounter *int) []*model.Part {
	maxRow, maxCol := 0, 0
	at := map[[2]int]*model.Block{}
	for _, gc := range cells {
		if gc.row > maxRow {
			maxRow = gc.row
		}
		if gc.col > maxCol {
			maxCol = gc.col
		}
		at[[2]int{gc.row, gc.col}] = gc.b
	}

	*groupCounter++
	tid := fmt.Sprintf("xltbl%d", *groupCounter)
	parts := []*model.Part{{Type: model.PartGroupStart, Resource: &model.GroupStart{ID: tid, Name: "table", Type: "table"}}}

	for row := 0; row <= maxRow; row++ {
		rid := fmt.Sprintf("%sr%d", tid, row)
		header := row == 0
		rowProps := map[string]string{}
		if header {
			rowProps["header"] = "true"
		}
		parts = append(parts, &model.Part{Type: model.PartGroupStart, Resource: &model.GroupStart{ID: rid, Name: "table-row", Type: "table-row", Properties: rowProps}})

		role := model.RoleTableCell
		if header {
			role = model.RoleTableHeader
		}
		for col := 0; col <= maxCol; col++ {
			b := at[[2]int{row, col}]
			if b == nil {
				b = &model.Block{ID: fmt.Sprintf("%sc%d", rid, col), Targets: map[model.VariantKey]*model.Target{}}
			}
			b.Type = "table-cell"
			b.SetSemanticRole(role, 0)
			if b.Properties == nil {
				b.Properties = map[string]string{}
			}
			b.Properties["column"] = strconv.Itoa(col)
			parts = append(parts, &model.Part{Type: model.PartBlock, Resource: b})
		}
		parts = append(parts, &model.Part{Type: model.PartGroupEnd, Resource: &model.GroupEnd{ID: rid}})
	}
	parts = append(parts, &model.Part{Type: model.PartGroupEnd, Resource: &model.GroupEnd{ID: tid}})
	return parts
}
