package image

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/vision"
)

// makePNG returns a w×h PNG with a solid fill.
func makePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{200, 200, 200, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func makeJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func readAll(t *testing.T, data []byte, uri string) []*model.Part {
	t.Helper()
	r := NewReader()
	doc := &model.RawDocument{URI: uri, Reader: io.NopCloser(bytes.NewReader(data))}
	if err := r.Open(context.Background(), doc); err != nil {
		t.Fatalf("Open: %v", err)
	}
	var parts []*model.Part
	for res := range r.Read(context.Background()) {
		if res.Error != nil {
			t.Fatalf("Read error: %v", res.Error)
		}
		parts = append(parts, res.Part)
	}
	return parts
}

func requireBalanced(t *testing.T, parts []*model.Part) {
	t.Helper()
	if len(parts) < 2 {
		t.Fatal("expected at least open/close layers")
	}
	if parts[0].Type != model.PartLayerStart || parts[len(parts)-1].Type != model.PartLayerEnd {
		t.Fatal("stream must open and close with a layer")
	}
	depth := 0
	for _, p := range parts {
		switch p.Type {
		case model.PartLayerStart:
			depth++
		case model.PartLayerEnd:
			depth--
		}
		if depth < 0 {
			t.Fatal("LayerEnd without matching LayerStart")
		}
	}
	if depth != 0 {
		t.Fatal("unbalanced layers")
	}
}

func findMedia(parts []*model.Part) *model.Media {
	for _, p := range parts {
		if p.Type == model.PartMedia {
			if m, ok := p.Resource.(*model.Media); ok {
				return m
			}
		}
	}
	return nil
}

func countBlocks(parts []*model.Part) int {
	n := 0
	for _, p := range parts {
		if p.Type == model.PartBlock {
			n++
		}
	}
	return n
}

// With no vision engine registered, the image still opens and emits its Media
// (dimensions, mime, bytes) — no text — and never errors.
func TestRead_NoEngine_MediaOnly(t *testing.T) {
	vision.ResetForTest()
	defer vision.ResetForTest()

	parts := readAll(t, makePNG(t, 64, 32), "pic.png")
	requireBalanced(t, parts)

	m := findMedia(parts)
	if m == nil {
		t.Fatal("expected a Media part")
	}
	if m.MimeType != "image/png" {
		t.Errorf("mime = %q, want image/png", m.MimeType)
	}
	// Media references the image by URI; bytes are never inlined into the stream.
	if len(m.Data) != 0 {
		t.Errorf("Media must not carry inline bytes, got %d", len(m.Data))
	}
	if m.URI == "" || m.Properties["width"] != "64" || m.Properties["height"] != "32" {
		t.Errorf("media uri/dims wrong: uri=%q props=%v", m.URI, m.Properties)
	}
	if n := countBlocks(parts); n != 0 {
		t.Errorf("no-engine read should have 0 text blocks, got %d", n)
	}
}

func TestRead_JPEG(t *testing.T) {
	vision.ResetForTest()
	defer vision.ResetForTest()

	parts := readAll(t, makeJPEG(t, 40, 20), "pic.jpg")
	m := findMedia(parts)
	if m == nil || m.MimeType != "image/jpeg" {
		t.Fatalf("expected jpeg media, got %+v", m)
	}
}

// ocrFake returns canned, positioned OCR lines so the reader produces text
// blocks and runs tier-2 structure.
type ocrFake struct{}

func (ocrFake) OCR(_ context.Context, imagePath string, _ vision.OCROptions) (*vision.OCRResult, error) {
	if imagePath == "" {
		return nil, nil
	}
	return &vision.OCRResult{
		Width: 200, Height: 100,
		Lines: []vision.OCRLine{
			{Text: "Heading", BBox: model.Rect{X: 0, Y: 0, W: 200, H: 24}, Confidence: 0.99},
			{Text: "First body line.", BBox: model.Rect{X: 0, Y: 40, W: 180, H: 10}, Confidence: 0.97},
		},
	}, nil
}
func (ocrFake) Close() error { return nil }

// With a vision engine registered, the reader emits OCR text blocks (in addition
// to the Media), with the recognized text and geometry.
func TestRead_WithEngine_OCRBlocks(t *testing.T) {
	vision.ResetForTest()
	defer vision.ResetForTest()
	vision.RegisterEngine("fake", func() (vision.Engine, error) { return ocrFake{}, nil })

	parts := readAll(t, makePNG(t, 200, 100), "scan.png")
	requireBalanced(t, parts)

	if findMedia(parts) == nil {
		t.Error("Media part should still be emitted alongside OCR")
	}
	var texts []string
	for _, p := range parts {
		if p.Type == model.PartBlock {
			texts = append(texts, p.Resource.(*model.Block).SourceText())
		}
	}
	if len(texts) != 2 || texts[0] != "Heading" || texts[1] != "First body line." {
		t.Fatalf("OCR blocks = %v, want [Heading, First body line.]", texts)
	}
}

// Garbage bytes are reported as a clean error, never a panic.
func TestRead_BadData(t *testing.T) {
	vision.ResetForTest()
	defer vision.ResetForTest()

	r := NewReader()
	doc := &model.RawDocument{URI: "x.png", Reader: io.NopCloser(bytes.NewReader([]byte("not an image")))}
	if err := r.Open(context.Background(), doc); err != nil {
		t.Fatalf("Open: %v", err)
	}
	var gotErr bool
	for res := range r.Read(context.Background()) {
		if res.Error != nil {
			gotErr = true
		}
	}
	if !gotErr {
		t.Error("expected a decode error for garbage bytes")
	}
}
