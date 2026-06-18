package structure

import (
	"reflect"
	"strings"
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
	var s strings.Builder
	for _, b := range c.Blocks {
		s.WriteString(b.SourceText())
	}
	return s.String()
}

// Blocks without geometry can't be reasoned about positionally, so Analyze must
// emit them as paragraphs in their original input order (never as a table).
func TestAnalyze_NoGeometry(t *testing.T) {
	blocks := []*model.Block{
		model.NewBlock("a", "first"),
		model.NewBlock("b", "second"),
		model.NewBlock("c", "third"),
	}
	regions := Analyze(blocks)
	if len(regions) != 3 {
		t.Fatalf("regions = %d, want 3", len(regions))
	}
	for i, want := range []string{"first", "second", "third"} {
		if regions[i].Kind != RegionBlock || regions[i].Role != model.RoleParagraph {
			t.Errorf("region %d = %+v, want paragraph block", i, regions[i])
		}
		if got := regions[i].Block.SourceText(); got != want {
			t.Errorf("region %d text = %q, want %q (input order preserved)", i, got, want)
		}
	}
}

// A mix of geometry-bearing and geometry-less blocks: the positioned ones are
// analyzed, the rest are appended as paragraphs (so nothing is dropped).
func TestAnalyze_MixedGeometry(t *testing.T) {
	blocks := []*model.Block{
		blk("h", "Title", 0, 0, 200, 20),
		model.NewBlock("x", "no geometry here"),
	}
	regions := Analyze(blocks)
	if len(regions) != 2 {
		t.Fatalf("regions = %d, want 2", len(regions))
	}
	if regions[1].Block.SourceText() != "no geometry here" || regions[1].Role != model.RoleParagraph {
		t.Errorf("geometry-less block must be appended as a paragraph, got %+v", regions[1])
	}
}

// Heading level scales with line height relative to the page median: a line ~2×
// the median is H1, ~1.5× is H2, ~1.3× is H3.
func TestAnalyze_HeadingLevels(t *testing.T) {
	// Five body lines (height 10) keep the page median at 10; the three taller
	// lines are headings sized 2.0× / 1.5× / 1.3× the median.
	blocks := []*model.Block{
		blk("h1", "Huge", 0, 0, 100, 20),    // 2.0× → level 1
		blk("h2", "Big", 0, 30, 100, 15),    // 1.5× → level 2
		blk("h3", "Medium", 0, 60, 100, 13), // 1.3× → level 3
		blk("b1", "body one", 0, 90, 100, 10),
		blk("b2", "body two", 0, 110, 100, 10),
		blk("b3", "body three", 0, 130, 100, 10),
		blk("b4", "body four", 0, 150, 100, 10),
		blk("b5", "body five", 0, 170, 100, 10),
	}
	regions := Analyze(blocks)
	want := []struct {
		role  string
		level int
	}{
		{model.RoleHeading, 1},
		{model.RoleHeading, 2},
		{model.RoleHeading, 3},
		{model.RoleParagraph, 0},
		{model.RoleParagraph, 0},
		{model.RoleParagraph, 0},
		{model.RoleParagraph, 0},
		{model.RoleParagraph, 0},
	}
	if len(regions) != len(want) {
		t.Fatalf("regions = %d, want %d: %+v", len(regions), len(want), regions)
	}
	for i, w := range want {
		if regions[i].Role != w.role || regions[i].Level != w.level {
			t.Errorf("region %d = role %q level %d, want role %q level %d",
				i, regions[i].Role, regions[i].Level, w.role, w.level)
		}
	}
}

func TestAnalyze_Empty(t *testing.T) {
	if r := Analyze(nil); len(r) != 0 {
		t.Errorf("Analyze(nil) = %+v, want empty", r)
	}
}

// ToParts turns regions into the docling-shaped Part stream: prose blocks carry
// their role; a table becomes a GroupStart("table") wrapping GroupStart("table-row")
// groups of cell blocks (table-header for the header row, table-cell otherwise).
// Group open/close IDs must be balanced and stable.
func TestToParts(t *testing.T) {
	regions := []Region{
		{Kind: RegionBlock, Block: model.NewBlock("h", "Heading"), Role: model.RoleHeading, Level: 2},
		{Kind: RegionTable, Table: &Table{Rows: [][]Cell{
			{{Blocks: []*model.Block{model.NewBlock("c1", "A")}, Header: true}, {Blocks: []*model.Block{model.NewBlock("c2", "B")}, Header: true}},
			{{Blocks: []*model.Block{model.NewBlock("c3", "1")}}, {Blocks: []*model.Block{model.NewBlock("c4", "2")}}},
		}}},
		{Kind: RegionBlock, Block: model.NewBlock("p", "Body"), Role: model.RoleParagraph},
	}
	counter := 0
	parts := ToParts(regions, &counter)

	// Walk the stream: verify group nesting is balanced and capture the block roles
	// in order.
	var stack []string
	var roles []string
	var tableGroups, rowGroups int
	for _, p := range parts {
		switch p.Type {
		case model.PartGroupStart:
			g := p.Resource.(*model.GroupStart)
			if g.ID == "" {
				t.Errorf("group start with empty ID: %+v", g)
			}
			if g.Type == "table" {
				tableGroups++
			}
			if g.Type == "table-row" {
				rowGroups++
			}
			stack = append(stack, g.ID)
		case model.PartGroupEnd:
			g := p.Resource.(*model.GroupEnd)
			if len(stack) == 0 || stack[len(stack)-1] != g.ID {
				t.Fatalf("unbalanced group end %q (stack %v)", g.ID, stack)
			}
			stack = stack[:len(stack)-1]
		case model.PartBlock:
			roles = append(roles, p.Resource.(*model.Block).SemanticRole())
		}
	}
	if len(stack) != 0 {
		t.Errorf("unclosed groups: %v", stack)
	}
	if tableGroups != 1 || rowGroups != 2 {
		t.Errorf("groups: table=%d row=%d, want 1 and 2", tableGroups, rowGroups)
	}
	wantRoles := []string{
		model.RoleHeading,
		model.RoleTableHeader, model.RoleTableHeader,
		model.RoleTableCell, model.RoleTableCell,
		model.RoleParagraph,
	}
	if !reflect.DeepEqual(roles, wantRoles) {
		t.Errorf("block roles = %v, want %v", roles, wantRoles)
	}

	// IDs are stable across runs: same input → same group IDs.
	counter2 := 0
	parts2 := ToParts(regions, &counter2)
	if len(parts) != len(parts2) {
		t.Fatalf("part count not deterministic: %d vs %d", len(parts), len(parts2))
	}
	for i := range parts {
		if g1, ok := parts[i].Resource.(*model.GroupStart); ok {
			g2 := parts2[i].Resource.(*model.GroupStart)
			if g1.ID != g2.ID {
				t.Errorf("group ID not stable at %d: %q vs %q", i, g1.ID, g2.ID)
			}
		}
	}
}
