package image

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/vision"
)

// readParts drains the reader into a slice for assertions.
func readParts(t *testing.T, in string, src []byte) []*model.Part {
	t.Helper()
	r := NewReader()
	doc := &model.RawDocument{URI: in, Reader: io.NopCloser(bytes.NewReader(src))}
	if err := r.Open(context.Background(), doc); err != nil {
		t.Fatalf("Open: %v", err)
	}
	var parts []*model.Part
	for res := range r.Read(context.Background()) {
		if res.Error != nil {
			t.Fatalf("read: %v", res.Error)
		}
		parts = append(parts, res.Part)
	}
	return parts
}

// TestReadAltSidecar: an "<image>.alt.txt" sidecar is attached to the Media and
// emitted as a translatable caption Block linked to the image.
func TestReadAltSidecar(t *testing.T) {
	vision.ResetForTest()
	defer vision.ResetForTest()

	dir := t.TempDir()
	in := filepath.Join(dir, "hero.png")
	src := makePNG(t, 32, 24)
	if err := os.WriteFile(in, src, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(in+".alt.txt", []byte("A red bicycle\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var media *model.Media
	var caption *model.Block
	for _, p := range readParts(t, in, src) {
		switch res := p.Resource.(type) {
		case *model.Media:
			media = res
		case *model.Block:
			if res.SemanticRole() == model.RoleCaption {
				caption = res
			}
		}
	}

	if media == nil {
		t.Fatal("no Media part emitted")
	}
	if media.AltText != "A red bicycle" {
		t.Errorf("Media.AltText = %q, want %q", media.AltText, "A red bicycle")
	}
	if caption == nil {
		t.Fatal("no caption Block emitted")
	}
	if !caption.Translatable {
		t.Error("caption block should be translatable")
	}
	if got := caption.SourceText(); got != "A red bicycle" {
		t.Errorf("caption source = %q, want %q", got, "A red bicycle")
	}
	rel, ok := caption.Relations()
	if !ok || len(rel.Relations) != 1 || rel.Relations[0].Type != model.RelCaptionOf || rel.Relations[0].Target != media.ID {
		t.Errorf("caption relation = %+v, want caption-of → %s", rel, media.ID)
	}
}

// TestNoAltSidecar: with no sidecar, no caption block is emitted and AltText is empty.
func TestNoAltSidecar(t *testing.T) {
	vision.ResetForTest()
	defer vision.ResetForTest()

	dir := t.TempDir()
	in := filepath.Join(dir, "plain.png")
	src := makePNG(t, 16, 16)
	if err := os.WriteFile(in, src, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, p := range readParts(t, in, src) {
		if b, ok := p.Resource.(*model.Block); ok && b.SemanticRole() == model.RoleCaption {
			t.Fatal("unexpected caption block without a sidecar")
		}
		if m, ok := p.Resource.(*model.Media); ok && m.AltText != "" {
			t.Errorf("AltText = %q, want empty", m.AltText)
		}
	}
}

// TestWriteAltSidecar_Localized: a translated caption Block is folded back into a
// per-locale "<output>.alt.txt" sidecar, and the image bytes are still written.
func TestWriteAltSidecar_Localized(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "hero.fr.png")
	src := makePNG(t, 32, 24)

	media := &model.Media{ID: "img1", MimeType: "image/png", Data: src}
	caption := model.NewBlock("alt1", "A red bicycle")
	caption.SetSemanticRole(model.RoleCaption, 0)
	caption.AddRelation(model.RelCaptionOf, "img1")
	caption.SetTargetText(model.LocaleID("fr"), "Un vélo rouge")

	w := NewWriter()
	if err := w.SetOutput(out); err != nil {
		t.Fatal(err)
	}
	w.SetLocale(model.LocaleID("fr"))

	parts := make(chan *model.Part, 4)
	parts <- &model.Part{Type: model.PartMedia, Resource: media}
	parts <- &model.Part{Type: model.PartBlock, Resource: caption}
	close(parts)
	if err := w.Write(context.Background(), parts); err != nil {
		t.Fatalf("Write: %v", err)
	}
	_ = w.Close()

	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if !bytes.Equal(got, src) {
		t.Errorf("image bytes not written verbatim (in=%d out=%d)", len(src), len(got))
	}
	sidecar, err := os.ReadFile(out + ".alt.txt")
	if err != nil {
		t.Fatalf("read alt sidecar: %v", err)
	}
	if string(sidecar) != "Un vélo rouge\n" {
		t.Errorf("alt sidecar = %q, want %q", string(sidecar), "Un vélo rouge\n")
	}
}

// TestWriteAltSidecar_SourceFallback: with no target for the writer's locale, the
// source alt text round-trips into the sidecar.
func TestWriteAltSidecar_SourceFallback(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "hero.png")
	src := makePNG(t, 16, 16)

	media := &model.Media{ID: "img1", MimeType: "image/png", Data: src}
	caption := model.NewBlock("alt1", "A red bicycle")
	caption.SetSemanticRole(model.RoleCaption, 0)

	w := NewWriter()
	if err := w.SetOutput(out); err != nil {
		t.Fatal(err)
	}
	parts := make(chan *model.Part, 4)
	parts <- &model.Part{Type: model.PartMedia, Resource: media}
	parts <- &model.Part{Type: model.PartBlock, Resource: caption}
	close(parts)
	if err := w.Write(context.Background(), parts); err != nil {
		t.Fatalf("Write: %v", err)
	}
	_ = w.Close()

	sidecar, err := os.ReadFile(out + ".alt.txt")
	if err != nil {
		t.Fatalf("read alt sidecar: %v", err)
	}
	if string(sidecar) != "A red bicycle\n" {
		t.Errorf("alt sidecar = %q, want source fallback", string(sidecar))
	}
}
