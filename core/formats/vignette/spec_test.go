package vignette_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/format/spectest"
	vignettefmt "github.com/neokapi/neokapi/core/formats/vignette"
)

// TestSpec drives every Feature × Example in spec.yaml through the
// native vignette reader. The native reader was rewritten under #501
// to parse Vignette CMS export/import XML (the upstream `vgnexport`
// format).
//
// A few examples retain `expected_fail:` tags because the bridge
// daemon still extracts 0 Blocks from these inputs — that gap is
// unrelated to the native rewrite (the bridge is the divergent side
// now). The native runner logs informational "now pass — remove the
// expected_fail tag" messages on those examples; the parity runner
// suppresses bridge failures via the same tag.
//
// Vignette is a single-variant format — variant is unused, NewReader
// returns the package's default reader for every example.
func TestSpec(t *testing.T) {
	s, err := spec.Load("spec.yaml")
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	r := &spectest.NativeRunner{
		Spec:      s,
		NewReader: func(_ string) format.DataFormatReader { return vignettefmt.NewReader() },
	}
	r.Run(t)
}
