package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAdjacentEmphasisKeepsItsDelimiters is the #2499 reproducer. Two emphasis
// nodes that touch resolve to the same start through their neighbours, so the
// second inherited the first's spelling on the byte-exact path.
func TestAdjacentEmphasisKeepsItsDelimiters(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"underscore then asterisk", "_0__*0*"},
		{"asterisk then underscore", "*0*__0_"},
		{"separated pairs", "_a_ and *b*"},
		{"strong pairs", "**a** and __b__"},
		{"emphasis around a link", "_[a](b)_"},
		{"emphasis around an image", "__![i](s)__"},
		{"emphasis around emphasis", "_*a*_"},
		{"emphasis then text", "_0__)_"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
		})
	}
}

// TestTouchingEmphasisRebuildsAsTwoPairs pins the rebuild path's side: two
// asterisk pairs written back-to-back spell a run of four, which CommonMark
// reads as one pair around the middle, so the block's content changed on the
// next pass. The second pair spells underscores, which read as a pair there.
func TestTouchingEmphasisRebuildsAsTwoPairs(t *testing.T) {
	t.Parallel()
	for _, src := range []string{"_!_*0*", "_0__*0*", "__0___)_", "*a* and *b*"} {
		out := roundtrip(t, src)
		assert.Equal(t, out, roundtrip(t, out), "the rebuild is not idempotent for %q", src)
	}
}
