package structure

import (
	"fmt"
	"math"
	"sort"
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
		for _, region := range splitRegions(byPage[page]) {
			parts = append(parts, regionParts(region, groupCounter)...)
		}
	}
	return parts
}

// cellSpan reads a cell's merged extent (columns × rows) from its geometry BBox
// W/H, defaulting to 1×1 for an unmerged or geometry-less cell.
func cellSpan(b *model.Block) (cols, rows int) {
	g, ok := b.Geometry()
	if !ok || g == nil {
		return 1, 1
	}
	cols, rows = int(g.BBox.W), int(g.BBox.H)
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	return cols, rows
}

// splitRegions partitions a worksheet's cells into disjoint table regions:
// connected components over occupied grid positions (4-neighbour adjacency,
// accounting for merged-cell coverage). A sheet with a data table and a separate
// criteria block (separated by a blank row/column) yields two regions rather
// than one wide sparse table. Components are returned in reading order (the
// top-left-most origin first).
func splitRegions(cells []gridCell) [][]gridCell {
	origin := map[[2]int]gridCell{}
	covered := map[[2]int]bool{}
	for _, gc := range cells {
		cols, rows := cellSpan(gc.b)
		origin[[2]int{gc.row, gc.col}] = gc
		for dr := range rows {
			for dc := range cols {
				covered[[2]int{gc.row + dr, gc.col + dc}] = true
			}
		}
	}

	keys := make([][2]int, 0, len(origin))
	for k := range origin {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i][0] != keys[j][0] {
			return keys[i][0] < keys[j][0]
		}
		return keys[i][1] < keys[j][1]
	})

	comp := map[[2]int]int{}
	next := 0
	neigh := [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}}
	for _, k := range keys {
		if _, ok := comp[k]; ok {
			continue
		}
		id := next
		next++
		queue := [][2]int{k}
		comp[k] = id
		for len(queue) > 0 {
			q := queue[0]
			queue = queue[1:]
			for _, d := range neigh {
				n := [2]int{q[0] + d[0], q[1] + d[1]}
				if covered[n] {
					if _, ok := comp[n]; !ok {
						comp[n] = id
						queue = append(queue, n)
					}
				}
			}
		}
	}

	groups := make([][]gridCell, next)
	for _, k := range keys {
		groups[comp[k]] = append(groups[comp[k]], origin[k])
	}
	return groups
}

// regionParts renders one region. A region of at least two rows and two columns
// becomes a table (its first row the header); anything smaller (a stray label or
// single column) renders as loose blocks so it is not forced into a 1×N table.
func regionParts(region []gridCell, groupCounter *int) []*model.Part {
	origin := map[[2]int]*model.Block{}
	covered := map[[2]int]bool{}
	minRow, minCol := math.MaxInt, math.MaxInt
	maxRow, maxCol := 0, 0
	for _, gc := range region {
		cols, rows := cellSpan(gc.b)
		origin[[2]int{gc.row, gc.col}] = gc.b
		if gc.row < minRow {
			minRow = gc.row
		}
		if gc.col < minCol {
			minCol = gc.col
		}
		for dr := range rows {
			for dc := range cols {
				pr, pc := gc.row+dr, gc.col+dc
				covered[[2]int{pr, pc}] = true
				if pr > maxRow {
					maxRow = pr
				}
				if pc > maxCol {
					maxCol = pc
				}
			}
		}
	}

	if maxRow-minRow < 1 || maxCol-minCol < 1 {
		return looseBlocks(region)
	}
	return tableParts(origin, covered, minRow, minCol, maxRow, maxCol, groupCounter)
}

// looseBlocks emits a region's cells as plain blocks in reading order (used for
// degenerate single-row / single-column regions).
func looseBlocks(region []gridCell) []*model.Part {
	sort.Slice(region, func(i, j int) bool {
		if region[i].row != region[j].row {
			return region[i].row < region[j].row
		}
		return region[i].col < region[j].col
	})
	parts := make([]*model.Part, 0, len(region))
	for _, gc := range region {
		parts = append(parts, &model.Part{Type: model.PartBlock, Resource: gc.b})
	}
	return parts
}

// tableParts renders a region's rectangle as a table. Cells carry their
// region-local column index; merged cells carry ColSpan/RowSpan and the
// positions they cover emit no cell; genuine gaps emit an empty placeholder so
// sequential renderers keep alignment.
func tableParts(origin map[[2]int]*model.Block, covered map[[2]int]bool, minRow, minCol, maxRow, maxCol int, groupCounter *int) []*model.Part {
	*groupCounter++
	tid := fmt.Sprintf("xltbl%d", *groupCounter)
	parts := []*model.Part{{Type: model.PartGroupStart, Resource: &model.GroupStart{ID: tid, Name: "table", Type: "table"}}}

	for row := minRow; row <= maxRow; row++ {
		rid := fmt.Sprintf("%sr%d", tid, row)
		header := row == minRow
		rowProps := map[string]string{}
		if header {
			rowProps["header"] = "true"
		}
		parts = append(parts, &model.Part{Type: model.PartGroupStart, Resource: &model.GroupStart{ID: rid, Name: "table-row", Type: "table-row", Properties: rowProps}})

		role := model.RoleTableCell
		if header {
			role = model.RoleTableHeader
		}
		for col := minCol; col <= maxCol; col++ {
			key := [2]int{row, col}
			b, isOrigin := origin[key]
			if !isOrigin {
				if covered[key] {
					// Covered by a merged cell that originates elsewhere — its span
					// accounts for this position, so emit no cell here.
					continue
				}
				b = &model.Block{ID: fmt.Sprintf("%sc%d", rid, col), Targets: map[model.VariantKey]*model.Target{}}
			}
			b.Type = "table-cell"
			b.SetSemanticRole(role, 0)
			if cols, rows := cellSpan(b); cols > 1 || rows > 1 {
				s, _ := b.Structure()
				if s == nil {
					s = &model.StructureAnnotation{Role: role}
				}
				s.ColSpan = cols
				s.RowSpan = rows
				b.SetStructure(s)
			}
			if b.Properties == nil {
				b.Properties = map[string]string{}
			}
			b.Properties["column"] = strconv.Itoa(col - minCol)
			parts = append(parts, &model.Part{Type: model.PartBlock, Resource: b})
		}
		parts = append(parts, &model.Part{Type: model.PartGroupEnd, Resource: &model.GroupEnd{ID: rid}})
	}
	parts = append(parts, &model.Part{Type: model.PartGroupEnd, Resource: &model.GroupEnd{ID: tid}})
	return parts
}
