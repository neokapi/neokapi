package txml_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/format/spectest"
	txmlfmt "github.com/neokapi/neokapi/core/formats/txml"
)

// TestSpec drives every Feature × Example in spec.yaml through the
// native TXML reader. The reader walks Wordfast Pro's real
// <translatable> schema, so every contract in spec.yaml is enforced
// (no expected_fail tags).
//
// TXML is a single-variant format — variant is unused, NewReader
// returns the package's default reader for every example.
func TestSpec(t *testing.T) {
	s, err := spec.Load("spec.yaml")
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	r := &spectest.NativeRunner{
		Spec:      s,
		NewReader: func(_ string) format.DataFormatReader { return txmlfmt.NewReader() },
	}
	r.Run(t)
}
