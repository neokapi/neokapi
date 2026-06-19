package asciidoc_test

import (
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCorpusByteFaithfulRoundTrip reads every AsciiDoc fixture under testdata/
// and asserts an untouched read→write reproduces the original bytes exactly via
// both the skeleton path and the original-content path. A failure here is a real
// byte-fidelity bug, not a fixture artefact.
func TestCorpusByteFaithfulRoundTrip(t *testing.T) {
	t.Parallel()
	matches, err := filepath.Glob(filepath.Join("testdata", "*.adoc"))
	require.NoError(t, err)
	require.NotEmpty(t, matches, "no AsciiDoc fixtures under testdata/")

	for _, path := range matches {
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()
			input := mustRead(t, path)

			// Every fixture must produce at least one translatable block.
			assert.NotEmpty(t, readBlocks(t, input), "fixture has no translatable content")

			assert.Equal(t, input, skelRoundtrip(t, input, ""), "skeleton path byte-exact")
			assert.Equal(t, input, origRoundtrip(t, input, ""), "original-content path byte-exact")
		})
	}
}

// TestCorpusBlockTextsNonEmpty asserts every extracted block carries non-empty
// source text (no empty translatable units leak from the parser).
func TestCorpusBlockTextsNonEmpty(t *testing.T) {
	t.Parallel()
	matches, _ := filepath.Glob(filepath.Join("testdata", "*.adoc"))
	for _, path := range matches {
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()
			for _, txt := range testutil.BlockTexts(readBlocks(t, mustRead(t, path))) {
				assert.NotEmpty(t, txt, "extracted block has empty source text")
			}
		})
	}
}
