package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBreakAfterAnEscapedBackslashKeepsItsSpaces covers a hard break of two
// spaces right after an escaped backslash, inside a bare bracket, where the
// rebuild spells a break with a backslash. The parser reads a backslash after
// an escaped one as text whatever follows it, so the break it spelled was
// gone on the pass after, and the backslash it left was doubled as a literal
// one. After a backslash the break keeps its two-space spelling.
func TestBreakAfterAnEscapedBackslashKeepsItsSpaces(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"bracket, escaped backslash, break", "[\\\\  \n0"},
		{"bracket, text, escaped backslash, break", "[a\\\\  \n0"},
		// Controls: the same outside a bracket, and a break after text.
		{"escaped backslash, break", "\\\\  \n0"},
		{"bracket, text, break", "[a  \n0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			assertRebuildKeepsItsBlocks(t, tc.input)
		})
	}
	assert.Equal(t, "[\\\\  \n0\n", assertRebuildKeepsItsBlocks(t, "[\\\\  \n0"))
}
