package applestrings_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/format/spectest"
	applestrings "github.com/neokapi/neokapi/core/formats/applestrings"
)

// TestSpec drives every Feature × Example in spec.yaml through the native
// Apple .strings/.stringsdict reader (and the writer for the roundtrip
// view). Harvest cohort: native-only, no parity bridge.
func TestSpec(t *testing.T) {
	s, err := spec.Load("spec.yaml")
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	r := &spectest.NativeRunner{
		Spec:      s,
		NewReader: func(_ string) format.DataFormatReader { return applestrings.NewReader() },
		NewWriter: func(_ string) format.DataFormatWriter { return applestrings.NewWriter() },
	}
	r.Run(t)
}
