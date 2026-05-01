//go:build parity

package formats

import (
	"path/filepath"
	"testing"

	parityspec "github.com/neokapi/neokapi/cli/parity/spec"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/mif"
)

// TestParityMifSpec drives the MIF spec.yaml through bridge AND native
// readers, validating both against the spec contract and against each
// other. The same spec file drives the always-on native test
// (core/formats/mif/spec_test.go) — one source of truth for the
// format's behavior across implementations.
func TestParityMifSpec(t *testing.T) {
	specPath := filepath.Join("..", "..", "..", "core", "formats", "mif", "spec.yaml")
	s, err := parityspec.LoadSpec(specPath)
	if err != nil {
		t.Fatalf("load spec %s: %v", specPath, err)
	}
	r := &parityspec.ParityRunner{
		Spec:      s,
		NewReader: func(_ string) format.DataFormatReader { return mif.NewReader() },
	}
	r.Run(t)
}
