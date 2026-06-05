package odf

import (
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/format/spectest"
)

// TestSpec drives every Feature × Example in spec.yaml through the
// native odf reader. Failures pinpoint the feature and example so the
// spec doubles as documentation and verification.
//
// The reader auto-detects odt/ods/odp/odg from the input ZIP's
// mimetype entry, so the variant tag in the spec is metadata for
// filtering and display, not a reader knob.
func TestSpec(t *testing.T) {
	s, err := spec.Load("spec.yaml")
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	r := &spectest.NativeRunner{
		Spec:      s,
		NewReader: func(_ string) format.DataFormatReader { return NewReader() },
	}
	r.Run(t)
}
