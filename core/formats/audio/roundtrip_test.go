package audio

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/asr"
	"github.com/neokapi/neokapi/core/model"
)

// TestRoundTrip is the read→write fidelity check (format-maturity L1): with no ASR
// engine the audio reader emits the track as an opaque Media part (by URI), and
// the passthrough writer copies those bytes, so a no-transform round-trip is
// byte-identical. This is the whole-audio replace-asset sink (AD-030): a
// localized per-locale audio file supplied by the user/connector is written
// out as-is. (When an ASR engine runs, the reader emits transcription Blocks
// instead — covered by the reader's ASR tests.)
func TestRoundTrip(t *testing.T) {
	asr.ResetForTest()
	defer asr.ResetForTest()

	src := []byte("RIFF\x24\x00\x00\x00WAVEfmt fake-audio-bytes-payload")
	dir := t.TempDir()
	in := filepath.Join(dir, "in.wav")
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
