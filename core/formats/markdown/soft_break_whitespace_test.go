package markdown_test

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/formats/markdown"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSoftBreakWhitespaceRoundTrips is the #2431 reproducer. A soft line
// break whose line ends in a tab or a carriage return was written back as one
// space, because the parser stops its text before those bytes and the
// continuation was only taken from a newline. The bytes now ride the soft
// break's continuation text, so the line structure survives byte-for-byte.
func TestSoftBreakWhitespaceRoundTrips(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		text  string // source text of the first block
	}{
		{"tab before the newline", "a\t\nb\n", "a\t\nb"},
		{"spaces then a tab", "a   \t\nb\n", "a   \t\nb"},
		{"crlf", "a\r\nb\r\n", "a\r\nb"},
		{"crlf with a trailing space", "a \r\nb\r\n", "a \r\nb"},
		{"crlf three lines", "a\r\nb\r\nc\r\n", "a\r\nb\r\nc"},
		{"crlf in a blockquote", "> a\r\n> b\r\n", "a\r\n> b"},
		{"crlf in a list item", "- a\r\n  b\r\n", "a\r\n  b"},
		{"tab in a list item", "- a\t\n  b\n", "a\t\n  b"},
		{"crlf inside emphasis", "*a\r\nb*\r\n", "a\r\nb"},
		{"crlf after a link", "[l](u)\r\nb\r\n", "l\r\nb"},
		{"crlf inside a code span", "`a\r\nb`\r\n", "a\r\nb"},
		{"crlf in a setext heading", "a\r\nb\r\n===\r\n", "a\r\nb"},
		{"crlf mixed with a hard break", "a  \r\nb\r\nc\r\n", "a\nb\r\nc"},
		// #2506: a carriage return that FOLLOWS the line ending is a line
		// ending of its own to the parser, which skipped it.
		{"bare cr opening the next line", "a\n\rb\n", "a\n\rb"},
		{"bare cr after a setext bar", "=\n\r.", "=\n\r."},
		// #2516: four spaces make the line indented code, which cannot
		// interrupt a paragraph, so its ">" is text the parser did not strip.
		{"indented lazy quote marker", "A line\n    > indented quote.\n", "A line\n    > indented quote."},
		{"indented lazy marker alone", "0\n    >", "0\n    >"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			blocks := assertSkeletonByteExact(t, tc.input)
			assert.Equal(t, tc.text, blocks[0].SourceText())
		})
	}
}

// TestSoftBreakSingleTrailingSpaceKeepsTheLine covers a line ending in a single
// space, which the writer used to drop for okapi parity (#2496). The block
// keeps the space, its rendered content reproduces the source (which is what
// the MDX reader's byte-exact check uses), and so does the file.
func TestSoftBreakSingleTrailingSpaceKeepsTheLine(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ input, text string }{
		{"a \nb\n", "a \nb"},
		{"> a \n> b\n", "a \n> b"},
	} {
		blocks := readBlocks(t, tc.input)
		require.Len(t, blocks, 1)
		assert.Equal(t, tc.text, blocks[0].SourceText(), "the block keeps the line structure")
		assert.Equal(t, tc.text, markdown.RenderBlockContent(blocks[0], blocks[0].Source), "the rendered content reproduces the source")
		assert.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
	}
}

// TestSoftBreakWhitespaceRebuildIsIdempotent drives the same spellings through
// the rebuild path: the bytes a soft break carries survive a rebuild, and a
// second rebuild is a fixed point.
func TestSoftBreakWhitespaceRebuildIsIdempotent(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"a\t\nb", "a\r\nb\r\n", "> a \r\n> b", "- a\t\n  b\n", "a \nb"} {
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			out1, keys1, ok := tripMarkdown(ctx, []byte(in))
			require.True(t, ok, "first trip declined %q", in)
			out2, keys2, ok2 := tripMarkdown(ctx, out1)
			require.True(t, ok2, "re-reading %q failed", out1)
			assert.Equal(t, string(out1), string(out2), "rebuild is not idempotent")
			assert.Len(t, keys2, len(keys1), "block count changed on second pass")
		})
	}
}
