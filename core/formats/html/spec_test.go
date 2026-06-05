package html_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/format/spectest"
	htmlfmt "github.com/neokapi/neokapi/core/formats/html"
)

// TestSpec drives every Feature × Example in spec.yaml through the
// native HTML reader. Same shape as openxml's TestSpec — single
// import, single Run() call. Failures pinpoint the feature and example
// so the spec doubles as documentation and verification.
func TestSpec(t *testing.T) {
	s, err := spec.Load("spec.yaml")
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	r := &spectest.NativeRunner{
		Spec:      s,
		NewReader: func(_ string) format.DataFormatReader { return htmlfmt.NewReader() },
	}
	r.Run(t)
}
