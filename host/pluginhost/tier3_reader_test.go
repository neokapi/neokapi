package pluginhost

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/vision"
)

type fakeLayoutEngine struct{ regions []vision.Region }

func (f fakeLayoutEngine) Layout(_ context.Context, _ string, _ vision.LayoutOptions) ([]vision.Region, error) {
	return f.regions, nil
}

func geomBlock(id, text string, x, y, w, h float64) *model.Block {
	b := model.NewBlock(id, text)
	b.SetGeometry(&model.GeometryAnnotation{BBox: model.Rect{X: x, Y: y, W: w, H: h}, Origin: "top-left"})
	return b
}

// tier3PageStream simulates the kapi-pdfium tier3 emission: root layer → page
// layer → page raster Media → raw positioned blocks → ends. The raster points at
// a real temp file so the consumer can delete it.
func tier3PageStream(t *testing.T, rasterPath string) []*model.Part {
	t.Helper()
	root := &model.Layer{ID: "doc1", Name: "doc"}
	page := &model.Layer{ID: "page1", Name: "Page 1"}
	raster := &model.Media{
		ID: "raster1", MimeType: "image/png", URI: rasterPath,
		Properties: map[string]string{"width": "300", "height": "80", vision.PageRasterProperty: "page"},
	}
	return []*model.Part{
		{Type: model.PartLayerStart, Resource: root},
		{Type: model.PartLayerStart, Resource: page},
		{Type: model.PartMedia, Resource: raster},
		{Type: model.PartBlock, Resource: geomBlock("b1", "Annual Report", 0, 0, 300, 30)},
		{Type: model.PartBlock, Resource: geomBlock("b2", "Intro text", 0, 40, 300, 20)},
		{Type: model.PartLayerEnd, Resource: page},
		{Type: model.PartLayerEnd, Resource: root},
	}
}

func runEnrich(t *testing.T, parts []*model.Part, le vision.LayoutEngine) []*model.Part {
	t.Helper()
	in := make(chan model.PartResult, len(parts))
	for _, p := range parts {
		in <- model.PartResult{Part: p}
	}
	close(in)
	out := make(chan model.PartResult, 64)
	go func() {
		defer close(out)
		enrichTier3(context.Background(), in, out, le)
	}()
	var got []*model.Part
	for res := range out {
		if res.Error != nil {
			t.Fatalf("enrich error: %v", res.Error)
		}
		got = append(got, res.Part)
	}
	return got
}

// With a layout engine, a page's raster + blocks become tier-3 structure: the
// raster is consumed (not forwarded) and deleted, blocks carry model roles.
func TestEnrichTier3_WithLayout(t *testing.T) {
	dir := t.TempDir()
	rasterPath := filepath.Join(dir, "page1.png")
	if err := os.WriteFile(rasterPath, []byte("PNGDATA"), 0o644); err != nil {
		t.Fatal(err)
	}
	le := fakeLayoutEngine{regions: []vision.Region{
		{Role: model.RoleHeading, BBox: model.Rect{X: 0, Y: 0, W: 300, H: 35}, ReadingOrder: 0},
		{Role: model.RoleParagraph, BBox: model.Rect{X: 0, Y: 38, W: 300, H: 24}, ReadingOrder: 1},
	}}

	got := runEnrich(t, tier3PageStream(t, rasterPath), le)

	var layerStarts, layerEnds, medias int
	var roles []string
	for _, p := range got {
		switch p.Type {
		case model.PartLayerStart:
			layerStarts++
		case model.PartLayerEnd:
			layerEnds++
		case model.PartMedia:
			medias++
		case model.PartBlock:
			roles = append(roles, p.Resource.(*model.Block).SemanticRole())
		}
	}
	if layerStarts != 2 || layerEnds != 2 {
		t.Errorf("layers: starts=%d ends=%d, want 2/2", layerStarts, layerEnds)
	}
	if medias != 0 {
		t.Errorf("raster Media should be consumed, got %d media parts", medias)
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
	if _, err := os.Stat(rasterPath); !os.IsNotExist(err) {
		t.Error("raster temp file should be deleted after enrichment")
	}
}

// If the stream ends mid-page (truncated / cancelled) before the page's
// LayerEnd, the in-progress raster temp file is still removed (deferred cleanup),
// not leaked.
func TestEnrichTier3_CleansRasterOnTruncatedStream(t *testing.T) {
	dir := t.TempDir()
	rasterPath := filepath.Join(dir, "page1.png")
	if err := os.WriteFile(rasterPath, []byte("PNGDATA"), 0o644); err != nil {
		t.Fatal(err)
	}
	full := tier3PageStream(t, rasterPath)
	// Drop the page LayerEnd and the root LayerEnd → stream stops mid-page.
	truncated := full[:len(full)-2]

	le := fakeLayoutEngine{regions: []vision.Region{
		{Role: model.RoleParagraph, BBox: model.Rect{X: 0, Y: 0, W: 300, H: 80}, ReadingOrder: 0},
	}}
	_ = runEnrich(t, truncated, le)

	if _, err := os.Stat(rasterPath); !os.IsNotExist(err) {
		t.Error("raster temp file should be removed even when the stream is truncated mid-page")
	}
}

// With no layout engine, it falls back to the geometric tier-2 over the same
// blocks (still consuming/deleting the raster).
func TestEnrichTier3_FallbackTier2(t *testing.T) {
	dir := t.TempDir()
	rasterPath := filepath.Join(dir, "page1.png")
	if err := os.WriteFile(rasterPath, []byte("PNGDATA"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := runEnrich(t, tier3PageStream(t, rasterPath), nil)

	var blocks, medias int
	for _, p := range got {
		switch p.Type {
		case model.PartBlock:
			blocks++
		case model.PartMedia:
			medias++
		}
	}
	if blocks == 0 {
		t.Error("tier-2 fallback should still emit blocks")
	}
	if medias != 0 {
		t.Errorf("raster should be consumed even on fallback, got %d", medias)
	}
	if _, err := os.Stat(rasterPath); !os.IsNotExist(err) {
		t.Error("raster temp file should be deleted on fallback too")
	}
}
