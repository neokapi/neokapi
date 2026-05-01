//go:build parity

package formats

import (
	"path/filepath"
	"testing"

	parityspec "github.com/neokapi/neokapi/cli/parity/spec"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/pdf"
)

// TestParityPdfSpec drives the pdf spec.yaml through bridge AND
// native readers, validating both against the spec contract and
// against each other. The same spec file drives the always-on native
// test (core/formats/pdf/spec_test.go) — one source of truth for
// the format's behavior across implementations.
//
// Several examples carry expected_fail tags because the native
// reader's regex-based content-stream walk diverges from the bridge's
// PDFBox backend on paragraph segmentation, escaped string-literal
// parens, ToUnicode CMap glyph decoding, and configuration surface —
// see the spec preamble and #510 (the native bug filed alongside
// this spec).
func TestParityPdfSpec(t *testing.T) {
	specPath := filepath.Join("..", "..", "..", "core", "formats", "pdf", "spec.yaml")
	s, err := parityspec.LoadSpec(specPath)
	if err != nil {
		t.Fatalf("load spec %s: %v", specPath, err)
	}
	r := &parityspec.ParityRunner{
		Spec:      s,
		NewReader: func(_ string) format.DataFormatReader { return pdf.NewReader() },
	}
	r.Run(t)
}
