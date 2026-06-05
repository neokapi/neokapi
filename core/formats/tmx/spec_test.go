package tmx_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/format/spectest"
	tmxfmt "github.com/neokapi/neokapi/core/formats/tmx"
)

// TestSpec drives every Feature × Example in spec.yaml through the
// native TMX reader. Same shape as json/openxml — single import,
// single Run() call.
func TestSpec(t *testing.T) {
	s, err := spec.Load("spec.yaml")
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	r := &spectest.NativeRunner{
		Spec:      s,
		NewReader: func(_ string) format.DataFormatReader { return tmxfmt.NewReader() },
	}
	r.Run(t)
}
