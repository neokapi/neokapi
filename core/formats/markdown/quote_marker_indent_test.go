package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestQuoteWhoseFirstLineIsIndentedAfterItsMarkers covers a blockquote whose
// first line carries spaces of the paragraph's own indent after the ">"
// markers. The reader recorded the markers only when nothing else sat before
// the text, so the rebuild path had no marker for the block and guessed one
// level from a continuation line: ">>  0\n>0\n>>0" came back as ">0\n>0\n>>0",
// which reads as two blocks.
func TestQuoteWhoseFirstLineIsIndentedAfterItsMarkers(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"nested quote with lazy lines", ">>  0\n>0\n>>0"},
		{"one marker and two spaces", ">  0\n>0"},
		{"marker, space and tab", "> \t0\n> 0"},
		{"nested quote on one line", ">>  0"},
		{"three spaces of indent", ">   a\n> b"},
		// Control: one space, which the marker itself carries.
		{"one space", "> a\n> b"},
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

// TestQuoteWhoseFirstLineIsIndentedAfterItsMarkersSpelling pins the rebuild:
// the first line's prefix is restored as spelled.
func TestQuoteWhoseFirstLineIsIndentedAfterItsMarkersSpelling(t *testing.T) {
	t.Parallel()
	assert.Equal(t, ">>  0\n>0\n>>0\n", assertRebuildKeepsItsBlocks(t, ">>  0\n>0\n>>0"))
}

// TestQuoteMarkerKeepsOneSpaceOfItsIndent covers a paragraph inside a quoted
// list item, whose recorded marker carries the item's indentation: ">\t\t0"
// is the item's paragraph, and written with the marker as recorded, with the
// item structure gone, it was indented code inside the quote. The marker is
// written with at most one space after it.
func TestQuoteMarkerKeepsOneSpaceOfItsIndent(t *testing.T) {
	t.Parallel()
	for _, in := range []string{">*\t0\n>\n>\t\t0\n0", ">*\t0\n>\n>\t\t0", ">-  a\n>\n>   b"} {
		require.Equal(t, in, roundtripWithSkeleton(t, in), "skeleton path is not byte-exact")
		assertRebuildKeepsItsBlocks(t, in)
	}
	assert.Equal(t, "- 0\n\n> 0\n0\n", assertRebuildKeepsItsBlocks(t, ">*\t0\n>\n>\t\t0\n0"))
}
