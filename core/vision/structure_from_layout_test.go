package vision

import (
	"context"
	"errors"
	"testing"

	"github.com/neokapi/neokapi/core/model"
)

// fakeLayout is a LayoutEngine returning preset regions, for testing the
// host-side tier-3 enrichment without the ONNX plugin.
type fakeLayout struct {
	regions []Region
	err     error
}

func (f fakeLayout) Layout(_ context.Context, _ string, _ LayoutOptions) ([]Region, error) {
	return f.regions, f.err
}

func block(id, text string, x, y, w, h float64) *model.Block {
	b := model.NewBlock(id, text)
	b.SetGeometry(&model.GeometryAnnotation{BBox: model.Rect{X: x, Y: y, W: w, H: h}, Origin: "top-left"})
	return b
}

func TestOCRResultFromBlocks(t *testing.T) {
	blocks := []*model.Block{
		block("b1", "Heading", 0, 0, 200, 30),
		block("b2", "Body", 0, 40, 200, 20),
		model.NewBlock("b3", "No geometry"), // skipped (no geometry)
		block("b4", "", 0, 80, 50, 20),      // skipped (no text)
	}
	res := OCRResultFromBlocks(blocks, 400, 300)
	if res.Width != 400 || res.Height != 300 {
		t.Errorf("dims = %dx%d, want 400x300", res.Width, res.Height)
	}
	if len(res.Lines) != 2 {
		t.Fatalf("lines = %d, want 2 (text+geometry only)", len(res.Lines))
	}
	if res.Lines[0].Text != "Heading" || res.Lines[1].Text != "Body" {
		t.Errorf("lines = %q, %q", res.Lines[0].Text, res.Lines[1].Text)
	}
}

func TestStructureFromLayout(t *testing.T) {
	blocks := []*model.Block{
		block("b1", "Annual Report", 0, 0, 300, 30),
		block("b2", "Intro paragraph", 0, 40, 300, 20),
	}
	le := fakeLayout{regions: []Region{
		{Role: model.RoleHeading, BBox: model.Rect{X: 0, Y: 0, W: 300, H: 35}, ReadingOrder: 0},
		{Role: model.RoleParagraph, BBox: model.Rect{X: 0, Y: 38, W: 300, H: 24}, ReadingOrder: 1},
	}}
	counter, gc := 0, 0
	parts, err := StructureFromLayout(context.Background(), le, "/tmp/page.png", blocks, 300, 80, LayoutOptions{}, &counter, &gc)
	if err != nil {
		t.Fatal(err)
	}
	var roles []string
	for _, p := range parts {
		if p.Type == model.PartBlock {
			roles = append(roles, p.Resource.(*model.Block).SemanticRole())
		}
	}
	want := []string{model.RoleHeading, model.RoleParagraph}
	if len(roles) != len(want) {
		t.Fatalf("roles = %v, want %v", roles, want)
	}
	for i := range want {
		if roles[i] != want[i] {
			t.Errorf("role[%d] = %q, want %q", i, roles[i], want[i])
		}
	}
}

func TestStructureFromLayout_NoRegions(t *testing.T) {
	c, g := 0, 0
	parts, err := StructureFromLayout(context.Background(), fakeLayout{}, "/tmp/p.png", nil, 100, 100, LayoutOptions{}, &c, &g)
	if err != nil || parts != nil {
		t.Errorf("no regions → (%v, %v), want (nil, nil) so caller falls back to tier-2", parts, err)
	}
}

func TestStructureFromLayout_Error(t *testing.T) {
	c, g := 0, 0
	want := errors.New("layout boom")
	_, err := StructureFromLayout(context.Background(), fakeLayout{err: want}, "/tmp/p.png", nil, 100, 100, LayoutOptions{}, &c, &g)
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want %v", err, want)
	}
}
