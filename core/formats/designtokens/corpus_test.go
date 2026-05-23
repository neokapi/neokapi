package designtokens_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// corpusFiles globs every real-world DTCG design-tokens file vendored under
// testdata/corpus/. Provenance for each file is recorded in
// testdata/corpus/SOURCES.md.
func corpusFiles(t *testing.T) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join("testdata", "corpus", "*.json"))
	require.NoError(t, err)
	require.NotEmpty(t, matches, "expected real-world DTCG token files under testdata/corpus/")
	return matches
}

// TestCorpusByteFaithfulRoundTrip is the PRIMARY corpus acceptance bar: every
// genuine W3C DTCG design-tokens file vendored from upstream must round-trip
// read→write byte-for-byte when nothing is translated. A failure here is a real
// reader/writer (JSON delegation) bug.
//
// The corpus exercises the full breadth of DTCG value types — color, dimension,
// fontFamily, fontWeight, number, cubicBezier, duration, and the composite
// types typography, transition, border, strokeStyle (incl. dashArray), and
// shadow (incl. a multi-value array) — together with the $type cascade (group
// $type inherited by leaf tokens) and curly-brace aliases.
func TestCorpusByteFaithfulRoundTrip(t *testing.T) {
	for _, path := range corpusFiles(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			original, err := os.ReadFile(path)
			require.NoError(t, err)
			parts, _ := readParts(t, path)
			out := writeParts(t, parts, "")
			assert.Equal(t, string(original), string(out),
				"real-world DTCG token file must round-trip byte-for-byte: %s", path)
		})
	}
}

// TestCorpusExtractsOnlyDescriptions verifies that across the real corpus the
// reader extracts ONLY $description values as translatable blocks — never a
// token $value, $type, or alias reference. The style-dictionary demo file
// carries no $description, so it exercises the zero-translatable-surface path
// (which still round-trips faithfully).
func TestCorpusExtractsOnlyDescriptions(t *testing.T) {
	for _, path := range corpusFiles(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			parts, _ := readParts(t, path)
			for name := range collectBlocks(parts) {
				assert.Contains(t, name, "$description",
					"only $description keys may be translatable; %s was extracted from %s", name, path)
			}
		})
	}
}
