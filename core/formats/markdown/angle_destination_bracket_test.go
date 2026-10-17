package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAngleDestinationHoldingAnOpeningBracket covers a link whose destination
// is spelled between angle brackets and holds a "<" of its own. The parser
// takes every byte up to the closing ">", so "[a](<(<> 't')" is a link to
// "(<" with the title "t". The reader's scan of the closer stopped at the
// inner "<" and reported no closer, and the link was rebuilt from the parsed
// values: the skeleton path wrote the title between double quotes where the
// source had single ones, and the rebuild path changed the spelling on its
// second pass.
func TestAngleDestinationHoldingAnOpeningBracket(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"single-quoted title", "[a](<(<> 't')"},
		{"title holding a double quote", "[]((< '\"')"},
		{"title holding a double quote, angled destination", "[](<(<> '\"')"},
		{"destination of two brackets", "[a](<<>)"},
		// Control.
		{"destination with no bracket", "[a](<(> 't')"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			out := assertRebuildKeepsItsBlocks(t, tc.input)
			assert.Equal(t, readBlockTexts(t, tc.input), readBlockTexts(t, out), "the rebuild changed the blocks")
		})
	}
	assert.Equal(t, "[a](<(<> 't')\n", assertRebuildKeepsItsBlocks(t, "[a](<(<> 't')"))
}
