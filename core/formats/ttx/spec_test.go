package ttx_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/format/spectest"
	ttxfmt "github.com/neokapi/neokapi/core/formats/ttx"
)

// TestSpec drives every Feature × Example in spec.yaml through the
// native TTX reader. Same shape as tmx/json/openxml — single import,
// single Run() call.
func TestSpec(t *testing.T) {
	s, err := spec.Load("spec.yaml")
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	r := &spectest.NativeRunner{
		Spec:      s,
		NewReader: func(_ string) format.DataFormatReader { return ttxfmt.NewReader() },
	}
	r.Run(t)
}
