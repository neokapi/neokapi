package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRebuildWritesNoBlockForATextlessParagraph covers a paragraph whose only
// content is markup around inline HTML. The HTML has no spelling on the
// rebuild path and is dropped, and "*<A>*" before a heading was written as
// "**", a paragraph of two asterisks the pass after read as text. A paragraph
// with no text renders nothing.
func TestRebuildWritesNoBlockForATextlessParagraph(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"empty emphasis before a heading", "*<A>*\n# 0"},
		{"empty emphasis after a paragraph", "a\n\n*<A>*\n# 0"},
		{"empty strong between paragraphs", "a\n\n**<A>**\n\nb"},
		{"empty emphasis in a heading", "0\n# *<A>*"},
		{"empty emphasis in a list item", "0\n* *<A>*"},
		// Control: the same emphasis with text after it keeps its block.
		{"empty emphasis then text", "*<A>*\n0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			assertRebuildKeepsItsBlocks(t, tc.input)
		})
	}
	assert.Equal(t, "# 0\n", assertRebuildKeepsItsBlocks(t, "*<A>*\n# 0"))
	assert.Equal(t, "0\n", assertRebuildKeepsItsBlocks(t, "0\n# *<A>*"))
	assert.Equal(t, "**\n0\n", assertRebuildKeepsItsBlocks(t, "*<A>*\n0"))
}
