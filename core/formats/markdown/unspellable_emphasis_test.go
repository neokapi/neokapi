package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRebuildDegradesEmphasisWithNoSpelling covers emphasis the rebuild path
// cannot spell at all. The inline HTML in "0<A>**0*!*" is dropped, which puts
// two nested pairs directly after a digit: an opener there has to merge with
// the inner one to open, and CommonMark's rule of three then keeps the merged
// run from pairing with a closer of one. The first pass wrote the pairs as
// markup that read back as text, and the second pass escaped that text, so
// the block's identity moved after the first pass.
func TestRebuildDegradesEmphasisWithNoSpelling(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"two pairs after dropped HTML and a digit", "0<A>**0*!*"},
		{"two pairs after dropped HTML and a letter", "x<A>**0*!*"},
		// The dropped HTML leaves the outer opener before a space, where it
		// cannot open, and the line it opens is a bullet to the block path.
		{"opener before a space after dropped HTML", "*<A> *0*0*"},
		{"opener before a space and text after dropped HTML", "*<A> a*"},
		// A form feed at the block's edge is content, so the parser check and
		// the block path judge the same text.
		{"form feed before the opener", "\f*<A> *0*0*"},
		// Controls: pairs the drop leaves spellable.
		{"one pair after dropped HTML", "0<A>*0*"},
		{"strong after dropped HTML", "a<A>**b**"},
		{"underscore pair after dropped HTML", "0<A>_0_"},
		{"literal and two pairs after dropped HTML", "x<A>***0*!*"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			assertRebuildKeepsItsBlocks(t, tc.input)
		})
	}
}

// TestRebuildDegradesEmphasisWithNoSpellingSpelling pins the degraded form:
// the delimiters are text, escaped where they could pair, which is what the
// pass after would have written.
func TestRebuildDegradesEmphasisWithNoSpellingSpelling(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "0\\*\\*0\\*!\\*\n", assertRebuildKeepsItsBlocks(t, "0<A>**0*!*"))
	assert.Equal(t, "0*0*\n", assertRebuildKeepsItsBlocks(t, "0<A>*0*"))
}
