package format

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/model"
)

// stubReader / stubWriter implement just enough of the format interfaces to
// exercise WireSkeleton's same-format gate and origin tagging.
type stubReader struct {
	BaseFormatReader
	wired *SkeletonStore
}

func (r *stubReader) Signature() FormatSignature                     { return FormatSignature{} }
func (r *stubReader) Open(context.Context, *model.RawDocument) error { return nil }
func (r *stubReader) Read(context.Context) <-chan model.PartResult   { return nil }
func (r *stubReader) Close() error                                   { return nil }
func (r *stubReader) Config() DataFormatConfig                       { return nil }
func (r *stubReader) SetConfig(DataFormatConfig) error               { return nil }
func (r *stubReader) DisplayName() string                            { return r.FormatName }
func (r *stubReader) SetSkeletonStore(s *SkeletonStore)              { r.wired = s }

type stubWriter struct {
	BaseFormatWriter
	wired *SkeletonStore
}

func (w *stubWriter) Write(context.Context, <-chan *model.Part) error { return nil }
func (w *stubWriter) SetSkeletonStore(s *SkeletonStore)               { w.wired = s }

func TestWireSkeleton_SameFormatWiresAndTags(t *testing.T) {
	store := NewMemorySkeletonStore()
	r := &stubReader{BaseFormatReader: BaseFormatReader{FormatName: "html"}}
	w := &stubWriter{BaseFormatWriter: BaseFormatWriter{FormatName: "html"}}

	if !WireSkeleton(store, r, w) {
		t.Fatal("same-format WireSkeleton should wire the writer")
	}
	if store.OriginFormat() != "html" {
		t.Errorf("origin = %q, want html", store.OriginFormat())
	}
	if r.wired != store || w.wired != store {
		t.Error("both reader and writer should be wired to the store")
	}
}

func TestWireSkeleton_CrossFormatDoesNotWireWriter(t *testing.T) {
	store := NewMemorySkeletonStore()
	r := &stubReader{BaseFormatReader: BaseFormatReader{FormatName: "openxml"}}
	w := &stubWriter{BaseFormatWriter: BaseFormatWriter{FormatName: "markdown"}}

	if WireSkeleton(store, r, w) {
		t.Fatal("cross-format WireSkeleton must NOT wire the foreign writer")
	}
	if store.OriginFormat() != "openxml" {
		t.Errorf("origin = %q, want openxml (tagged by the reader)", store.OriginFormat())
	}
	if w.wired != nil {
		t.Error("the markdown writer must not receive the openxml skeleton")
	}
}
