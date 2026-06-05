package vtt_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/format/spectest"
	vttfmt "github.com/neokapi/neokapi/core/formats/vtt"
)

// TestSpec drives every Feature × Example in spec.yaml through the
// native VTT reader. Same shape as openxml/html/markdown — single
// import, single Run() call. Failures pinpoint the feature and example
// so the spec doubles as documentation and verification.
func TestSpec(t *testing.T) {
	s, err := spec.Load("spec.yaml")
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	r := &spectest.NativeRunner{
		Spec:      s,
		NewReader: func(_ string) format.DataFormatReader { return vttfmt.NewReader() },
	}
	r.Run(t)
}
