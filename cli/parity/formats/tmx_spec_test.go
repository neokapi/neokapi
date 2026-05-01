//go:build parity

package formats

import (
	"path/filepath"
	"testing"

	parityspec "github.com/neokapi/neokapi/cli/parity/spec"
	"github.com/neokapi/neokapi/core/format"
	tmxfmt "github.com/neokapi/neokapi/core/formats/tmx"
)

// TestParityTmxSpec drives the TMX spec.yaml through the bridge AND
// the native reader, comparing both to the spec contract and to each
// other. The same spec file drives the always-on native test
// (core/formats/tmx/spec_test.go), so a single source of truth
// describes TMX's behavior across implementations.
func TestParityTmxSpec(t *testing.T) {
	specPath := filepath.Join("..", "..", "..", "core", "formats", "tmx", "spec.yaml")
	s, err := parityspec.LoadSpec(specPath)
	if err != nil {
		t.Fatalf("load spec %s: %v", specPath, err)
	}
	r := &parityspec.ParityRunner{
		Spec:      s,
		NewReader: func(_ string) format.DataFormatReader { return tmxfmt.NewReader() },
	}
	r.Run(t)
}
