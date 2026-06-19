package asciidoc_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/format/spectest"
	"github.com/neokapi/neokapi/core/formats/asciidoc"
)

// TestSpec drives every Feature × Example in spec.yaml through the native
// AsciiDoc reader (and the writer for the roundtrip view). Harvest cohort:
// native-only, no parity bridge.
func TestSpec(t *testing.T) {
	s, err := spec.Load("spec.yaml")
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	r := &spectest.NativeRunner{
		Spec:      s,
		NewReader: func(_ string) format.DataFormatReader { return asciidoc.NewReader() },
		NewWriter: func(_ string) format.DataFormatWriter { return asciidoc.NewWriter() },
	}
	r.Run(t)
}
