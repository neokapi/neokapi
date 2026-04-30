//go:build parity

package formats

import (
	"path/filepath"
	"testing"

	parityspec "github.com/neokapi/neokapi/cli/parity/spec"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/markdown"
)

// TestParityMarkdownSpec drives the markdown spec.yaml through bridge
// AND native readers, validating both against the spec contract and
// against each other.
func TestParityMarkdownSpec(t *testing.T) {
	specPath := filepath.Join("..", "..", "..", "core", "formats", "markdown", "spec.yaml")
	s, err := parityspec.LoadSpec(specPath)
	if err != nil {
		t.Fatalf("load spec %s: %v", specPath, err)
	}
	r := &parityspec.ParityRunner{
		Spec:      s,
		NewReader: func(_ string) format.DataFormatReader { return markdown.NewReader() },
	}
	r.Run(t)
}
