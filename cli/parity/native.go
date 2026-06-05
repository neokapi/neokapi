//go:build parity

package parity

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// NativeRequest configures a native (in-process) format reader run.
type NativeRequest struct {
	// NewReader builds a fresh reader instance. Each call to RunNative
	// invokes this once.
	NewReader func() format.DataFormatReader

	// InputBytes is the document content fed through the reader.
	InputBytes []byte

	// SourceLocale and TargetLocale default to "en" / "fr" when empty.
	SourceLocale string
	TargetLocale string

	// Encoding defaults to "UTF-8".
	Encoding string

	// MimeType is optional.
	MimeType string

	// URI is the document URI (used by some formats for resolution).
	URI string

	// Params overrides the reader's default config via
	// reader.Config().ApplyMap. The same map is stringified for the
	// bridge side so both implementations run under the same
	// configuration. Unknown keys make the test fail fast — the same
	// behaviour as ApplyMap in production.
	Params map[string]any
}

// RunNative drives a neokapi format reader in-process and returns its
// part stream. Failures are fatal. Use TryRunNative when failures
// should surface as errors instead.
func RunNative(t *testing.T, req NativeRequest) []*model.Part {
	t.Helper()
	parts, err := TryRunNative(t, req)
	if err != nil {
		t.Fatalf("RunNative: %v", err)
	}
	return parts
}

// TryRunNative is the non-fatal variant of RunNative.
func TryRunNative(t *testing.T, req NativeRequest) ([]*model.Part, error) {
	t.Helper()
	if req.NewReader == nil {
		return nil, errors.New("NewReader is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	reader := req.NewReader()
	if len(req.Params) > 0 {
		if cfg := reader.Config(); cfg != nil {
			if err := cfg.ApplyMap(req.Params); err != nil {
				return nil, fmt.Errorf("ApplyMap: %w", err)
			}
		} else {
			return nil, errors.New("params set but reader has no Config()")
		}
	}
	doc := &model.RawDocument{
		URI:          req.URI,
		SourceLocale: model.LocaleID(defaultStr(req.SourceLocale, "en")),
		TargetLocale: model.LocaleID(defaultStr(req.TargetLocale, "fr")),
		Encoding:     defaultStr(req.Encoding, "UTF-8"),
		MimeType:     req.MimeType,
		Reader:       io.NopCloser(bytes.NewReader(req.InputBytes)),
	}
	if err := reader.Open(ctx, doc); err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}

	var parts []*model.Part
	for pr := range reader.Read(ctx) {
		if pr.Error != nil {
			return nil, fmt.Errorf("read part: %w", pr.Error)
		}
		parts = append(parts, pr.Part)
	}
	if err := reader.Close(); err != nil {
		return nil, fmt.Errorf("close: %w", err)
	}
	return parts, nil
}

// NativeRoundTripRequest wires a reader → writer round-trip in-process.
type NativeRoundTripRequest struct {
	NewReader    func() format.DataFormatReader
	NewWriter    func() format.DataFormatWriter
	InputBytes   []byte
	SourceLocale string
	TargetLocale string
	Encoding     string
	MimeType     string
	URI          string
	// Params overrides the reader+writer default config via
	// ApplyMap on each side. See NativeRequest.Params.
	Params map[string]any
}

// NativeRoundTripResult captures the parts read and the bytes the writer
// produced.
type NativeRoundTripResult struct {
	Parts  []*model.Part
	Output []byte
}

// RunNativeRoundTrip reads the document with NewReader, then writes
// every part through NewWriter and returns both the part list and the
// rewritten bytes. Skeleton store wiring (where supported) preserves
// the original byte layout for roundtrip-stable formats.
func RunNativeRoundTrip(t *testing.T, req NativeRoundTripRequest) NativeRoundTripResult {
	t.Helper()
	if req.NewReader == nil || req.NewWriter == nil {
		t.Fatal("RunNativeRoundTrip: NewReader and NewWriter are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	reader := req.NewReader()
	writer := req.NewWriter()
	if len(req.Params) > 0 {
		applyNativeParams(t, "RunNativeRoundTrip", reader, req.Params)
		if cw, ok := writer.(interface {
			Config() format.DataFormatConfig
		}); ok {
			if cfg := cw.Config(); cfg != nil {
				if err := cfg.ApplyMap(req.Params); err != nil {
					t.Fatalf("RunNativeRoundTrip: writer ApplyMap: %v", err)
				}
			}
		}
	}

	store, err := format.NewSkeletonStore()
	if err != nil {
		t.Fatalf("RunNativeRoundTrip: skeleton store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if emitter, ok := reader.(format.SkeletonStoreEmitter); ok {
		emitter.SetSkeletonStore(store)
	}
	if consumer, ok := writer.(format.SkeletonStoreConsumer); ok {
		consumer.SetSkeletonStore(store)
	}
	if setter, ok := writer.(format.OriginalContentSetter); ok {
		setter.SetOriginalContent(req.InputBytes)
	}

	target := defaultStr(req.TargetLocale, "fr")
	writer.SetLocale(model.LocaleID(target))

	doc := &model.RawDocument{
		URI:          req.URI,
		SourceLocale: model.LocaleID(defaultStr(req.SourceLocale, "en")),
		TargetLocale: model.LocaleID(target),
		Encoding:     defaultStr(req.Encoding, "UTF-8"),
		MimeType:     req.MimeType,
		Reader:       io.NopCloser(bytes.NewReader(req.InputBytes)),
	}
	if err := reader.Open(ctx, doc); err != nil {
		t.Fatalf("RunNativeRoundTrip: open: %v", err)
	}

	var parts []*model.Part
	for pr := range reader.Read(ctx) {
		if pr.Error != nil {
			t.Fatalf("RunNativeRoundTrip: read part: %v", pr.Error)
		}
		parts = append(parts, pr.Part)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("RunNativeRoundTrip: close reader: %v", err)
	}

	var buf bytes.Buffer
	if err := writer.SetOutputWriter(&buf); err != nil {
		t.Fatalf("RunNativeRoundTrip: SetOutputWriter: %v", err)
	}

	ch := make(chan *model.Part, len(parts))
	for _, p := range parts {
		ch <- p
	}
	close(ch)
	if err := writer.Write(ctx, ch); err != nil {
		t.Fatalf("RunNativeRoundTrip: write: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("RunNativeRoundTrip: close writer: %v", err)
	}
	return NativeRoundTripResult{Parts: parts, Output: buf.Bytes()}
}

// applyNativeParams overlays params on a reader's existing config via
// ApplyMap. Readers without a Config() (very rare) fail loudly so the
// spec author notices rather than silently running with defaults.
func applyNativeParams(t *testing.T, where string, reader format.DataFormatReader, params map[string]any) {
	t.Helper()
	cfg := reader.Config()
	if cfg == nil {
		t.Fatalf("%s: Params set but reader has no Config()", where)
	}
	if err := cfg.ApplyMap(params); err != nil {
		t.Fatalf("%s: ApplyMap: %v", where, err)
	}
}
