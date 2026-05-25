package mdx

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// corpusFiles globs every real-world .mdx file vendored under testdata/corpus/.
// Provenance for each file is recorded in testdata/corpus/SOURCES.md (all are
// verbatim copies of .mdx files from this repository's documentation sites).
func corpusFiles(t *testing.T) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join("testdata", "corpus", "*.mdx"))
	require.NoError(t, err)
	require.NotEmpty(t, matches, "expected real-world .mdx files under testdata/corpus/")
	return matches
}

// TestCorpusByteFaithfulRoundTrip is the PRIMARY corpus acceptance bar: every
// genuine .mdx file (drawn from this repo's docs sites) must round-trip
// read→write byte-for-byte when nothing is translated. The self-verifying
// opaque fallback (see reader.go) guarantees this unconditionally, so a failure
// is a real scanner / markdown-delegation bug — investigate and fix, do not
// paper over.
//
// The corpus spans the breadth of MDX docs prose: YAML frontmatter, ESM
// imports, block-level JSX (self-closing components, components with children),
// GFM tables, fenced code blocks, headings, lists, and inline markup.
func TestCorpusByteFaithfulRoundTrip(t *testing.T) {
	for _, path := range corpusFiles(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			src, err := os.ReadFile(path)
			require.NoError(t, err)
			out := roundTrip(t, src)
			assert.True(t, bytes.Equal(out, src),
				"real-world MDX must round-trip byte-for-byte: %s (src=%d out=%d)", path, len(src), len(out))
		})
	}
}

// TestCorpusOpaqueConstructsNotTranslatable verifies that across the real
// corpus, no ESM/JSX/expression/table bytes leak into a translatable Block.
// Component names, attribute names, and import paths must never be presented as
// translatable prose.
func TestCorpusOpaqueConstructsNotTranslatable(t *testing.T) {
	for _, path := range corpusFiles(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			src, err := os.ReadFile(path)
			require.NoError(t, err)
			parts, _ := readParts(t, src)
			for _, p := range parts {
				if p.Type != model.PartBlock {
					continue
				}
				b := p.Resource.(*model.Block)
				txt := b.SourceText()
				assert.NotContains(t, txt, "import ",
					"ESM import leaked into a translatable block in %s: %q", path, txt)
				assert.NotContains(t, txt, "```",
					"code fence leaked into a translatable block in %s: %q", path, txt)
			}
		})
	}
}
