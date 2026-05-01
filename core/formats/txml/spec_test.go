package txml_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	txmlfmt "github.com/neokapi/neokapi/core/formats/txml"
)

// TestSpec drives every Feature × Example in spec.yaml through the
// native TXML reader. Many examples carry expected_fail tags because
// the native reader currently targets a synthetic <body>/<segment>
// shape rather than Wordfast Pro's real <translatable> schema (see
// issue #453, okf_txml row); the spec runner logs those as expected
// failures rather than turning them into test errors.
//
// TXML is a single-variant format — variant is unused, NewReader
// returns the package's default reader for every example.
func TestSpec(t *testing.T) {
	s, err := spec.Load("spec.yaml")
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	r := &spec.NativeRunner{
		Spec:      s,
		NewReader: func(_ string) format.DataFormatReader { return txmlfmt.NewReader() },
	}
	r.Run(t)
}
