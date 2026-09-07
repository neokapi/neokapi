package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHeadingHashResidueStaysContent is the #2484 reproducer. The rebuild path
// drops inline HTML, which is accepted lossiness, and the residue was a bare
// "#". Written after "# " it landed where CommonMark 4.2 reads an ATX closing
// sequence, so the heading carried no content and re-read as no block at all.
func TestHeadingHashResidueStaysContent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		written string
	}{
		{"bare hash", "# <A>#", "# \\#\n"},
		{"level two", "## <A>#", "## \\#\n"},
		{"two hashes", "# <A>##", "# \\##\n"},
		{"hash after a word", "# a <A>#", "# a \\#\n"},
		{"hash run with no html", "0\n# # #", "0\n\n# \\#\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out := roundtrip(t, tc.input)
			assert.Equal(t, tc.written, out)
			// The heading survives the round trip: reading the rebuilt document
			// yields a block, and rebuilding it again is a fixed point.
			require.NotEmpty(t, readBlocks(t, out), "the rebuilt heading re-reads as no block")
			assert.Equal(t, out, roundtrip(t, out), "the rebuild is not idempotent")
		})
	}
}

// TestHeadingClosingSequenceAndContentHashesAreLeftAlone pins the two shapes
// the escape must not touch: a hash the source meant as content because no
// whitespace precedes it, and a real closing sequence the reader already left
// out of the block's text.
func TestHeadingClosingSequenceAndContentHashesAreLeftAlone(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "# C#\n", roundtrip(t, "# C#\n"))
	assert.Equal(t, "# a\n", roundtrip(t, "# a #\n"))
	assert.Equal(t, "# a\n", roundtrip(t, "# a\n"))
}
