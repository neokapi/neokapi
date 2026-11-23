package tools

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

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

	pseudoLocalizeMedia(part, cfg, "")

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

	pseudoLocalizeMedia(part, cfg, "")
	if !bytes.Equal(m.Data, orig) {
		t.Error("non-image media should pass through unchanged")
	}
}

// A Media URI is content. With a project root, one that leads outside the
// project, by its spelling or through a symbolic link, is not read; one
// beneath the root is.
func TestPseudoLocalizeMedia_URIStaysInTheProject(t *testing.T) {
	cfg := &PseudoConfig{TargetLocale: "qps"}
	cfg.Reset()
	cfg.TargetLocale = "qps"

	project, outside := t.TempDir(), t.TempDir()
	secret := filepath.Join(outside, "secret.png")
	require.NoError(t, os.WriteFile(secret, pngBytes(t, 20, 20), 0o644))
	inside := filepath.Join(project, "pic.png")
	require.NoError(t, os.WriteFile(inside, pngBytes(t, 20, 20), 0o644))
	link := filepath.Join(project, "link.png")
	if err := os.Symlink(secret, link); err != nil {
		t.Skipf("symbolic links unavailable: %v", err)
	}

	for _, tc := range []struct {
		name, uri string
		read      bool
	}{
		{"outside", secret, false},
		{"climbs", filepath.Join(project, "..", filepath.Base(outside), "secret.png"), false},
		{"link out", link, false},
		{"inside", inside, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := &model.Media{MimeType: "image/png", URI: tc.uri}
			pseudoLocalizeMedia(&model.Part{Type: model.PartMedia, Resource: m}, cfg, project)
			require.Equal(t, tc.read, len(m.Data) > 0)
		})
	}
}
