package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRebuildOfAQuoteOpeningWithAHardBreak is the #3227 reproducer. A
// blockquote's paragraph opens with a backslash hard break on a line of its
// own. The break spells nothing on the rebuild path, and the text after it
// carried the next line's "> " prefix, so the body began with the marker the
// block's own prefix supplies and the rebuild escaped it.
func TestRebuildOfAQuoteOpeningWithAHardBreak(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"one leading break", "> \\\n> * \n> 0"},
		{"two leading breaks", "> \\\n> \\\n> * \n> 0"},
		{"leading break then text", "> \\\n> a\n> b"},
		{"leading spaces break", ">   \n> * \n> 0"},
		{"leading break in a nested quote", ">> \\\n>> * \n>> 0"},
		// Controls: a break after content, and the paragraph form (#2448).
		{"break after content", "> a\\\n> * \n> 0"},
		{"paragraph opening with a break", "\\\n* \n0"},
		// Outside a quote the ">" after the break's line ending is content,
		// and so is one after a bare line ending the writer restores the
		// prefix for.
		{"paragraph opening with a break then a content marker", "\\\n\r>"},
		{"quote opening with a break then a lazy content marker", ">\\\n\r>"},
		{"quote opening with a break then a lazy content marker and text", ">\\\n\r>a"},
		{"quote opening with a break then a marked content marker", "> \\\n> \r>a"},
		{"paragraph opening with a break then a content marker and text", "\\\n\r>a"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			out := assertRebuildKeepsItsBlocks(t, tc.input)
			assert.Len(t, readBlockTexts(t, out), len(readBlockTexts(t, tc.input)), "the rebuild split the block")
		})
	}
}

// TestRebuildOfAQuoteOpeningWithAHardBreakSpelling pins the rebuild: the
// marker once, and the bullet that now opens the body escaped.
func TestRebuildOfAQuoteOpeningWithAHardBreakSpelling(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "> \\* \n> 0\n", assertRebuildKeepsItsBlocks(t, "> \\\n> * \n> 0"))
	assert.Equal(t, "> \\* \n> 0\n", assertRebuildKeepsItsBlocks(t, "> \\\n> \\\n> * \n> 0"))
}
