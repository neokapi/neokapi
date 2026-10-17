package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStrikethroughClosesAfterTheHeldEmphasisCloser covers emphasis that ends
// where the strikethrough around it ends. An emphasis closer is held until
// the byte after it is known, and the strikethrough closer was written before
// the held one was spelled, so "~!*~0*~~" came back as "~~!*~0~~*~" with the
// closers in the wrong order.
func TestStrikethroughClosesAfterTheHeldEmphasisCloser(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		// The literal tilde after the closer is escaped, as beside any
		// strikethrough closer.
		{"emphasis ending with the strikethrough, literal tilde after", "~!*~0*~~"},
		{"emphasis ending with the strikethrough", "~~a *b*~~"},
		{"strong ending with the strikethrough", "~~a **b**~~ c"},
		// Control: text between the two closers.
		{"emphasis then text inside the strikethrough", "~~*a* b~~"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			assertRebuildKeepsItsBlocks(t, tc.input)
		})
	}
	assert.Equal(t, "~~a *b*~~\n", assertRebuildKeepsItsBlocks(t, "~~a *b*~~"))
}
