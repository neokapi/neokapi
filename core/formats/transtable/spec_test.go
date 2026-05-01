package transtable

import (
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/format/spec"
)

// TestSpec drives every Feature × Example in spec.yaml through the
// native TransTable reader. The native reader implements a generic
// key/value table that diverges from the upstream Okapi TransTable v1
// contract the spec describes, so every example carries an
// `expected_fail` tag — see #453 and the file-level note in
// spec.yaml. This test still serves to surface the divergences as
// per-example dashboard rows and to flag the day a fixed example
// unexpectedly starts passing (NativeRunner logs a warning so the tag
// can be removed).
//
// The TransTable filter is single-variant; the variant arg is unused.
func TestSpec(t *testing.T) {
	s, err := spec.Load("spec.yaml")
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	r := &spec.NativeRunner{
		Spec:      s,
		NewReader: func(_ string) format.DataFormatReader { return NewReader() },
	}
	r.Run(t)
}
