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

	// WriterOverlay is a curated map applied to the writer's
	// WriterConfig().ApplyMap to align the native writer's output with
	// upstream Okapi's defaults for parity comparison. These are NOT
	// format defaults — they exist solely so the parity test can verify
	// "given the same semantic config, native produces the same bytes
	// as okapi". Document the intent inline at the call site so the
	// "why" survives.
	WriterOverlay map[string]any
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
	if len(e.WriterOverlay) > 0 {
		cfgable, ok := writer.(format.WriterConfigurable)
		if !ok {
			t.Fatalf("NativeEngine: WriterOverlay set but writer %q does not implement WriterConfigurable", e.FormatID)
		}
		cfg := cfgable.WriterConfig()
		if cfg == nil {
			t.Fatalf("NativeEngine: WriterOverlay set but writer %q returned nil WriterConfig", e.FormatID)
		}
		if err := cfg.ApplyMap(e.WriterOverlay); err != nil {
			t.Fatalf("NativeEngine: WriterConfig.ApplyMap: %v", err)
		}
	}

	tgt := model.LocaleID(spec.TgtLocale())

	tmpDir := t.TempDir()
	inputPath := filepath.Join(tmpDir, in.Filename)
	if err := os.WriteFile(inputPath, in.Bytes, 0o644); err != nil {
		t.Fatalf("NativeEngine: write input: %v", err)
	}
	for name, data := range in.Companions {
		if err := os.WriteFile(filepath.Join(tmpDir, name), data, 0o644); err != nil {
			t.Fatalf("NativeEngine: write companion %q: %v", name, err)
		}
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

	// Wire the skeleton store before reader.Open so the reader can
	// stream entries while it parses. Writers that consume it produce
	// byte-stable output by replaying skeleton text + filling block
	// refs with translated content, instead of falling back to a
	// best-effort no-skeleton path.
	store, err := format.NewSkeletonStore()
	if err != nil {
		t.Fatalf("NativeEngine: skeleton store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if emitter, ok := reader.(format.SkeletonStoreEmitter); ok {
		emitter.SetSkeletonStore(store)
	}
	if consumer, ok := writer.(format.SkeletonStoreConsumer); ok {
		consumer.SetSkeletonStore(store)
	}

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

	// Drain the reader fully into a part slice (applying the inline
	// pseudo transform on Block parts) before touching the writer.
	// Sequencing read-then-write lets us detect a stub
	// SkeletonStoreEmitter (one that registers but emits zero entries)
	// and unwire the writer's skeleton consumer before it runs — without
	// the unwiring, a writer that branches on skeletonStore != nil would
	// silently produce empty output. Streaming concurrency isn't worth
	// preserving for parity fixtures (small, in-process).
	//
	// The pseudo transform is inline instead of the registered
	// PseudoTranslate tool — that tool always applies an accent map,
	// which would diverge from bridge/tikal outputs. We want a
	// deterministic wrap that all three engines produce identically.
	var parts []*model.Part
	for res := range reader.Read(ctx) {
		if res.Error != nil {
			if !errors.Is(res.Error, io.EOF) {
				t.Fatalf("NativeEngine: reader stream: %v", res.Error)
			}
			continue
		}
		if res.Part == nil {
			continue
		}
		if res.Part.Type == model.PartBlock {
			if b, ok := res.Part.Resource.(*model.Block); ok {
				applyPseudoToBlock(b, spec)
			}
		}
		parts = append(parts, res.Part)
	}

	if consumer, ok := writer.(format.SkeletonStoreConsumer); ok {
		if store.EntriesWritten() == 0 {
			// Reader registered as a SkeletonStoreEmitter but never
			// actually emitted (stubbed). Unwire the writer so it
			// takes its no-skeleton path.
			consumer.SetSkeletonStore(nil)
		}
	}

	writerIn := make(chan *model.Part, len(parts)+1)
	for _, p := range parts {
		writerIn <- p
	}
	close(writerIn)

	var writeErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		writeErr = writer.Write(ctx, writerIn)
	}()
	wg.Wait()
	if err := writer.Close(); err != nil && writeErr == nil {
		writeErr = err
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
