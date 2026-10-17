package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestItemKeepsALazyLineThatIsABareBullet covers a list item whose paragraph
// runs on into a line that is a bullet and a carriage return, which the parser
// reads as the text "*": the item's text is "x" over "*". The sink escapes a
// marker on an item's continuation line as the block path does: written bare,
// the convergence re-read saw "- x" over a bare "*", an item and an empty
// one, and took the item's text without its second line.
func TestItemKeepsALazyLineThatIsABareBullet(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"* x\n*\r":  "- x\n\\*\n",
		"* \f\n*\r": "- \f\n\\*\n",
		// A tab and "=" after a quoted item's lazy line is text in the
		// quote, and rebuilt inside the item alone, four columns in, it
		// underlined the lines above into a heading.
		">* 0\n0\n\t=": "- 0\n0\n\t\\=\n",
	} {
		require.Equal(t, in, roundtripWithSkeleton(t, in), "skeleton path is not byte-exact")
		assert.Equal(t, want, assertRebuildKeepsItsBlocks(t, in), "input %q", in)
	}
}
