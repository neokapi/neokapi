package archive

import (
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
)

// TestSpec drives every Feature × Example in spec.yaml through the
// native archive reader. The archive filter is a meta-filter — Okapi
// dispatches entries through configured subfilters. The native reader
// implements a different contract (line-by-line text extraction when no
// SubfilterResolver is wired, see #504), so every example carries an
// expected_fail tag referencing that issue. The test still runs each
// example to exercise the spec parser, the fixture loader, and the
// expected_fail logging path.
func TestSpec(t *testing.T) {
	s, err := spec.Load("spec.yaml")
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	r := &spec.NativeRunner{
		Spec:      s,
		NewReader: func(_ string) format.DataFormatReader { return NewReader() },
	}
	r.Run(t)
}
