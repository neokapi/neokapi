package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAdmonitionTitleReferenceMakesNoLabelVisible covers a Docusaurus
// admonition opener whose title is "[0]:0", over a definition of "0". The
// title is text to the reader, so the reference the parser formed in it
// resolves to nothing the reader offers: it made the label visible through
// the title alone, and the rebuild, writing the title as text, lost the
// label's block.
func TestAdmonitionTitleReferenceMakesNoLabelVisible(t *testing.T) {
	t.Parallel()
	in := "::: [0]:0\n\n[0]:0"
	require.Equal(t, in, roundtripWithSkeleton(t, in), "skeleton path is not byte-exact")
	assert.Equal(t, []string{"[0]:0"}, readBlockTexts(t, in))
	assert.Equal(t, "\\[0]:0\n\n[0]: 0\n", assertRebuildKeepsItsBlocks(t, in))

	// The rebuild writes the title as a paragraph, where "[0]" would be a
	// reference, so the title's brackets are escaped.
	plain := "::: [0]\n\n[0]:0"
	require.Equal(t, plain, roundtripWithSkeleton(t, plain), "skeleton path is not byte-exact")
	assert.Equal(t, []string{"[0]"}, readBlockTexts(t, plain))
	assert.Equal(t, "\\[0]\n\n[0]: 0\n", assertRebuildKeepsItsBlocks(t, plain))
}
