package pdf_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/format/spectest"
	"github.com/neokapi/neokapi/core/formats/pdf"
)

// TestSpec drives every Feature × Example in spec.yaml through the
// native pdf reader. Several examples carry expected_fail tags
// because the native reader is a regex-based content-stream walker
// without font / ToUnicode-CMap handling, without escaped-paren
// support, and without a configuration surface — see the spec
// preamble's "Native vs bridge divergence" section and #510 (the
// native bug filed alongside this spec).
//
// PDF is a single-variant format — variant is unused, NewReader
// returns the package's default reader for every example.
func TestSpec(t *testing.T) {
	s, err := spec.Load("spec.yaml")
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	r := &spectest.NativeRunner{
		Spec:      s,
		NewReader: func(_ string) format.DataFormatReader { return pdf.NewReader() },
	}
	r.Run(t)
}
