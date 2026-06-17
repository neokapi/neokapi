//go:build pdfium

package pdf

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/klippa-app/go-pdfium/responses"
	"github.com/neokapi/neokapi/core/model"
)

// readPdfium runs the PDFium-backed reader (wazero backend under -tags pdfium)
// over a testdata file and returns the emitted blocks.
func readPdfium(t *testing.T, path string) []*model.Block {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()

	r := NewPdfiumReader()
	doc := &model.RawDocument{URI: path, SourceLocale: model.LocaleEnglish, Encoding: "UTF-8", Reader: f}
	if err := r.Open(context.Background(), doc); err != nil {
		t.Fatalf("reader open: %v", err)
	}
	defer r.Close()

	var blocks []*model.Block
	for res := range r.Read(context.Background()) {
		if res.Error != nil {
			t.Fatalf("read: %v", res.Error)
		}
		if res.Part != nil && res.Part.Type == model.PartBlock {
			if b, ok := res.Part.Resource.(*model.Block); ok {
				blocks = append(blocks, b)
			}
		}
	}
	return blocks
}

// The PDFium reader extracts Latin text and attaches positioned geometry —
// the win over the hand-rolled reader's whole-page, geometry-less blocks.
func TestPdfium_LatinWithGeometry(t *testing.T) {
	blocks := readPdfium(t, "testdata/hello.pdf")
	if len(blocks) == 0 {
		t.Fatal("no blocks from hello.pdf")
	}
	var joined string
	var withGeom int
	for _, b := range blocks {
		joined += b.SourceText() + " "
		if g, ok := b.Geometry(); ok && g != nil && g.BBox.W > 0 {
			withGeom++
			if g.Page != 1 {
				t.Errorf("expected page 1, got %d", g.Page)
			}
			if g.Origin != "top-left" {
				t.Errorf("expected top-left origin, got %q", g.Origin)
			}
		}
	}
	if !strings.Contains(joined, "Hello World") {
		t.Errorf("expected 'Hello World', got %q", strings.TrimSpace(joined))
	}
	if withGeom == 0 {
		t.Error("expected at least one block with positioned geometry")
	}
}

// The decisive case: a Type0/Identity-H/CIDFontType0 PDF. The hand-rolled
// reader garbles this (it drops CID glyphs); PDFium extracts the CJK correctly.
func TestPdfium_CIDFontCJK(t *testing.T) {
	blocks := readPdfium(t, "testdata/cjk.pdf")
	if len(blocks) == 0 {
		t.Fatal("no blocks from cjk.pdf")
	}
	var joined string
	for _, b := range blocks {
		joined += b.SourceText()
	}
	for _, want := range []string{"こんにちは世界", "日本語", "テキスト", "Hello"} {
		if !strings.Contains(joined, want) {
			t.Errorf("CID/CJK extraction missing %q; got %q", want, joined)
		}
	}
	// Every non-empty CJK block should carry geometry.
	for _, b := range blocks {
		if _, ok := b.Geometry(); !ok {
			t.Errorf("block %q lacks geometry", b.SourceText())
		}
	}
}

// geometryFromRect flips PDFium's bottom-left coordinates to the top-left
// convention when the page height is known, and degrades to bottom-left
// otherwise.
func TestGeometryFromRect(t *testing.T) {
	// Upper edge y=708, lower y=698 on a 792-tall page → top-left y = 792-708 = 84.
	g := geometryFromRect(responses.CharPosition{Left: 100, Top: 708, Right: 160, Bottom: 698}, 1, 792)
	if g == nil {
		t.Fatal("nil geometry")
	}
	if g.Origin != "top-left" || g.Page != 1 {
		t.Fatalf("origin/page = %q/%d", g.Origin, g.Page)
	}
	if g.BBox.X != 100 || g.BBox.W != 60 || g.BBox.H != 10 {
		t.Fatalf("bbox = %+v", g.BBox)
	}
	if g.BBox.Y != 84 {
		t.Fatalf("top-left Y = %v, want 84", g.BBox.Y)
	}
	// No page height → bottom-left fallback (Y = lower edge).
	g2 := geometryFromRect(responses.CharPosition{Left: 0, Top: 50, Right: 100, Bottom: 0}, 2, 0)
	if g2.Origin != "bottom-left" || g2.BBox.Y != 0 || g2.BBox.H != 50 {
		t.Fatalf("fallback bbox = %+v origin=%q", g2.BBox, g2.Origin)
	}
}
