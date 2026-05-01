//go:build parity

package formats

import (
	"path/filepath"
	"testing"

	parityspec "github.com/neokapi/neokapi/cli/parity/spec"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/rtf"
)

// TestParityRtfSpec drives the RTF spec.yaml through the bridge AND
// the native reader, validating both against the spec contract and
// against each other. The same spec file drives the always-on native
// test (core/formats/rtf/spec_test.go), so a single source of truth
// describes RTF behavior across implementations.
func TestParityRtfSpec(t *testing.T) {
	specPath := filepath.Join("..", "..", "..", "core", "formats", "rtf", "spec.yaml")
	s, err := parityspec.LoadSpec(specPath)
	if err != nil {
		t.Fatalf("load spec %s: %v", specPath, err)
	}
	r := &parityspec.ParityRunner{
		Spec:      s,
		NewReader: func(_ string) format.DataFormatReader { return rtf.NewReader() },
	}
	r.Run(t)
}
