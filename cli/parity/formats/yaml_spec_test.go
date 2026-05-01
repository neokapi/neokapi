//go:build parity

package formats

import (
	"path/filepath"
	"testing"

	parityspec "github.com/neokapi/neokapi/cli/parity/spec"
	"github.com/neokapi/neokapi/core/format"
	yamlfmt "github.com/neokapi/neokapi/core/formats/yaml"
)

// TestParityYamlSpec drives the YAML spec.yaml through bridge AND
// native readers, validating both against the spec contract and
// against each other. The same spec file drives the always-on native
// test (core/formats/yaml/spec_test.go) — one source of truth for
// the format's behavior across implementations.
func TestParityYamlSpec(t *testing.T) {
	specPath := filepath.Join("..", "..", "..", "core", "formats", "yaml", "spec.yaml")
	s, err := parityspec.LoadSpec(specPath)
	if err != nil {
		t.Fatalf("load spec %s: %v", specPath, err)
	}
	r := &parityspec.ParityRunner{
		Spec:      s,
		NewReader: func(_ string) format.DataFormatReader { return yamlfmt.NewReader() },
	}
	r.Run(t)
}
