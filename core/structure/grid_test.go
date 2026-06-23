package structure

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func gridBlock(id, text, cell, si string, col, row int) *model.Block {
	b := &model.Block{
		ID:         id,
		Type:       "cell",
		Source:     []model.Run{{Text: &model.TextRun{Text: text}}},
		Properties: map[string]string{"cell": cell},
	}
	if si != "" {
		b.Properties["siIndex"] = si
	}
	b.SetGeometry(&model.GeometryAnnotation{
		Page:   1,
		BBox:   model.Rect{X: float64(col), Y: float64(row), W: 1, H: 1},
		Origin: "cell-grid",
	})
	return b
}

func block(id, text, typ string, props map[string]string) *model.Block {
	return &model.Block{
		ID:         id,
		Type:       typ,
		Source:     []model.Run{{Text: &model.TextRun{Text: text}}},
		Properties: props,
	}
}

func layerStart(name string) *model.Part {
	return &model.Part{Type: model.PartLayerStart, Resource: &model.Layer{ID: "l-" + name, Name: name}}
}
func layerEnd(name string) *model.Part {
	return &model.Part{Type: model.PartLayerEnd, Resource: &model.Layer{ID: "l-" + name, Name: name}}
}
func blockPart(b *model.Block) *model.Part {
	return &model.Part{Type: model.PartBlock, Resource: b}
}

// collectGroups walks a part stream and returns the (type, header) of each table
// row and the texts of cells, so a test can assert structure without coupling to
// exact ids.
func tableShape(parts []*model.Part) (tables int, rows int, headerRows int, cellTexts []string) {
	for _, p := range parts {
		switch p.Type {
		case model.PartGroupStart:
			g, _ := p.Resource.(*model.GroupStart)
			switch g.Type {
			case "table":
				tables++
			case "table-row":
				rows++
				if g.Properties["header"] == "true" {
					headerRows++
				}
			}
		case model.PartBlock:
			b, _ := p.Resource.(*model.Block)
			if b.SemanticRole() == model.RoleTableCell || b.SemanticRole() == model.RoleTableHeader {
				cellTexts = append(cellTexts, runsText(b.Source))
			}
		}
	}
	return
}

func runsText(rs []model.Run) string {
	s := ""
	for _, r := range rs {
		if r.Text != nil {
			s += r.Text.Text
		}
	}
	return s
}

func TestSpreadsheetGridToTables(t *testing.T) {
	// A worksheet with two header cells (row 0) and one data row, plus the
	// deduplicated shared-string source blocks and an Excel table-column name —
	// both of which the grid already represents and must be dropped.
	parts := []*model.Part{
		layerStart("xl/sharedStrings.xml"),
		blockPart(block("ss0", "Name", "shared-string", map[string]string{"siIndex": "0"})),
		blockPart(block("ss1", "City", "shared-string", map[string]string{"siIndex": "1"})),
		layerEnd("xl/sharedStrings.xml"),
		layerStart("xl/worksheets/sheet1.xml"),
		blockPart(gridBlock("cA1", "Name", "A1", "0", 0, 0)),
		blockPart(gridBlock("cB1", "City", "B1", "1", 1, 0)),
		blockPart(gridBlock("cA2", "Ada", "A2", "", 0, 1)),
		blockPart(gridBlock("cB2", "Oslo", "B2", "", 1, 1)),
		blockPart(block("tc0", "Name", "table-column", map[string]string{"partPath": "xl/tables/table1.xml"})),
		layerEnd("xl/worksheets/sheet1.xml"),
	}

	counter := 0
	out := SpreadsheetGridToTables(parts, &counter)

	tables, rows, headerRows, cells := tableShape(out)
	assert.Equal(t, 1, tables, "one table")
	assert.Equal(t, 2, rows, "two rows (header + data)")
	assert.Equal(t, 1, headerRows, "the first row is the header")
	assert.Equal(t, []string{"Name", "City", "Ada", "Oslo"}, cells, "cells in row-major order")

	// The deduplicated shared-string blocks and the table-column name must not
	// survive as loose blocks (they would render as stray paragraphs).
	for _, p := range out {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			require.NotContains(t, []string{"ss0", "ss1", "tc0"}, b.ID, "redundant block %s should be dropped", b.ID)
		}
	}

	// Header cells carry the table-header role; data cells the table-cell role.
	var headerCells, dataCells int
	for _, p := range out {
		if p.Type != model.PartBlock {
			continue
		}
		switch p.Resource.(*model.Block).SemanticRole() {
		case model.RoleTableHeader:
			headerCells++
		case model.RoleTableCell:
			dataCells++
		}
	}
	assert.Equal(t, 2, headerCells)
	assert.Equal(t, 2, dataCells)
}

