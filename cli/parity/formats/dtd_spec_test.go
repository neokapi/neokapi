//go:build parity

package formats

import (
	"path/filepath"
	"testing"

	parityspec "github.com/neokapi/neokapi/cli/parity/spec"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/dtd"
)

// TestParityDtdSpec drives the DTD spec.yaml through the bridge AND the
// native reader, comparing both to the spec contract and to each other
// (block-text equivalence). The same spec file drives the always-on
// native test (core/formats/dtd/spec_test.go), so a single source of
// truth describes DTD's behavior across implementations.
func TestParityDtdSpec(t *testing.T) {
	specPath := filepath.Join("..", "..", "..", "core", "formats", "dtd", "spec.yaml")
	s, err := parityspec.LoadSpec(specPath)
	if err != nil {
		t.Fatalf("load spec %s: %v", specPath, err)
	}
	r := &parityspec.ParityRunner{
		Spec:      s,
		NewReader: func(_ string) format.DataFormatReader { return dtd.NewReader() },
	}
	r.Run(t)
}
