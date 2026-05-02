//go:build parity

package roundtrip

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
)

// NativeEngine drives a neokapi format reader → PseudoTranslate tool
// → writer pipeline in-process. The format ID is the registered
// neokapi name (e.g. "plaintext", "html", "po") rather than the
// upstream Okapi class name.
type NativeEngine struct {
	// FormatID is the registry key for the reader/writer pair.
	FormatID registry.FormatID

	// ReaderConfig is applied via reader.Config().ApplyMap when
	// non-nil. Pass the same semantic config the spec.yaml example
	// uses; defaults are taken otherwise.
	ReaderConfig map[string]any
}

// Name returns "native".
func (e *NativeEngine) Name() string { return "native" }

// Available always succeeds — the native pipeline runs in-process
// with no external dependencies.
func (e *NativeEngine) Available() error { return nil }

// RoundTrip extracts via the registered reader, applies the
// PseudoTranslate tool to every Block part, and writes through the
// registered writer. Returns the merged output bytes.
func (e *NativeEngine) RoundTrip(t *testing.T, in Input, spec PseudoSpec) []byte {
	t.Helper()
	if e.FormatID == "" {
		t.Fatal("NativeEngine.RoundTrip: FormatID is required")
	}
	reg := registry.NewFormatRegistry()
	formats.RegisterAll(reg)

	reader, err := reg.NewReader(e.FormatID)
	if err != nil {
		t.Fatalf("NativeEngine: reader for %q: %v", e.FormatID, err)
	}
	if len(e.ReaderConfig) > 0 {
		cfg := reader.Config()
		if cfg == nil {
			t.Fatalf("NativeEngine: ReaderConfig set but reader %q has no Config()", e.FormatID)
		}
		if err := cfg.ApplyMap(e.ReaderConfig); err != nil {
			t.Fatalf("NativeEngine: ApplyMap: %v", err)
		}
	}
	writer, err := reg.NewWriter(e.FormatID)
	if err != nil {
		t.Fatalf("NativeEngine: writer for %q: %v", e.FormatID, err)
	}

	tgt := model.LocaleID(spec.TgtLocale())

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, in.Filename)
	if err := os.WriteFile(inputPath, in.Bytes, 0o644); err != nil {
		t.Fatalf("NativeEngine: write input: %v", err)
	}

	doc := &model.RawDocument{
		URI:          inputPath,
		SourceLocale: model.LocaleID(spec.SrcLocale()),
		Reader:       io.NopCloser(bytes.NewReader(in.Bytes)),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := reader.Open(ctx, doc); err != nil {
		t.Fatalf("NativeEngine: reader.Open: %v", err)
	}
	defer reader.Close()

	// Set skeleton from input bytes for writers that reuse it.
	if setter, ok := writer.(format.OriginalContentSetter); ok {
		setter.SetOriginalContent(in.Bytes)
	}
	if setter, ok := writer.(format.SourcePathSetter); ok {
		setter.SetSourcePath(inputPath)
	}
	writer.SetLocale(tgt)

	var outBuf bytes.Buffer
	if err := writer.SetOutputWriter(&outBuf); err != nil {
		t.Fatalf("NativeEngine: SetOutputWriter: %v", err)
	}

	// Wire reader → pseudo transform → writer. The transform is
	// inline (a tiny goroutine that mutates Block parts) instead of
	// the registered PseudoTranslate tool — that tool always
	// applies an accent map, which would diverge from bridge/tikal
	// outputs. We want a deterministic wrap that all three engines
	// produce identically.
	readerCh := reader.Read(ctx)
	writerIn := make(chan *model.Part, 16)

	var wg sync.WaitGroup
	var readErr, writeErr error

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(writerIn)
		for res := range readerCh {
			if res.Error != nil {
				readErr = res.Error
				return
			}
			if res.Part == nil {
				continue
			}
			if res.Part.Type == model.PartBlock {
				if b, ok := res.Part.Resource.(*model.Block); ok {
					applyPseudoToBlock(b, spec)
				}
			}
			select {
			case writerIn <- res.Part:
			case <-ctx.Done():
				readErr = ctx.Err()
				return
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		writeErr = writer.Write(ctx, writerIn)
	}()

	wg.Wait()
	if err := writer.Close(); err != nil && writeErr == nil {
		writeErr = err
	}

	if readErr != nil && !errors.Is(readErr, io.EOF) {
		t.Fatalf("NativeEngine: reader stream: %v", readErr)
	}
	if writeErr != nil {
		t.Fatalf("NativeEngine: writer: %v", writeErr)
	}

	out := outBuf.Bytes()
	if len(out) == 0 {
		t.Fatalf("NativeEngine: writer produced empty output")
	}
	return out
}

// Compile-time interface check.
var _ Engine = (*NativeEngine)(nil)
