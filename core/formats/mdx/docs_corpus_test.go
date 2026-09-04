package mdx

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// docsRoot is the repo's own documentation tree, the corpus the dogfood
// recipe binds to this reader, relative to this package.
var docsRoot = filepath.Join("..", "..", "..", "web", "docs")

// TestDocsCorpusReconstructsExactly walks every page under web/docs through
// the reader and asserts that no span and no block takes the opaque fallback.
// The fallback keeps the bytes safe and costs the page its translatable
// content, and it fires for a markdown round-trip defect the MDX self-check
// merely exposes: eight pages went opaque for a code span wrapping a
// continuation line, an entity in link text, or a task-list marker (#1870).
// When a page regresses, the failure names it and the first byte that
// diverged.
func TestDocsCorpusReconstructsExactly(t *testing.T) {
	t.Parallel()
	_, err := os.Stat(docsRoot)
	require.NoError(t, err, "web/docs must sit three levels above this package")

	var pages []string
	require.NoError(t, filepath.WalkDir(docsRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && (strings.HasSuffix(path, ".md") || strings.HasSuffix(path, ".mdx")) {
			pages = append(pages, path)
		}
		return nil
	}))
	require.NotEmpty(t, pages, "no pages under web/docs")

	for _, page := range pages {
		rel, err := filepath.Rel(docsRoot, page)
		require.NoError(t, err)
		t.Run(rel, func(t *testing.T) {
			t.Parallel()
			src, err := os.ReadFile(page)
			require.NoError(t, err)

			blocks, diags := readAll(t, string(src))
			for _, d := range diags {
				switch d.Category {
				case "structure.markdown-span-opaque", "structure.markdown-block-opaque":
					assert.Failf(t, "opaque fallback", "%s line %d: %s", rel, d.Line, d.Message)
				}
			}
			assert.NotEmpty(t, blocks, "%s yields no translatable block", rel)
			assert.Equal(t, string(src), string(roundTrip(t, src)), "%s does not round-trip byte-for-byte", rel)
		})
	}
}
