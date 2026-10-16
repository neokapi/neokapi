package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCodeSpanClosingAfterABareCarriageReturn is the #3105 reproducer. A code
// span's content may carry the line ending, and the continuation prefix of the
// line that holds the closing fence then sits at the content's end with no
// line ending in front of it, so the fence carries it (#2525). The walk over
// that prefix took spaces, tabs and quote markers and stopped at a carriage
// return, which the parser skips the same way: "code\n\r`" lost its "\r"
// between the content and the fence.
func TestCodeSpanClosingAfterABareCarriageReturn(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"bare CR before the closing fence", "A tick `code\n\r` in a sentence.\n"},
		{"bare CR then a space", "A tick `code\n\r ` in a sentence.\n"},
		{"space then a bare CR", "A tick `code\n \r` in a sentence.\n"},
		{"in a blockquote", "> A tick `code\n>\r` in a sentence.\n"},
		{"in a list item", "- A tick `code\n  \r` in a sentence.\n"},
		{"two bare CRs", "A tick `code\n\r\r` in a sentence.\n"},
		{"CRLF then a bare CR", "A tick `code\r\n\r` in a sentence.\r\n"},
		// The same walk stopped after one space of indent, because that space
		// had been taken as the fence's padding first.
		{"closing fence indented by two spaces", "A tick `code\n  ` in a sentence.\n"},
		{"closing fence indented by three spaces", "A tick `code\n   ` in a sentence.\n"},
		{"closing fence indented by a tab", "A tick `code\n\t` in a sentence.\n"},
		// The controls: the shapes #2481 and #2525 pinned, which carry no
		// bare carriage return.
		{"closing fence on its own line", "See ` code\n` here.\n"},
		{"crlf break", "See ` code\r\n` here.\r\n"},
		{"content carries the break, in a blockquote", "> `a\n> ` b"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			assertRebuildKeepsItsBlocks(t, tc.input)
		})
	}
}

// TestCodeSpanClosingAfterABareCarriageReturnRebuild pins the rebuild path's
// side: the fence is spelled from the vocabulary and the carriage return it
// carried goes with the rest of the markup, which is the accepted lossiness of
// that path; what matters is that the output is its own rebuild.
func TestCodeSpanClosingAfterABareCarriageReturnRebuild(t *testing.T) {
	t.Parallel()
	out := assertRebuildKeepsItsBlocks(t, "A tick `code\n\r` in a sentence.\n")
	assert.Equal(t, "A tick `code\n` in a sentence.\n", out)
}
