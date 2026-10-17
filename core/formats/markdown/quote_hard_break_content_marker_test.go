package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestQuoteKeepsAContentMarkerAfterAHardBreak covers a blockquote whose line
// after a hard break is a lazy one holding a ">" as content: the carriage
// return before it keeps it from being a marker. The rebuild folds a
// continuation line that is a marker alone, and that line went with it, so
// the quote whose first line was a vertical tab was left with nothing the
// reader keeps. The line's prefix says the ">" is content: the carriage
// return the reader recorded as the prefix is put back before it, and a
// content marker with no such prefix is escaped before the body is folded.
func TestQuoteKeepsAContentMarkerAfterAHardBreak(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"vertical tab line then a content marker", ">\v\\\n\r>"},
		{"text line then a content marker", ">a\\\n\r>"},
		// With the prefix recorded, the ">" after the opening break's line
		// ending is content, not a marker to drop with the line ending.
		{"opening break then a content marker after the prefix", ">\\\n>\r>"},
		{"opening break then a content marker and text", ">\\\n>\r>a"},
		// Controls.
		{"content marker after a soft break", ">\v\n\r>"},
		{"content marker outside a quote", "\v\\\n\r>"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			out := assertRebuildKeepsItsBlocks(t, tc.input)
			assert.Len(t, readBlockTexts(t, out), len(readBlockTexts(t, tc.input)), "the rebuild changed the block count")
		})
	}
	assert.Equal(t, ">a\n\r>\n", assertRebuildKeepsItsBlocks(t, ">a\\\n\r>"))
}
