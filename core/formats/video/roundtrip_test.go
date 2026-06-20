package video

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
)

// TestRoundTrip is the read→write fidelity check (format-maturity L1): with no
// ffmpeg/av engine the video reader degrades to emitting the file as an opaque
// Media part (by URI), and the passthrough writer copies those bytes, so a
// no-transform round-trip is byte-identical. This is the whole-video
// replace-asset sink (AD-030): a localized per-locale video supplied by the
// user/connector is written out as-is. (When ffmpeg is present the reader
// demuxes into audio/frame Layers — covered by the reader's demux tests.)
func TestRoundTrip(t *testing.T) {
	// Force ffmpeg/ffprobe unresolvable so the reader takes the Media path.
	t.Setenv("PATH", "")
	t.Setenv("KAPI_AV_DIR", "")

	src := []byte("\x00\x00\x00\x18ftypmp42 fake-video-bytes-payload")
	dir := t.TempDir()
	in := filepath.Join(dir, "in.mp4")
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
