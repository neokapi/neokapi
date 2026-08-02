package markdown_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mustReadFixture reads a testdata fixture, failing the test if it cannot.
func mustReadFixture(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(b)
}

// TestCorpusByteFaithfulRoundTrip reads every Markdown fixture under testdata/
// and asserts that an untouched read → skeleton-write reproduces the original
// bytes exactly. The skeleton path is the byte-exact write path (Mode 1 in
// writer.go); a drift here is a real byte-fidelity bug, not a fixture artefact.
//
// The fixtures are own-created (Apache-2.0), covered 100% by corpus.yaml, and
// span the format's common constructs: headings, inline emphasis/code, links
// and images, ordered/unordered/nested lists, a fenced code block, a thematic
// break, a blockquote, a GFM table, and YAML front matter. This is the
// corpus/upstream test the Engine-L3 floor requires (format-maturity.md §2.1).
func TestCorpusByteFaithfulRoundTrip(t *testing.T) {
	t.Parallel()
	matches, err := filepath.Glob(filepath.Join("testdata", "*.md"))
	require.NoError(t, err)
	require.NotEmpty(t, matches, "no Markdown fixtures under testdata/")

	for _, path := range matches {
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()
			input := mustReadFixture(t, path)

			// Every fixture must produce at least one translatable block.
			assert.NotEmpty(t, readBlocks(t, input), "fixture has no translatable content")

			assert.Equal(t, input, roundtripWithSkeleton(t, input), "skeleton path byte-exact")
		})
	}
}

// TestCorpusBlockTextsNonEmpty asserts every extracted block carries non-empty
// source text — the reader must not leak empty translatable units from a real
// document.
func TestCorpusBlockTextsNonEmpty(t *testing.T) {
	t.Parallel()
	matches, err := filepath.Glob(filepath.Join("testdata", "*.md"))
	require.NoError(t, err)

	for _, path := range matches {
		t.Run(filepath.Base(path), func(t *testing.T) {
			t.Parallel()
			for _, txt := range testutil.BlockTexts(readBlocks(t, mustReadFixture(t, path))) {
				assert.NotEmpty(t, txt, "extracted block has empty source text")
			}
		})
	}
}
