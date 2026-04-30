package json_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
	jsonfmt "github.com/neokapi/neokapi/core/formats/json"
)

// TestSpec drives every Feature × Example in spec.yaml through the
// native JSON reader. Same shape as openxml's TestSpec — single
// import, single Run() call.
func TestSpec(t *testing.T) {
	s, err := spec.Load("spec.yaml")
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	r := &spec.NativeRunner{
		Spec:      s,
		NewReader: func(_ string) format.DataFormatReader { return jsonfmt.NewReader() },
	}
	r.Run(t)
}
