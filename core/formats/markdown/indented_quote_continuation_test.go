package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestIndentedQuoteMarkerAfterAHardBreak is the #3214 reproducer. A paragraph
// line ends in a hard break, the next line is indented by a tab or four spaces
// and begins with ">", and a line follows. CommonMark 4.8 keeps the indented
// line as paragraph text, so the ">" is content; the hard break's continuation
// prefix ran on past it and the block carried the marker twice.
func TestIndentedQuoteMarkerAfterAHardBreak(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"tab indent, spaces break", "One line  \n\t>quoted\nnext line.\n"},
		{"tab indent, backslash break", "One line\\\n\t>quoted\nnext line.\n"},
		{"four spaces indent", "One line  \n    >quoted\nnext line.\n"},
		{"marker with a space", "One line  \n\t> quoted\nnext line.\n"},
		// With no line after it the break is the paragraph's only one, and
		// the writer restores the tab as the continuation prefix; the ">" after
		// it is content and must not re-mark the paragraph as a quote.
		{"no line after", "One line  \n\t>quoted\n"},
		{"no line after, four spaces", "One line  \n    >quoted\n"},
		// Controls: a soft break in place of the hard one, and an indent of
		// three spaces, where the ">" opens a quote.
		{"soft break", "One line\n\t>quoted\nnext line.\n"},
		{"three spaces indent", "One line  \n   >quoted\nnext line.\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			assertRebuildKeepsItsBlocks(t, tc.input)
		})
	}
}

// TestIndentedQuoteMarkerAfterAHardBreakText pins the block's text and its
// rebuild: one ">" in the text, and the rebuild keeps the indentation that
// keeps the ">" text on its continuation line, whether the text carries it or
// the reader recorded it as the lines' prefix.
func TestIndentedQuoteMarkerAfterAHardBreakText(t *testing.T) {
	t.Parallel()
	assert.Equal(t, []string{"One line\n\t>quoted\nnext line."}, readBlockTexts(t, "One line  \n\t>quoted\nnext line.\n"))
	assert.Equal(t, "One line\n\t>quoted\nnext line.\n", assertRebuildKeepsItsBlocks(t, "One line  \n\t>quoted\nnext line.\n"))
	assert.Equal(t, "One line\n\t>quoted\n", assertRebuildKeepsItsBlocks(t, "One line  \n\t>quoted\n"))
}
