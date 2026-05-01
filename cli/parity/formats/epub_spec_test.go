//go:build parity

package formats

import (
	"path/filepath"
	"testing"

	parityspec "github.com/neokapi/neokapi/cli/parity/spec"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/epub"
)

// TestParityEpubSpec drives the EPUB spec.yaml through the parity
// runner. Every example carries `expected_fail:` because the
// okapi-bridge does NOT ship the okf_epub filter — the bridge v2
// manifest has no `okf_epub` entry, so dispatch errors with
// "FilterClass: okf_epub not found". Native still satisfies the
// assertions; the runner records each example as expected_fail rather
// than green parity. Drop the `expected_fail:` blocks once the bridge
// adds EPUB coverage.
func TestParityEpubSpec(t *testing.T) {
	specPath := filepath.Join("..", "..", "..", "core", "formats", "epub", "spec.yaml")
	s, err := parityspec.LoadSpec(specPath)
	if err != nil {
		t.Fatalf("load spec %s: %v", specPath, err)
	}
	r := &parityspec.ParityRunner{
		Spec:      s,
		NewReader: func(_ string) format.DataFormatReader { return epub.NewReader() },
	}
	r.Run(t)
}
