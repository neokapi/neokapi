package vision

import (
	"context"
	"errors"
	"testing"

	"github.com/neokapi/neokapi/core/model"
)

type fakeEngine struct{ closed bool }

func (f *fakeEngine) OCR(context.Context, string, OCROptions) (*OCRResult, error) {
	return &OCRResult{Width: 100, Height: 50}, nil
}
func (f *fakeEngine) Close() error { f.closed = true; return nil }

func TestRegistry(t *testing.T) {
	ResetForTest()
	defer ResetForTest()

	if Available("") {
		t.Fatal("no engine should be available after reset")
	}
	if _, err := Open(""); !errors.Is(err, ErrNoEngine) {
		t.Fatalf("Open(\"\") = %v, want ErrNoEngine", err)
	}

	RegisterEngine("fake", func() (Engine, error) { return &fakeEngine{}, nil })
	if !Available("") {
		t.Error("first registered engine should become the default")
	}
	if !Available("fake") {
		t.Error("named engine should be available")
	}
	if Available("missing") {
		t.Error("unregistered name should not be available")
	}

	eng, err := Open("")
	if err != nil {
		t.Fatalf("Open(default) error: %v", err)
	}
	if err := eng.Close(); err != nil {
		t.Errorf("Close error: %v", err)
	}
	if _, err := Open("missing"); err == nil {
		t.Error("Open(missing) should error")
	}
}

func TestBlocksFromOCR(t *testing.T) {
	res := &OCRResult{
		Width: 200, Height: 100,
		Lines: []OCRLine{
			{Text: "Title", BBox: model.Rect{X: 0, Y: 0, W: 120, H: 20}, Confidence: 0.99},
			{Text: "", BBox: model.Rect{X: 0, Y: 30, W: 50, H: 10}}, // skipped
			{Text: "Body line", BBox: model.Rect{X: 0, Y: 30, W: 180, H: 12}, Confidence: 0.95},
		},
	}
	counter := 0
	blocks := BlocksFromOCR(res, 1, &counter)
	if len(blocks) != 2 {
		t.Fatalf("blocks = %d, want 2 (empty line skipped)", len(blocks))
	}
	if blocks[0].SourceText() != "Title" || blocks[1].SourceText() != "Body line" {
		t.Errorf("unexpected block text: %q, %q", blocks[0].SourceText(), blocks[1].SourceText())
	}
	g, ok := blocks[0].Geometry()
	if !ok || g.Origin != "top-left" || g.BBox.W != 120 || g.Page != 1 {
		t.Errorf("block geometry = %+v (ok=%v), want top-left page-1 W=120", g, ok)
	}
	if counter != 2 {
		t.Errorf("counter = %d, want 2", counter)
	}
}

func TestBlocksFromOCR_Nil(t *testing.T) {
	c := 0
	if b := BlocksFromOCR(nil, 1, &c); b != nil {
		t.Errorf("BlocksFromOCR(nil) = %v, want nil", b)
	}
}
