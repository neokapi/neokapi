package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestStrikethroughPairsTheParserNestsBeyondLocatingAreDegraded covers tilde
// runs the parser pairs into a nesting the rebuild cannot locate in its own
// output: "~0 ~0~~!~ 0~0~0" is two strikethroughs closing together, a literal
// tilde and a third, and written as "~~0 ~~0~~~~!~ 0~~0~~0" the parser forms
// four nested strikethroughs around different text. A verdict it cannot give
// on the pairs is no verdict for them, and the pairs are written as text.
func TestStrikethroughPairsTheParserNestsBeyondLocatingAreDegraded(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"nested pair, literal tilde, third pair", "~0 ~0~~!~ 0~0~0"},
		{"nested pair, literal tilde", "~0 ~0~~!~"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			assertRebuildKeepsItsBlocks(t, tc.input)
		})
	}
}
