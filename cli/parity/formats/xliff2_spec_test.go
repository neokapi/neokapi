//go:build parity

package formats

import (
	"path/filepath"
	"testing"

	parityspec "github.com/neokapi/neokapi/cli/parity/spec"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/xliff2"
)

// TestParityXliff2Spec drives the xliff2 spec.yaml through the bridge
// AND the native reader, validating both against the spec contract
// and against each other (block-text equivalence). The same spec
// powers the always-on native test in core/formats/xliff2/spec_test.go.
func TestParityXliff2Spec(t *testing.T) {
	specPath := filepath.Join("..", "..", "..", "core", "formats", "xliff2", "spec.yaml")
	s, err := parityspec.LoadSpec(specPath)
	if err != nil {
		t.Fatalf("load spec %s: %v", specPath, err)
	}
	r := &parityspec.ParityRunner{
		Spec:      s,
		NewReader: func(_ string) format.DataFormatReader { return xliff2.NewReader() },
	}
	r.Run(t)
}
