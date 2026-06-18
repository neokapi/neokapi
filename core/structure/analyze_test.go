package structure

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
)

func blk(id, text string, x, y, w, h float64) *model.Block {
	b := model.NewBlock(id, text)
	b.SetGeometry(&model.GeometryAnnotation{BBox: model.Rect{X: x, Y: y, W: w, H: h}, Origin: "top-left"})
	return b
}

// A page with a heading, a 3×3 table, and a trailing paragraph. Coordinates are
// top-left (Y increases downward); most lines are height 10, the heading 20.
func TestAnalyze_HeadingTableParagraph(t *testing.T) {
	var blocks []*model.Block
	blocks = append(blocks, blk("h", "Quarterly Results", 0, 0, 200, 20)) // tall → heading
	// table: 3 rows × 3 cols at x = 0/100/200, rows at y = 30/45/60.
	cells := [][]string{{"Q", "2024", "2025"}, {"Rev", "10", "12"}, {"Cost", "4", "5"}}
	id := 0
	for r, row := range cells {
		for c, txt := range row {
			id++
			blocks = append(blocks, blk(string(rune('a'+id)), txt, float64(c*100), float64(30+r*15), 50, 10))
		}
	}
	blocks = append(blocks, blk("p", "See notes for details.", 0, 90, 300, 10)) // paragraph

	regions := Analyze(blocks)

	if len(regions) != 3 {
		t.Fatalf("regions = %d, want 3 (heading, table, paragraph): %+v", len(regions), regions)
	}
	if regions[0].Kind != RegionBlock || regions[0].Role != model.RoleHeading {
		t.Errorf("region0 = %+v, want heading block", regions[0])
	}
	if regions[0].Level != 1 {
		t.Errorf("heading level = %d, want 1", regions[0].Level)
	}
	if regions[1].Kind != RegionTable {
		t.Fatalf("region1 kind = %v, want table", regions[1].Kind)
	}
	tbl := regions[1].Table
	if len(tbl.Rows) != 3 {
		t.Errorf("table rows = %d, want 3", len(tbl.Rows))
	}
	if len(tbl.Rows[0]) != 3 {
		t.Errorf("table cols = %d, want 3", len(tbl.Rows[0]))
	}
	if !tbl.Rows[0][0].Header {
		t.Errorf("first row should be header")
	}
	if tbl.Rows[1][0].Header {
		t.Errorf("second row should not be header")
	}
	if got := cellText(tbl.Rows[1][2]); got != "12" {
		t.Errorf("cell[1][2] = %q, want 12", got)
	}
	if regions[2].Kind != RegionBlock || regions[2].Role != model.RoleParagraph {
		t.Errorf("region2 = %+v, want paragraph block", regions[2])
	}
}

// Plain prose (stacked single-column lines) must NOT be detected as a table.
func TestAnalyze_ProseNoTable(t *testing.T) {
	blocks := []*model.Block{
		blk("l1", "The first line of a paragraph.", 0, 0, 300, 10),
		blk("l2", "The second line continues here.", 0, 15, 300, 10),
		blk("l3", "And a third line to finish.", 0, 30, 280, 10),
	}
	for _, r := range Analyze(blocks) {
		if r.Kind == RegionTable {
			t.Fatalf("single-column prose misdetected as table")
		}
	}
}

func cellText(c Cell) string {
	s := ""
	for _, b := range c.Blocks {
		s += b.SourceText()
	}
	return s
}
