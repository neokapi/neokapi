//go:build parity

package formats

import (
	"path/filepath"
	"testing"

	parityspec "github.com/neokapi/neokapi/cli/parity/spec"
	"github.com/neokapi/neokapi/core/format"
	vttfmt "github.com/neokapi/neokapi/core/formats/vtt"
)

// TestParityVttSpec drives the VTT spec.yaml through bridge AND
// native readers, validating both against the spec contract and
// against each other. The same spec file drives the always-on native
// test (core/formats/vtt/spec_test.go) — one source of truth for the
// format's behavior across implementations.
func TestParityVttSpec(t *testing.T) {
	specPath := filepath.Join("..", "..", "..", "core", "formats", "vtt", "spec.yaml")
	s, err := parityspec.LoadSpec(specPath)
	if err != nil {
		t.Fatalf("load spec %s: %v", specPath, err)
	}
	r := &parityspec.ParityRunner{
		Spec:      s,
		NewReader: func(_ string) format.DataFormatReader { return vttfmt.NewReader() },
	}
	r.Run(t)
}
