// Package structure infers document structure (tables, headings, paragraphs)
// from positioned content blocks — the geometric "tier 2" between raw geometry
// and an ML layout model. It is format-agnostic: it consumes blocks that carry a
// model.GeometryAnnotation (e.g. from the PDFium reader) and produces a neutral
// Layout the caller maps onto the content model (table groups + structure roles).
//
// The heuristics are intentionally simple and deterministic: cluster blocks into
// rows by vertical overlap, then find runs of consecutive rows whose cells align
// into stable columns — that's a table. Everything else is prose, tagged heading
// or paragraph by relative size. Merged cells, nested tables, and borderless
// edge cases are out of scope here (that's the ML tier).
package structure

import (
	"fmt"
	"math"
	"sort"

	"github.com/neokapi/neokapi/core/model"
)

// RegionKind discriminates a Region.
type RegionKind int

const (
	// RegionBlock is a single prose block (heading or paragraph).
	RegionBlock RegionKind = iota
	// RegionTable is a detected table (rows of cells).
	RegionTable
)

// Region is one top-level piece of a page, in reading order.
type Region struct {
	Kind  RegionKind
	Block *model.Block // set when Kind == RegionBlock
	Role  string       // model.Role* for a RegionBlock (heading/paragraph)
	Level int          // heading level (1..) for RoleHeading; 0 otherwise
	Table *Table       // set when Kind == RegionTable
}

// Table is a detected grid. Rows are top-to-bottom; cells left-to-right.
type Table struct {
	Rows [][]Cell
}

// Cell is one table cell: the blocks that fall in it (usually one) and whether
// it is in the header row.
type Cell struct {
	Blocks []*model.Block
	Header bool
}

type placed struct {
	b          *model.Block
	x, y, w, h float64
	cx, cy     float64
}

// Analyze groups page blocks into a structural Layout. Blocks without geometry
// are emitted as paragraph regions in input order (nothing to reason about).
func Analyze(blocks []*model.Block) []Region {
	items := make([]placed, 0, len(blocks))
	var noGeo []*model.Block
	for _, b := range blocks {
		g, ok := b.Geometry()
		if !ok || g == nil || (g.BBox.W == 0 && g.BBox.H == 0) {
			noGeo = append(noGeo, b)
			continue
		}
		items = append(items, placed{
			b: b, x: g.BBox.X, y: g.BBox.Y, w: g.BBox.W, h: g.BBox.H,
			cx: g.BBox.X + g.BBox.W/2, cy: g.BBox.Y + g.BBox.H/2,
		})
	}
	if len(items) == 0 {
		out := make([]Region, len(noGeo))
		for i, b := range noGeo {
			out[i] = Region{Kind: RegionBlock, Block: b, Role: model.RoleParagraph}
		}
		return out
	}

	medianH := medianHeight(items)
	rows := groupRows(items, medianH)
	var regions []Region

	i := 0
	for i < len(rows) {
		// Try to start a table at row i: it needs ≥2 cells and at least one
		// following row whose cells align into the same columns.
		if span := tableSpan(rows, i, medianH); span >= 2 {
			regions = append(regions, Region{Kind: RegionTable, Table: buildTable(rows[i : i+span])})
			i += span
			continue
		}
		// Prose row(s): emit each block as heading/paragraph.
		for _, it := range rows[i] {
			regions = append(regions, Region{
				Kind: RegionBlock, Block: it.b,
				Role: proseRole(it, medianH), Level: headingLevel(it, medianH),
			})
		}
		i++
	}

	for _, b := range noGeo {
		regions = append(regions, Region{Kind: RegionBlock, Block: b, Role: model.RoleParagraph})
	}
	return regions
}

func medianHeight(items []placed) float64 {
	hs := make([]float64, len(items))
	for i, it := range items {
		hs[i] = it.h
	}
	sort.Float64s(hs)
	m := hs[len(hs)/2]
	if m <= 0 {
		m = 1
	}
	return m
}

// groupRows clusters blocks into visual rows (vertical-center proximity), each
// sorted left-to-right; rows are returned top-to-bottom.
func groupRows(items []placed, medianH float64) [][]placed {
	sorted := append([]placed(nil), items...)
	sort.SliceStable(sorted, func(a, b int) bool {
		if sorted[a].cy != sorted[b].cy {
			return sorted[a].cy < sorted[b].cy // top-left: smaller Y is higher
		}
		return sorted[a].x < sorted[b].x
	})
	var rows [][]placed
	var cur []placed
	var rowCy float64
	for _, it := range sorted {
		if len(cur) == 0 || math.Abs(it.cy-rowCy) <= 0.6*medianH {
			cur = append(cur, it)
			rowCy = (rowCy*float64(len(cur)-1) + it.cy) / float64(len(cur))
			continue
		}
		rows = append(rows, sortRow(cur))
		cur = []placed{it}
		rowCy = it.cy
	}
	if len(cur) > 0 {
		rows = append(rows, sortRow(cur))
	}
	return rows
}

