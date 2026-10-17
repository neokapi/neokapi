package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRebuildKeepsAControlCharacterAtTheBlockEdge covers a form feed or a
// vertical tab at the start of a block. The parser strips spaces, tabs and
// line endings from a paragraph's edges and keeps these, so the rebuild keeps
// them too. Trimmed away, a quote whose first line was a vertical tab lost
// that line, and the hard break on the line after it was then at the block's
// opening, where it is spelled as nothing, so the pass after had no break.
func TestRebuildKeepsAControlCharacterAtTheBlockEdge(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"vertical tab line then a break in a quote", ">\v\n>\\\n>0"},
		{"vertical tab line then a break", "\v\n\\\n0"},
		{"form feed before text", "\fx"},
		{"form feed before emphasis", "\f*a*"},
		{"form feed line in a quote", ">\f\n>0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			out := assertRebuildKeepsItsBlocks(t, tc.input)
			assert.Equal(t, readBlockTexts(t, tc.input), readBlockTexts(t, out), "the rebuild changed the blocks")
		})
	}
	assert.Equal(t, ">\v\n>\\\n>0\n", assertRebuildKeepsItsBlocks(t, ">\v\n>\\\n>0"))
}
