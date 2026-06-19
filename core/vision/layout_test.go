package vision

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
)

func TestSortReadingOrder_TwoColumns(t *testing.T) {
	// Left column: x≈0 (two stacked regions); right column: x≈200.
	regions := []Region{
		{Role: model.RoleParagraph, BBox: model.Rect{X: 200, Y: 0, W: 180, H: 40}},  // right-top
		{Role: model.RoleParagraph, BBox: model.Rect{X: 0, Y: 60, W: 180, H: 40}},   // left-bottom
		{Role: model.RoleParagraph, BBox: model.Rect{X: 0, Y: 0, W: 180, H: 40}},    // left-top
		{Role: model.RoleParagraph, BBox: model.Rect{X: 200, Y: 60, W: 180, H: 40}}, // right-bottom
	}
	ordered := SortReadingOrder(regions)
	// Expect left column top→bottom, then right column top→bottom.
	wantX := []float64{0, 0, 200, 200}
	wantY := []float64{0, 60, 0, 60}
	for i := range ordered {
		if ordered[i].BBox.X != wantX[i] || ordered[i].BBox.Y != wantY[i] {
			t.Errorf("order[%d] = (%v,%v), want (%v,%v)", i, ordered[i].BBox.X, ordered[i].BBox.Y, wantX[i], wantY[i])
		}
		if ordered[i].ReadingOrder != i {
			t.Errorf("region %d ReadingOrder = %d, want %d", i, ordered[i].ReadingOrder, i)
		}
	}
}

func TestPartsFromLayout(t *testing.T) {
	regions := []Region{
		{Role: model.RoleHeading, BBox: model.Rect{X: 0, Y: 0, W: 300, H: 30}, ReadingOrder: 0},
		{Role: model.RoleTable, BBox: model.Rect{X: 0, Y: 40, W: 300, H: 60}, ReadingOrder: 1},
		{Role: model.RoleParagraph, BBox: model.Rect{X: 0, Y: 110, W: 300, H: 30}, ReadingOrder: 2},
	}
	res := &OCRResult{
		Width: 300, Height: 160,
		Lines: []OCRLine{
			{Text: "Title", BBox: model.Rect{X: 5, Y: 5, W: 100, H: 20}},
			{Text: "Cell A", BBox: model.Rect{X: 5, Y: 45, W: 80, H: 15}},
			{Text: "Cell B", BBox: model.Rect{X: 120, Y: 45, W: 80, H: 15}},
			{Text: "Body text here", BBox: model.Rect{X: 5, Y: 115, W: 200, H: 15}},
			{Text: "Stray", BBox: model.Rect{X: 5, Y: 300, W: 50, H: 15}}, // outside all regions
		},
	}
	counter, gc := 0, 0
	parts := PartsFromLayout(regions, res, &counter, &gc)

	var roles []string
	var tableGroups int
	for _, p := range parts {
		switch p.Type {
		case model.PartGroupStart:
			if g := p.Resource.(*model.GroupStart); g.Type == "table" {
				tableGroups++
			}
		case model.PartBlock:
			roles = append(roles, p.Resource.(*model.Block).SemanticRole())
		}
	}
	if tableGroups != 1 {
		t.Errorf("table groups = %d, want 1", tableGroups)
	}
	// heading, (table: 2 cells), paragraph, then the stray paragraph.
	want := []string{model.RoleHeading, model.RoleTableCell, model.RoleTableCell, model.RoleParagraph, model.RoleParagraph}
	if len(roles) != len(want) {
		t.Fatalf("roles = %v, want %v", roles, want)
	}
	for i := range want {
		if roles[i] != want[i] {
			t.Errorf("role[%d] = %q, want %q", i, roles[i], want[i])
		}
	}
}

func TestPartsFromLayout_TableRowsCells(t *testing.T) {
	// A 2-column × 3-row table region: header row + two body rows. Each cell is
	// one OCR line. Expect one table group, three table-row groups, the first
	// row's cells as table-header and the rest as table-cell.
	regions := []Region{
		{Role: model.RoleTable, BBox: model.Rect{X: 0, Y: 0, W: 400, H: 120}, ReadingOrder: 0},
	}
	res := &OCRResult{
		Width: 400, Height: 120,
		Lines: []OCRLine{
			{Text: "Name", BBox: model.Rect{X: 10, Y: 5, W: 80, H: 18}},
			{Text: "Price", BBox: model.Rect{X: 210, Y: 5, W: 80, H: 18}},
			{Text: "Apple", BBox: model.Rect{X: 10, Y: 45, W: 80, H: 18}},
			{Text: "1.20", BBox: model.Rect{X: 210, Y: 45, W: 80, H: 18}},
			{Text: "Pear", BBox: model.Rect{X: 10, Y: 85, W: 80, H: 18}},
			{Text: "0.90", BBox: model.Rect{X: 210, Y: 85, W: 80, H: 18}},
		},
	}
	counter, gc := 0, 0
	parts := PartsFromLayout(regions, res, &counter, &gc)

	var tableGroups, rowGroups int
	var roles []string
	for _, p := range parts {
		switch p.Type {
		case model.PartGroupStart:
			switch p.Resource.(*model.GroupStart).Type {
			case "table":
				tableGroups++
			case "table-row":
				rowGroups++
			}
		case model.PartBlock:
			roles = append(roles, p.Resource.(*model.Block).SemanticRole())
		}
	}
	if tableGroups != 1 {
		t.Errorf("table groups = %d, want 1", tableGroups)
	}
	if rowGroups != 3 {
		t.Errorf("table-row groups = %d, want 3", rowGroups)
	}
	want := []string{
		model.RoleTableHeader, model.RoleTableHeader,
		model.RoleTableCell, model.RoleTableCell,
		model.RoleTableCell, model.RoleTableCell,
	}
	if len(roles) != len(want) {
		t.Fatalf("roles = %v, want %v", roles, want)
	}
	for i := range want {
		if roles[i] != want[i] {
			t.Errorf("role[%d] = %q, want %q", i, roles[i], want[i])
		}
	}
}

func TestPartsFromLayout_Nil(t *testing.T) {
	c, g := 0, 0
	if p := PartsFromLayout(nil, nil, &c, &g); p != nil {
		t.Errorf("nil result → %v, want nil", p)
	}
}
