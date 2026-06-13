package mo_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/format/spectest"
	mo "github.com/neokapi/neokapi/core/formats/mo"
)

// TestSpec drives the spec.yaml through the native MO reader. MO is a
// write-only runtime catalog (harvest cohort, native-only): the single
// class: invalid case asserts the reader rejects a read attempt cleanly.
func TestSpec(t *testing.T) {
	s, err := spec.Load("spec.yaml")
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	r := &spectest.NativeRunner{
		Spec:      s,
		NewReader: func(_ string) format.DataFormatReader { return mo.NewReader() },
	}
	r.Run(t)
}
