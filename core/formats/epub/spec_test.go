package epub

import (
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/format/spectest"
)

// TestSpec drives every Feature × Example in spec.yaml through the
// native EPUB reader. Failures pinpoint the feature and example so
// the spec doubles as documentation and verification.
//
// The reader runs in its direct-XHTML fallback mode (no
// SubfilterResolver attached) — the spec assertions describe that
// path. Sub-filtered behaviour is exercised by the
// `TestSubfilter_*` tests in `subfilter_test.go`.
func TestSpec(t *testing.T) {
	s, err := spec.Load("spec.yaml")
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	r := &spectest.NativeRunner{
		Spec:      s,
		NewReader: func(_ string) format.DataFormatReader { return NewReader() },
	}
	r.Run(t)
}
