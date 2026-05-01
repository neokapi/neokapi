//go:build parity

package formats

import (
	"path/filepath"
	"testing"

	parityspec "github.com/neokapi/neokapi/cli/parity/spec"
	"github.com/neokapi/neokapi/core/format"
	vignettefmt "github.com/neokapi/neokapi/core/formats/vignette"
)

// TestParityVignetteSpec drives the vignette spec.yaml through bridge
// AND native readers, validating both against the spec contract and
// against each other. The same spec file drives the always-on native
// test (core/formats/vignette/spec_test.go) — one source of truth for
// the format's behavior across implementations.
//
// Every example's expected_fail tag covers the native side: the
// native reader currently parses R Markdown / R Sweave rather than
// Vignette CMS XML (see #453 + #501), so the bridge side carries the
// contract and the native side is logged as a documented divergence
// until the reader is rewritten.
func TestParityVignetteSpec(t *testing.T) {
	specPath := filepath.Join("..", "..", "..", "core", "formats", "vignette", "spec.yaml")
	s, err := parityspec.LoadSpec(specPath)
	if err != nil {
		t.Fatalf("load spec %s: %v", specPath, err)
	}
	r := &parityspec.ParityRunner{
		Spec:      s,
		NewReader: func(_ string) format.DataFormatReader { return vignettefmt.NewReader() },
	}
	r.Run(t)
}
