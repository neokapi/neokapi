package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestQuoteMarkedLineEscapesATabIndentedMarker covers a quote whose marked
// second line is "> ", a space, a tab and "#". The parser measures the
// indentation before a marker from the column after the quote marker: two
// columns there, and the "#" opens a heading. The escape measured it from the
// line's start, where a space and a tab are four columns and the "#" is text,
// so the "#" went unescaped and the rebuilt quote held a heading and a
// paragraph of a vertical tab, which is no block.
func TestQuoteMarkedLineEscapesATabIndentedMarker(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		">\v\n> \t#": ">\v\n> \t\\#\n",
		">a\n> \t#":  ">a\n> \t\\#\n",
		// With one marker of two the line is lazy for the inner quote, and
		// the "#" is measured from after the marker it has.
		">>\v\n> \t#": ">>\v\n> \t\\#\n",
		// At a line's start the same bytes are four columns of indent, and
		// the "#" is text.
		"a\n \t#": "a\n \t#\n",
	} {
		require.Equal(t, in, roundtripWithSkeleton(t, in), "skeleton path is not byte-exact")
		assert.Equal(t, want, assertRebuildKeepsItsBlocks(t, in), "input %q", in)
	}
}
