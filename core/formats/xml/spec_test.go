package xml_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/format/spectest"
	"github.com/neokapi/neokapi/core/formats/xml"
)

// TestSpec drives every Feature × Example in spec.yaml through the
// native XML reader. The same spec.yaml drives the parity bridge
// runner under cli/parity/formats/xmlstream_spec_test.go so a single
// authored description covers both implementations.
func TestSpec(t *testing.T) {
	s, err := spec.Load("spec.yaml")
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	r := &spectest.NativeRunner{
		Spec:      s,
		NewReader: func(_ string) format.DataFormatReader { return xml.NewReader() },
	}
	r.Run(t)
}