func sortRow(r []placed) []placed {
	sort.SliceStable(r, func(a, b int) bool { return r[a].x < r[b].x })
	return r
}

// tableSpan returns how many consecutive rows starting at `start` form a table:
// the rows must each have ≥2 cells and share aligned column centers with the
// anchor row. Returns 0 when no table starts here.
func tableSpan(rows [][]placed, start int, medianH float64) int {
	anchor := rows[start]
	if len(anchor) < 2 {
		return 0
	}
	cols := make([]float64, len(anchor))
	for i, it := range anchor {
		cols[i] = it.cx
	}
	tol := 1.5 * medianH // column-center alignment tolerance
	span := 1
	for r := start + 1; r < len(rows); r++ {
		if !rowAligns(rows[r], cols, tol) {
			break
		}
		span++
	}
	if span < 2 {
		return 0
	}
	return span
}

// rowAligns reports whether every cell in the row matches one of the column
// centers within tol, and the row covers ≥2 of the columns (so a single stray
// block on its own line doesn't extend a table).
func rowAligns(row []placed, cols []float64, tol float64) bool {
	if len(row) < 2 {
		return false
	}
	hit := make([]bool, len(cols))
	for _, it := range row {
		matched := false
		for c, cc := range cols {
			if math.Abs(it.cx-cc) <= tol {
				hit[c] = true
				matched = true
				break
			}
		}
		if !matched {
			return false // a cell outside all columns → not the same table
		}
	}
	n := 0
	for _, h := range hit {
		if h {
			n++
		}
	}
	return n >= 2
}

// buildTable assigns each row's blocks to the anchor columns, header = first row.
func buildTable(rows [][]placed) *Table {
	cols := make([]float64, len(rows[0]))
	for i, it := range rows[0] {
		cols[i] = it.cx
	}
	t := &Table{}
	for ri, row := range rows {
		cells := make([]Cell, len(cols))
		for ci := range cells {
			cells[ci] = Cell{Header: ri == 0}
		}
		for _, it := range row {
			ci := nearestCol(it.cx, cols)
			cells[ci].Blocks = append(cells[ci].Blocks, it.b)
		}
		t.Rows = append(t.Rows, cells)
	}
	return t
}

func nearestCol(x float64, cols []float64) int {
	best, bestD := 0, math.Inf(1)
	for i, c := range cols {
		if d := math.Abs(x - c); d < bestD {
			best, bestD = i, d
		}
	}
	return best
}

// proseRole tags a non-table block: a noticeably taller line is a heading, else
// a paragraph.
func proseRole(it placed, medianH float64) string {
	if it.h >= 1.3*medianH {
		return model.RoleHeading
	}
	return model.RoleParagraph
}

// ToParts emits the regions as a Part stream matching the docling reader's
// structure: a table becomes a GroupStart("table") wrapping GroupStart("table-row")
// groups of cell Blocks (role table-cell / table-header), and a prose block is
// emitted with its heading/paragraph role. The cell/prose Blocks are the same
// objects Analyze was given, so their geometry and text are preserved. ToParts
// mutates those caller-owned Blocks: it sets each block's structure role and, for
// table cells, also sets Block.Type to "table-cell" (matching the docling reader's
// emission). groupCounter is advanced for unique group IDs across pages.
func ToParts(regions []Region, groupCounter *int) []*model.Part {
	var parts []*model.Part
	for _, reg := range regions {
		switch reg.Kind {
		case RegionBlock:
			if reg.Role != "" {
				reg.Block.SetSemanticRole(reg.Role, reg.Level)
			}
			parts = append(parts, &model.Part{Type: model.PartBlock, Resource: reg.Block})
		case RegionTable:
			*groupCounter++
			tid := fmt.Sprintf("tbl%d", *groupCounter)
			parts = append(parts, &model.Part{Type: model.PartGroupStart, Resource: &model.GroupStart{ID: tid, Name: "table", Type: "table"}})
			for _, row := range reg.Table.Rows {
				*groupCounter++
				rid := fmt.Sprintf("%sr%d", tid, *groupCounter)
				parts = append(parts, &model.Part{Type: model.PartGroupStart, Resource: &model.GroupStart{ID: rid, Name: "table-row", Type: "table-row"}})
				for _, cell := range row {
					role := model.RoleTableCell
					if cell.Header {
						role = model.RoleTableHeader
					}
					for _, b := range cell.Blocks {
						b.Type = "table-cell"
						b.SetSemanticRole(role, 0)
						parts = append(parts, &model.Part{Type: model.PartBlock, Resource: b})
					}
				}
				parts = append(parts, &model.Part{Type: model.PartGroupEnd, Resource: &model.GroupEnd{ID: rid}})
			}
			parts = append(parts, &model.Part{Type: model.PartGroupEnd, Resource: &model.GroupEnd{ID: tid}})
		}
	}
	return parts
}

func headingLevel(it placed, medianH float64) int {
	switch {
	case it.h >= 2.0*medianH:
		return 1
	case it.h >= 1.5*medianH:
		return 2
	case it.h >= 1.3*medianH:
		return 3
	default:
		return 0
	}
}
