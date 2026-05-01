package vignette_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	vignettefmt "github.com/neokapi/neokapi/core/formats/vignette"
)

// TestSpec drives every Feature × Example in spec.yaml through the
// native vignette reader. Every example carries an expected_fail tag
// because the native reader currently implements R Markdown / R Sweave
// (.Rmd / .Rnw) rather than the Vignette CMS export XML format that
// `okf_vignette` actually targets — see issue #453's `okf_vignette`
// row and #501 (the native bug filed alongside this spec). The runner
// logs those as expected failures rather than turning them into test
// errors.
//
// Vignette is a single-variant format — variant is unused, NewReader
// returns the package's default reader for every example.
func TestSpec(t *testing.T) {
	s, err := spec.Load("spec.yaml")
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	r := &spec.NativeRunner{
		Spec:      s,
		NewReader: func(_ string) format.DataFormatReader { return vignettefmt.NewReader() },
	}
	r.Run(t)
}
