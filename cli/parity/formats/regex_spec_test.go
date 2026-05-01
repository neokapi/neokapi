//go:build parity

package formats

import (
	"path/filepath"
	"testing"

	parityspec "github.com/neokapi/neokapi/cli/parity/spec"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/regex"
)

// TestParityRegexSpec drives the regex spec.yaml through bridge AND
// native readers, validating both against the spec contract and
// against each other. The same spec file drives the always-on native
// test (core/formats/regex/spec_test.go) — one source of truth for
// the format's behavior across implementations.
//
// Most rule-driven examples carry expected_fail because the okf_regex
// rules list cannot be expressed through gRPC FilterParams
// (map<string,string>) — Okapi's StringParameters preset format
// requires a buffered group structure. Only the no_rules_default
// feature parity-passes; the others record the bridge transport gap
// while still asserting the native extraction contract.
func TestParityRegexSpec(t *testing.T) {
	specPath := filepath.Join("..", "..", "..", "core", "formats", "regex", "spec.yaml")
	s, err := parityspec.LoadSpec(specPath)
	if err != nil {
		t.Fatalf("load spec %s: %v", specPath, err)
	}
	r := &parityspec.ParityRunner{
		Spec:      s,
		NewReader: func(_ string) format.DataFormatReader { return regex.NewReader() },
	}
	r.Run(t)
}
