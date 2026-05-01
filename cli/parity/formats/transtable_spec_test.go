//go:build parity

package formats

import (
	"path/filepath"
	"testing"

	parityspec "github.com/neokapi/neokapi/cli/parity/spec"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/transtable"
)

// TestParityTranstableSpec drives the TransTable spec.yaml through
// bridge AND native readers, validating both against the spec
// contract and against each other. The native reader implements a
// generic key/value table rather than the upstream TransTable v1
// format the spec describes, so every example is tagged
// `expected_fail` per the divergence documented in #453.
func TestParityTranstableSpec(t *testing.T) {
	specPath := filepath.Join("..", "..", "..", "core", "formats", "transtable", "spec.yaml")
	s, err := parityspec.LoadSpec(specPath)
	if err != nil {
		t.Fatalf("load spec %s: %v", specPath, err)
	}
	r := &parityspec.ParityRunner{
		Spec:      s,
		NewReader: func(_ string) format.DataFormatReader { return transtable.NewReader() },
	}
	r.Run(t)
}
