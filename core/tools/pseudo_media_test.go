package tools

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/neokapi/neokapi/core/model"
)

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetRGBA(x, y, color.RGBA{255, 255, 255, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// The pseudo-translate tool pseudo-localizes an image Media part: it replaces
// the bytes with a watermarked variant, marks it, and pseudo-translates alt-text.
func TestPseudoLocalizeMedia_Image(t *testing.T) {
	cfg := &PseudoConfig{TargetLocale: "qps"}
	cfg.Reset()
	cfg.TargetLocale = "qps"

	src := pngBytes(t, 80, 50)
	m := &model.Media{MimeType: "image/png", Data: src, Filename: "pic.png", AltText: "a cat"}
	part := &model.Part{Type: model.PartMedia, Resource: m}

	pseudoLocalizeMedia(part, cfg)

	if bytes.Equal(m.Data, src) {
		t.Error("image bytes unchanged — expected a pseudo-localized variant")
	}
	if m.Properties["pseudo"] != "true" {
		t.Errorf("pseudo property = %q, want true", m.Properties["pseudo"])
	}
	if _, _, err := image.Decode(bytes.NewReader(m.Data)); err != nil {
		t.Errorf("output is not a decodable image: %v", err)
	}
	if m.AltText == "a cat" || m.AltText == "" {
		t.Errorf("alt-text not pseudo-translated: %q", m.AltText)
	}
}

// Non-image media passes through untouched.
func TestPseudoLocalizeMedia_NonImage(t *testing.T) {
	cfg := &PseudoConfig{TargetLocale: "qps"}
	cfg.Reset()
	cfg.TargetLocale = "qps"

	orig := []byte("%PDF-1.7 ...")
	m := &model.Media{MimeType: "application/pdf", Data: orig, Filename: "doc.pdf"}
	part := &model.Part{Type: model.PartMedia, Resource: m}

	pseudoLocalizeMedia(part, cfg)
	if !bytes.Equal(m.Data, orig) {
		t.Error("non-image media should pass through unchanged")
	}
}
