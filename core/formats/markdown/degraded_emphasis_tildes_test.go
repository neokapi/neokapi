package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDegradedEmphasisLeavesNoStrikethroughBehind covers emphasis the rebuild
// cannot spell and degrades to text, with literal tildes around it. GFM pairs
// tilde runs of any two lengths, so a literal tilde the degrade left bare
// paired with a degraded run: "*~!~~0~*~~!~" was written with its first tilde
// and its last two unescaped, and the pass after read a strikethrough from
// them and escaped it.
func TestDegradedEmphasisLeavesNoStrikethroughBehind(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"literal tildes after the degraded pairs", "*~!~~0~*~~!~"},
		{"degraded pairs alone", "*~!~~0~*"},
		{"literal tilde before and after", "~*~!~~0~*~"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			assertRebuildKeepsItsBlocks(t, tc.input)
		})
	}
}
