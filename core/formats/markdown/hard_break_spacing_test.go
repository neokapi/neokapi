package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHardBreakLineKeepsItsInteriorSpaces is the #2515 reproducer. goldmark
// splits a line ending in a hard break into several text nodes and flags the
// last of them, so the trim that drops the spaces spelling the break ran on a
// node whose trailing spaces are ordinary content: "a   b  \nc" reached the
// translator as "a b".
func TestHardBreakLineKeepsItsInteriorSpaces(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		text  string
	}{
		{"three interior spaces", "a   b  \nc", "a   b\nc"},
		{"two interior spaces", "a  b  \nc", "a  b\nc"},
		{"one interior space", "a b  \nc", "a b\nc"},
		{"two hard-break lines", "a  b  \nc  d  \ne", "a  b\nc  d\ne"},
		{"five trailing spaces", "0     \n0", "0\n0"},
		{"backslash break", "a  b\\\nc", "a  b\nc"},
		{"in a blockquote", "> a  b  \n> c", "a  b\nc"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			blocks := readBlocks(t, tc.input)
			require.NotEmpty(t, blocks)
			assert.Equal(t, tc.text, blocks[0].SourceText(), "the block's spacing changed")
		})
	}
}

// TestBackslashHardBreakKeepsTheLineIndent is the #2529 reproducer. The
// backslash form of a hard break (CommonMark 6.7) begins AT the backslash.
// Walking on past it over the whitespace in front took the line's own leading
// space into the spelling, and the skeleton wrote that space twice: once as the
// indentation it replays for the line, once inside the placeholder the break
// rides.
func TestBackslashHardBreakKeepsTheLineIndent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		text  string
	}{
		{"indented break on the first line", " \\\n0", "\n0"},
		{"indented break on a later line", "a\n \\\n0", "a\n \n0"},
		{"two spaces of indent", "  \\\n0", "\n0"},
		{"three spaces of indent", "   \\\n0", "\n0"},
		{"indented break in a blockquote", "> \\\n> 0", "\n0"},
		{"a space that is content", "a \\\nb", "a \nb"},
		{"no indent", "\\\n0", "\n0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			blocks := readBlocks(t, tc.input)
			require.NotEmpty(t, blocks)
			assert.Equal(t, tc.text, blocks[0].SourceText(), "the block text changed")
		})
	}
}
