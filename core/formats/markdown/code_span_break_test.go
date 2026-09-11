package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCodeSpanClosingOnTheNextLine is the #2481 reproducer. CommonMark 6.1
// turns a code span's interior line ending into a space and then strips one
// leading and one trailing space, so the parser's resolved content is shorter
// than the source spelled. The reader took the fences from the bytes next to
// that content, so a closing fence on a line of its own lost the break and, in
// a container, the continuation prefix with it.
func TestCodeSpanClosingOnTheNextLine(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"closing fence on its own line", "See ` code\n` here.\n"},
		{"in a blockquote", "> See ` code\n> ` here.\n"},
		{"in a list item", "- See ` code\n  ` here.\n"},
		{"padded double fence", "See `` ` a\n `` here.\n"},
		{"trailing space before the break", "See `code \n` here.\n"},
		{"opening fence at the end of a line", "See `\ncode` here.\n"},
		{"content on both sides of the break", "See ` a\n b ` here.\n"},
		{"crlf break", "See ` code\r\n` here.\r\n"},
		{"plain code span", "See `code` here.\n"},
		{"literal backtick", "a `` ` `` b\n"},
		// #2525: the content already carries the line ending, so the
		// continuation prefix sits at the content's end with no line ending in
		// front of it to recognise it by. It belonged to neither the content nor
		// the fence, and the rebuilt second line opened with no quote marker.
		{"content carries the break, in a blockquote", "> `a\n> ` b"},
		{"content carries the break, no space after the marker", ">`\n>`"},
		{"content carries the break, in a list item", "- `a\n  ` b"},
		{"content carries the break, nested quote", ">> `a\n>> ` b"},
		{"content carries the break, at the top level", "`a\n` b"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
		})
	}
}
