package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRebuildKeepsTheSourceLinkCloser covers a link whose closer the source
// spelled with whitespace or angle brackets the rebuild would drop. The
// whitespace takes part in how the brackets around the link pair:
// "[[][](]( 0))" is one link whose text is "[][](", and re-spelled as
// "[[][](](0))" the inner brackets read as the link instead. A closer that
// still reads as the same destination is kept as the source spelled it, with a
// line ending inside it collapsed to a space.
func TestRebuildKeepsTheSourceLinkCloser(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"space before the destination, nested brackets", "[[][](]( 0))"},
		{"space before the destination", "[a]( 0)"},
		{"angle brackets", "[a](<b>)"},
		{"line ending before the destination", "[a](\n0)"},
		{"image with a space before the source", "![a]( s)"},
		// The same with a title: the destination's spelling is kept and the
		// title the stream carries goes between the source's delimiters.
		{"space before the destination, title, nested brackets", "[[][](]( 0 '0')"},
		{"space before the destination, title", "[a]( 0 '0')"},
		{"angle brackets, title", "[a](<b> 't')"},
		{"line ending before the destination, title", "[a](\n0 't')"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			out := assertRebuildKeepsItsBlocks(t, tc.input)
			assert.Equal(t, readBlockTexts(t, tc.input), readBlockTexts(t, out), "the rebuild changed the blocks")
		})
	}
	assert.Equal(t, "[a]( 0)\n", assertRebuildKeepsItsBlocks(t, "[a]( 0)"))
	assert.Equal(t, "[a]( 0)\n", assertRebuildKeepsItsBlocks(t, "[a](\n0)"))
	assert.Equal(t, "[a](<b> 't')\n", assertRebuildKeepsItsBlocks(t, "[a](<b> 't')"))
	assert.Equal(t, "[a]( 0 't')\n", assertRebuildKeepsItsBlocks(t, "[a](\n0 't')"))
}
