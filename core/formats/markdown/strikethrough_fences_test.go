package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestStrikethroughFencesComeFromSource is the #2500 reproducer. GFM spells a
// strikethrough with one tilde or two, and the reader walked greedily over the
// tildes on either side of the content, so an unbalanced pair claimed the
// tilde its neighbour carried and came back spelled three times. A pair around
// markup rather than text fell through to the default spelling and a
// single-tilde pair came back doubled.
func TestStrikethroughFencesComeFromSource(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"shorter closing run", "~~a~"},
		{"shorter opening run", "~a~~"},
		{"single tilde", "~one~ tilde\n"},
		{"double tilde", "~~gone~~ here\n"},
		{"around emphasis", "~*0*~"},
		{"around a code span", "~`a`~"},
		{"two pairs", "~~a~~b~~"},
		{"in a sentence", "A ~~b~~ and *c*\n"},
		{"single tilde around emphasis", "A ~*struck*~ word.\n"},
		{"around a link", "~[a](b)~"},
		{"double tilde around a link", "~~[a](b)~~"},
		{"spaced around a link", "~ [a](b) ~"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
		})
	}
}
