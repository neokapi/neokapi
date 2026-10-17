package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTableCellCodeSpanPipeKeepsItsBackslashes covers a table cell holding a
// code span whose content ends in backslashes and a pipe. The row splitter
// takes the backslash of each escaped pipe out of a code span's content,
// where backslashes are characters, so content of two backslashes and a pipe
// is spelled with three. The writer escaped a pipe only when no backslash
// stood before it, wrote the content with two, and the pass after read it
// with one; the skeleton path lost the backslash the same way.
func TestTableCellCodeSpanPipeKeepsItsBackslashes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"two backslashes and a pipe in a code span", "`\\\\\\|`\n-|"},
		{"one backslash and a pipe in a code span", "`\\|`\n-|"},
		// A backtick with no closing run opens no span, and the pipe after
		// it is the cell's own escaped pipe; an escaped backtick is text and
		// opens none either.
		{"unclosed backtick before an escaped pipe", "0\n-|\n`\\|0"},
		{"escaped backtick, escaped pipe, backtick", "\\`\\|`\n-|"},
		// Inside a span a backslash is a character, so the second backtick
		// closes the span and the pipe after it is the cell's own.
		{"span of one backslash, then an escaped pipe", "`\\`\\|`\n-|"},
		// Controls: a pipe escaped in the cell's own text, and a code span
		// outside a table.
		{"escaped pipe in text", "a\\|b\n-|"},
		{"code span outside a table", "`\\\\\\|`"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			out := assertRebuildKeepsItsBlocks(t, tc.input)
			assert.Equal(t, readBlockTexts(t, tc.input), readBlockTexts(t, out), "the rebuild changed the blocks")
		})
	}
	assert.Equal(t, []string{"\\\\|"}, readBlockTexts(t, "`\\\\\\|`\n-|"))
}
