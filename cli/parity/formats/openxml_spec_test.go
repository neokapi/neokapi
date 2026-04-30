//go:build parity

package formats

import (
	"path/filepath"
	"testing"

	parityspec "github.com/neokapi/neokapi/cli/parity/spec"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/openxml"
)

// TestParityOpenXMLSpec drives the openxml spec.yaml through the
// bridge AND the native reader, comparing both to the spec contract
// and to each other (block-text equivalence). The same spec file
// drives the always-on native test (core/formats/openxml/spec_test.go),
// so a single source of truth describes openxml's behavior across
// implementations.
func TestParityOpenXMLSpec(t *testing.T) {
	specPath := filepath.Join("..", "..", "..", "core", "formats", "openxml", "spec.yaml")
	s, err := parityspec.LoadSpec(specPath)
	if err != nil {
		t.Fatalf("load spec %s: %v", specPath, err)
	}
	r := &parityspec.ParityRunner{
		Spec:      s,
		NewReader: func(_ string) format.DataFormatReader { return openxml.NewReader() },
	}
	r.Run(t)
}
