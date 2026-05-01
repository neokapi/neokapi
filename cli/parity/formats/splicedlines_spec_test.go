//go:build parity

package formats

import (
	"path/filepath"
	"testing"

	parityspec "github.com/neokapi/neokapi/cli/parity/spec"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/splicedlines"
)

// TestParitySplicedlinesSpec drives the spliced-lines spec.yaml
// through the bridge AND the native reader, validating both against
// the spec contract and against each other. The same spec file drives
// the always-on native test (core/formats/splicedlines/spec_test.go) —
// one source of truth for the format's behavior across implementations.
func TestParitySplicedlinesSpec(t *testing.T) {
	specPath := filepath.Join("..", "..", "..", "core", "formats", "splicedlines", "spec.yaml")
	s, err := parityspec.LoadSpec(specPath)
	if err != nil {
		t.Fatalf("load spec %s: %v", specPath, err)
	}
	r := &parityspec.ParityRunner{
		Spec:      s,
		NewReader: func(_ string) format.DataFormatReader { return splicedlines.NewReader() },
	}
	r.Run(t)
}
