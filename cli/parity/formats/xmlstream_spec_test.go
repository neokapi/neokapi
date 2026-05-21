//go:build parity

package formats

import (
	"path/filepath"
	"testing"

	parityspec "github.com/neokapi/neokapi/cli/parity/spec"
	"github.com/neokapi/neokapi/core/format"
	xmlfmt "github.com/neokapi/neokapi/core/formats/xml"
)

// TestParityXmlstreamSpec drives the xmlstream spec.yaml through the
// bridge AND the native reader, validating both against the spec
// contract and against each other (block-text equivalence). The same
// spec file lives at core/formats/xml/spec.yaml and powers the
// always-on native test (core/formats/xml/spec_test.go) — native
// package is `xml`, bridge filter id is `okf_xmlstream`. Function name
// uses the bridge id (xmlstream) per spec coverage convention.
func TestParityXmlstreamSpec(t *testing.T) {
	specPath := filepath.Join("..", "..", "..", "core", "formats", "xml", "spec.yaml")
	s, err := parityspec.LoadSpec(specPath)
	if err != nil {
		t.Fatalf("load spec %s: %v", specPath, err)
	}
	r := &parityspec.ParityRunner{
		Spec:         s,
		NewReader:    func(_ string) format.DataFormatReader { return xmlfmt.NewReader() },
		BridgeConfig: xmlstreamBridgeConfig,
	}
	r.Run(t)
}
