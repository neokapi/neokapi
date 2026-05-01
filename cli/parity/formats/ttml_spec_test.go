//go:build parity

package formats

import (
	"path/filepath"
	"testing"

	parityspec "github.com/neokapi/neokapi/cli/parity/spec"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/ttml"
)

// TestParityTtmlSpec drives the TTML spec.yaml through the bridge AND
// the native reader, validating both against the spec contract and
// against each other. The same spec file drives the always-on native
// test (core/formats/ttml/spec_test.go), so a single source of truth
// describes TTML behavior across implementations.
//
// The spec uses neokapi-canonical config keys (mergeAdjacentCaptions,
// escapeBR); ttmlBridgeConfig translates them to the upstream Okapi
// names (mergeCaptions, escapeBrMode) before the bridge dispatch.
// Native config receives the keys verbatim.
func TestParityTtmlSpec(t *testing.T) {
	specPath := filepath.Join("..", "..", "..", "core", "formats", "ttml", "spec.yaml")
	s, err := parityspec.LoadSpec(specPath)
	if err != nil {
		t.Fatalf("load spec %s: %v", specPath, err)
	}
	r := &parityspec.ParityRunner{
		Spec:         s,
		NewReader:    func(_ string) format.DataFormatReader { return ttml.NewReader() },
		BridgeConfig: ttmlBridgeConfig,
	}
	r.Run(t)
}
