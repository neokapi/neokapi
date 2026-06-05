package regex_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/format/spectest"
	"github.com/neokapi/neokapi/core/formats/regex"
)

// TestSpec drives every Feature × Example in spec.yaml through the
// native regex reader. Same shape as plaintext's TestSpec — single
// import, single Run() call. Failures pinpoint the feature and example
// so the spec doubles as documentation and verification.
//
// okf_regex is a single-variant filter; the variant tag in the spec
// is unused.
func TestSpec(t *testing.T) {
	s, err := spec.Load("spec.yaml")
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	r := &spectest.NativeRunner{
		Spec:      s,
		NewReader: func(_ string) format.DataFormatReader { return regex.NewReader() },
	}
	r.Run(t)
}
