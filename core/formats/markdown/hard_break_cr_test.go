package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHardBreakAfterABareCarriageReturn is the #3196 reproducer. goldmark ends
// a line on "\n" alone, so a bare "\r" before the two spaces that spell a hard
// break is whitespace inside the line: the parser trims it off the text node
// with the spaces, and the walk back from the newline that recovers the
// break's spelling stopped at it. The "\r" belonged to neither the text nor
// the placeholder, and the skeleton path wrote the paragraph one byte short.
// The spelling now takes every space, tab and carriage return the parser
// trimmed, so the text keeps the bare "\n" it always carried and the bytes
// ride the placeholder.
func TestHardBreakAfterABareCarriageReturn(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		text  string
	}{
		{"bare CR before the spaces", "One line\r  \nnext line.\n", "One line\nnext line."},
		{"bare CR before three spaces", "One line\r   \nnext line.\n", "One line\nnext line."},
		{"space then a bare CR before the spaces", "One line \r  \nnext line.\n", "One line\nnext line."},
		{"two bare CRs before the spaces", "One line\r\r  \nnext line.\n", "One line\nnext line."},
		{"in a blockquote", "> One line\r  \n> next line.\n", "One line\nnext line."},
		{"in a list item", "- One line\r  \n  next line.\n", "One line\nnext line."},
		{"two such lines", "a\r  \nb\r  \nc\n", "a\nb\nc"},
		// The controls: the shapes #1652, #2431 and #2529 pinned.
		{"CRLF after the spaces", "One line  \r\nnext line.\r\n", "One line\nnext line."},
		{"backslash break after a bare CR", "One line\r\\\nnext line.\n", "One line\r\nnext line."},
		{"a bare CR between the spaces is no break", "One line \r \nnext line.\n", "One line \r \nnext line."},
		{"plain hard break", "One line  \nnext line.\n", "One line\nnext line."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			blocks := readBlocks(t, tc.input)
			require.Len(t, blocks, 1)
			assert.Equal(t, tc.text, blocks[0].SourceText(), "the block text changed")
			assertRebuildKeepsItsBlocks(t, tc.input)
		})
	}
}
