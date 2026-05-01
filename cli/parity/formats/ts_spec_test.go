//go:build parity

package formats

import (
	"path/filepath"
	"testing"

	parityspec "github.com/neokapi/neokapi/cli/parity/spec"
	"github.com/neokapi/neokapi/core/format"
	tsfmt "github.com/neokapi/neokapi/core/formats/ts"
)

// TestParityTsSpec drives the ts spec.yaml through the bridge AND the
// native reader, comparing both to the spec contract and to each other.
// The same spec file drives the always-on native test
// (core/formats/ts/spec_test.go), so a single source of truth describes
// Qt Linguist TS's behavior across implementations.
func TestParityTsSpec(t *testing.T) {
	specPath := filepath.Join("..", "..", "..", "core", "formats", "ts", "spec.yaml")
	s, err := parityspec.LoadSpec(specPath)
	if err != nil {
		t.Fatalf("load spec %s: %v", specPath, err)
	}
	r := &parityspec.ParityRunner{
		Spec:      s,
		NewReader: func(_ string) format.DataFormatReader { return tsfmt.NewReader() },
	}
	r.Run(t)
}
