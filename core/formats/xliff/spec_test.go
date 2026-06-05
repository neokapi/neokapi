package xliff

import (
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/format/spectest"
)

// TestSpec drives every Feature × Example in spec.yaml through the
// native xliff reader. Failures pinpoint the feature and example so
// the spec doubles as documentation and verification.
//
// XLIFF 1.2 is a single-variant format — variant is unused, NewReader
// returns the package's default reader for every example.
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
