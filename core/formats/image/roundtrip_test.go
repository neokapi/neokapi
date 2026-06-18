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

// TestRoundTrip is the read→write fidelity check (format-maturity L1): reading an
// image and writing it back reproduces the original bytes. The image reader
// emits the picture as a Media part (by URI); the writer copies those bytes, so
// a no-transform round-trip is byte-identical. (A pseudo-localize or real
// localization transform mutates the Media between read and write — covered by
// the imageops + pseudo-translate tests.)
func TestRoundTrip(t *testing.T) {
	vision.ResetForTest()
	defer vision.ResetForTest()

	src := makePNG(t, 64, 48)
	dir := t.TempDir()
	in := filepath.Join(dir, "in.png")
	if err := os.WriteFile(in, src, 0o644); err != nil {
		t.Fatal(err)
	}

	r := NewReader()
	doc := &model.RawDocument{URI: in, Reader: io.NopCloser(bytes.NewReader(src))}
	if err := r.Open(context.Background(), doc); err != nil {
		t.Fatalf("Open: %v", err)
	}
	parts := make(chan *model.Part, 16)
	go func() {
		defer close(parts)
		for res := range r.Read(context.Background()) {
			if res.Error != nil {
				t.Errorf("read: %v", res.Error)
				return
			}
			parts <- res.Part
		}
	}()

	w := NewWriter()
	var buf bytes.Buffer
	if err := w.SetOutputWriter(&buf); err != nil {
		t.Fatal(err)
	}
	if err := w.Write(context.Background(), parts); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if !bytes.Equal(buf.Bytes(), src) {
		t.Errorf("round-trip not byte-identical: in=%d out=%d", len(src), buf.Len())
	}
}
