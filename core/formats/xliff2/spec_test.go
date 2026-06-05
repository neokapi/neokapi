package xliff2_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	"github.com/neokapi/neokapi/core/format/spectest"
	"github.com/neokapi/neokapi/core/formats/xliff2"
)

// TestSpec drives every Feature × Example in spec.yaml through the
// native XLIFF 2.x reader. Same shape as json/openxml's TestSpec — a
// single Load + Run() pair feeds the runner.
func TestSpec(t *testing.T) {
	s, err := spec.Load("spec.yaml")
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	r := &spectest.NativeRunner{
		Spec:      s,
		NewReader: func(_ string) format.DataFormatReader { return xliff2.NewReader() },
	}
	r.Run(t)
}
