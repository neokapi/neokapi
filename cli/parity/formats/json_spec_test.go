//go:build parity

package formats

import (
	"path/filepath"
	"testing"

	parityspec "github.com/neokapi/neokapi/cli/parity/spec"
	"github.com/neokapi/neokapi/core/format"
	jsonfmt "github.com/neokapi/neokapi/core/formats/json"
)

// TestParityJSONSpec drives the JSON spec.yaml through bridge AND
// native readers, validating both against the spec contract and
// against each other.
func TestParityJSONSpec(t *testing.T) {
	specPath := filepath.Join("..", "..", "..", "core", "formats", "json", "spec.yaml")
	s, err := parityspec.LoadSpec(specPath)
	if err != nil {
		t.Fatalf("load spec %s: %v", specPath, err)
	}
	r := &parityspec.ParityRunner{
		Spec:      s,
		NewReader: func(_ string) format.DataFormatReader { return jsonfmt.NewReader() },
	}
	r.Run(t)
}
