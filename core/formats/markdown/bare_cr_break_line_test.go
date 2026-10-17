package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRebuildSpellsABreakAfterABareCarriageReturn covers a hard break on a
// line that holds only a bare carriage return before it. The parser keeps the
// "\r" inside the line as whitespace, and the sink took it for content, so the
// break on its own line spelled no backslash and the empty line ended the
// paragraph on re-read: ">0\n>\r\\\n0" came back as two blocks.
func TestRebuildSpellsABreakAfterABareCarriageReturn(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"in a quote", ">0\n>\r\\\n0", ">0\n>\r\\\n0\n"},
		{"in a paragraph", "0\n\r\\\n0", "0\n\r\\\n0\n"},
		{"in a quote with a space", "> 0\n> \r\\\n> 0", "> 0\n> \r\\\n> 0\n"},
		{"in a list item", "- 0\n  \r\\\n  0", "- 0\n  \r\\\n  0\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			assert.Equal(t, tc.want, assertRebuildKeepsItsBlocks(t, tc.input))
		})
	}
}
