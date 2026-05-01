//go:build parity

package formats

import (
	"path/filepath"
	"testing"

	parityspec "github.com/neokapi/neokapi/cli/parity/spec"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/csv"
)

// TestParityCsvSpec drives the CSV spec.yaml through the bridge AND
// the native reader, validating both against the spec contract and
// against each other. The same spec file drives the always-on native
// test (core/formats/csv/spec_test.go), so a single source of truth
// describes CSV behavior across implementations.
func TestParityCsvSpec(t *testing.T) {
	specPath := filepath.Join("..", "..", "..", "core", "formats", "csv", "spec.yaml")
	s, err := parityspec.LoadSpec(specPath)
	if err != nil {
		t.Fatalf("load spec %s: %v", specPath, err)
	}
	r := &parityspec.ParityRunner{
		Spec:      s,
		NewReader: func(_ string) format.DataFormatReader { return csv.NewReader() },
	}
	r.Run(t)
}
