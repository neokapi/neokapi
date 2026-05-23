package i18next_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// corpusFiles globs every real-world i18next bundle vendored under
// testdata/corpus/. Provenance for each file is recorded in
// testdata/corpus/SOURCES.md.
func corpusFiles(t *testing.T) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join("testdata", "corpus", "*.json"))
	require.NoError(t, err)
	require.NotEmpty(t, matches, "expected real-world i18next bundles under testdata/corpus/")
	return matches
}

// TestCorpusByteFaithfulRoundTrip is the PRIMARY corpus acceptance bar: every
// genuine i18next/react-i18next resource bundle vendored from upstream
// repositories must round-trip read→write byte-for-byte when nothing is
// translated. A failure here is a real reader/writer (JSON delegation) bug.
//
// The corpus exercises constructs the synthetic fixtures only approximate:
// nested namespaces, dotted flat keys, {{interpolation}}, $t() nesting, v4
// plural sibling keys, context keys, ICU MessageFormat single-brace bodies,
// and <Trans> component placeholders — all drawn from real shipping bundles.
func TestCorpusByteFaithfulRoundTrip(t *testing.T) {
	resolver := newResolver(t)
	for _, path := range corpusFiles(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			original, err := os.ReadFile(path)
			require.NoError(t, err)
			parts, _ := readParts(t, path, resolver)
			out := writeParts(t, parts, "", resolver)
			assert.Equal(t, string(original), string(out),
				"real-world i18next bundle must round-trip byte-for-byte: %s", path)
		})
	}
}

// TestCorpusReadsAsTranslatableBundle verifies that every corpus bundle reads
// into at least one translatable block and that no block leaks an i18next
// interpolation token ({{...}}) into its translatable text — the protected
// codes must be split out as inline placeholder runs, not left in the prose.
func TestCorpusReadsAsTranslatableBundle(t *testing.T) {
	resolver := newResolver(t)
	for _, path := range corpusFiles(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			parts, _ := readParts(t, path, resolver)
			blocks := collectBlocks(parts)
			require.NotEmpty(t, blocks, "corpus bundle should yield translatable blocks: %s", path)
			for _, b := range blocks {
				require.NotEmpty(t, b.Source, "block %s has no source segment", b.Name)
				assert.NotContains(t, b.Source[0].Text(), "{{",
					"i18next interpolation leaked into translatable text of %s", b.Name)
				assert.NotContains(t, b.Source[0].Text(), "$t(",
					"i18next $t() nesting leaked into translatable text of %s", b.Name)
			}
		})
	}
}
