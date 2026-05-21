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
// The okf_regex rules list rides through the bridge under the reserved
// `regexRulesJson` parameter (see regexBridgeConfig): the spec's
// neokapi-keyed `rules` list is serialised to a JSON array, and the
// okapi-bridge daemon rebuilds real net.sf.okapi.filters.regex.Rule
// objects and compileRules() before extraction. regexBridgeConfig also
// converges the escape discriminator and forces regexOptions=0 so the
// bridge matches native RE2 semantics. With this transport in place the
// rule-driven examples parity-pass head-to-head.
func TestParityRegexSpec(t *testing.T) {
	specPath := filepath.Join("..", "..", "..", "core", "formats", "regex", "spec.yaml")
	s, err := parityspec.LoadSpec(specPath)
	if err != nil {
		t.Fatalf("load spec %s: %v", specPath, err)
	}
	r := &parityspec.ParityRunner{
		Spec:         s,
		NewReader:    func(_ string) format.DataFormatReader { return regex.NewReader() },
		BridgeConfig: regexBridgeConfig,
	}
	r.Run(t)
}
