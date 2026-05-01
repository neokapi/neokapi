//go:build parity

package formats

import (
	"path/filepath"
	"testing"

	parityspec "github.com/neokapi/neokapi/cli/parity/spec"
	"github.com/neokapi/neokapi/core/format"
	phpcontentfmt "github.com/neokapi/neokapi/core/formats/phpcontent"
)

// TestParityPhpSpec drives the PHP content spec.yaml through bridge
// AND native readers, validating both against the spec contract and
// against each other. The same spec file drives the always-on native
// test (core/formats/phpcontent/spec_test.go) — one source of truth
// for the format's behavior across implementations.
//
// The native package is `phpcontent` but the bridge filter id is
// `okf_phpcontent`; the spec.yaml `format:` field carries the bridge
// id so the contract-audit dashboard joins cleanly. The function name
// uses the bridge filter id (php) per the spec-rollout convention.
func TestParityPhpSpec(t *testing.T) {
	specPath := filepath.Join("..", "..", "..", "core", "formats", "phpcontent", "spec.yaml")
	s, err := parityspec.LoadSpec(specPath)
	if err != nil {
		t.Fatalf("load spec %s: %v", specPath, err)
	}
	r := &parityspec.ParityRunner{
		Spec:      s,
		NewReader: func(_ string) format.DataFormatReader { return phpcontentfmt.NewReader() },
	}
	r.Run(t)
}
