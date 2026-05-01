//go:build parity

package formats

import (
	"path/filepath"
	"testing"

	parityspec "github.com/neokapi/neokapi/cli/parity/spec"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/archive"
)

// TestParityArchiveSpec drives the archive (ZIP container) spec.yaml
// through the okapi-bridge daemon AND the native reader, validating
// both against the spec contract and against each other.
//
// The native side is currently divergent (see #504 — line-by-line
// extraction instead of subfilter dispatch); every example tags
// `expected_fail:` so the parity gate downgrades the bridge↔native
// mismatch to a logged divergence rather than failing CI.
func TestParityArchiveSpec(t *testing.T) {
	specPath := filepath.Join("..", "..", "..", "core", "formats", "archive", "spec.yaml")
	s, err := parityspec.LoadSpec(specPath)
	if err != nil {
		t.Fatalf("load spec %s: %v", specPath, err)
	}
	r := &parityspec.ParityRunner{
		Spec:      s,
		NewReader: func(_ string) format.DataFormatReader { return archive.NewReader() },
	}
	r.Run(t)
}