func TestSpreadsheetGridToTables_PassthroughWithoutGrid(t *testing.T) {
	// No cell-grid blocks → stream returned unchanged (non-spreadsheet exports
	// are unaffected).
	parts := []*model.Part{
		layerStart("word/document.xml"),
		blockPart(block("p1", "Hello", "paragraph", nil)),
		layerEnd("word/document.xml"),
	}
	counter := 0
	out := SpreadsheetGridToTables(parts, &counter)
	assert.Equal(t, parts, out)
	assert.Equal(t, 0, counter)
}

func gridBlockSpan(id, text, cell string, col, row, cols, rows int) *model.Block {
	b := &model.Block{
		ID:         id,
		Type:       "cell",
		Source:     []model.Run{{Text: &model.TextRun{Text: text}}},
		Properties: map[string]string{"cell": cell},
	}
	b.SetGeometry(&model.GeometryAnnotation{
		Page:   1,
		BBox:   model.Rect{X: float64(col), Y: float64(row), W: float64(cols), H: float64(rows)},
		Origin: "cell-grid",
	})
	return b
}

func TestSpreadsheetGridToTables_MergedCell(t *testing.T) {
	// A1 is merged across two columns (a banner header); the covered B1 position
	// must emit no cell, and A1 must carry ColSpan=2.
	parts := []*model.Part{
		layerStart("xl/worksheets/sheet1.xml"),
		blockPart(gridBlockSpan("a1", "Banner", "A1", 0, 0, 2, 1)),
		blockPart(gridBlock("a2", "x", "A2", "", 0, 1)),
		blockPart(gridBlock("b2", "y", "B2", "", 1, 1)),
		layerEnd("xl/worksheets/sheet1.xml"),
	}
	counter := 0
	out := SpreadsheetGridToTables(parts, &counter)

	// Header row has a single cell (the merge), spanning two columns.
	var headerCells []*model.Block
	inHeader := false
	for _, p := range out {
		switch p.Type {
		case model.PartGroupStart:
			g := p.Resource.(*model.GroupStart)
			inHeader = g.Type == "table-row" && g.Properties["header"] == "true"
		case model.PartGroupEnd:
			inHeader = false
		case model.PartBlock:
			if inHeader {
				headerCells = append(headerCells, p.Resource.(*model.Block))
			}
		}
	}
	require.Len(t, headerCells, 1, "merged header row emits one cell, not two")
	s, ok := headerCells[0].Structure()
	require.True(t, ok)
	assert.Equal(t, 2, s.ColSpan, "the banner cell spans two columns")
}

func TestSpreadsheetGridToTables_RegionSplit(t *testing.T) {
	// Two clusters separated by a blank column → two tables, not one wide sparse
	// table.
	parts := []*model.Part{
		layerStart("xl/worksheets/sheet1.xml"),
		// Cluster 1: cols 0-1, rows 0-1.
		blockPart(gridBlock("a", "A", "A1", "", 0, 0)),
		blockPart(gridBlock("b", "B", "B1", "", 1, 0)),
		blockPart(gridBlock("c", "1", "A2", "", 0, 1)),
		blockPart(gridBlock("d", "2", "B2", "", 1, 1)),
		// Cluster 2: cols 4-5, rows 0-1 (blank cols 2-3 between).
		blockPart(gridBlock("e", "X", "E1", "", 4, 0)),
		blockPart(gridBlock("f", "Y", "F1", "", 5, 0)),
		blockPart(gridBlock("g", "9", "E2", "", 4, 1)),
		blockPart(gridBlock("h", "8", "F2", "", 5, 1)),
		layerEnd("xl/worksheets/sheet1.xml"),
	}
	counter := 0
	out := SpreadsheetGridToTables(parts, &counter)
	tables, _, _, _ := tableShape(out)
	assert.Equal(t, 2, tables, "disjoint clusters split into two tables")
}
