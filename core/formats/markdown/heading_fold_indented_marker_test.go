package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHeadingFoldKeepsAnIndentedContentMarker covers a setext heading inside
// a quote whose second line is a tab and ">": four columns in, the ">" is
// content to the parser (CommonMark 4.8), and the heading's text is "a", a
// line ending, a tab and ">". The fold onto one ATX line took every ">" after
// the break for a quote marker, so the heading was written as "## a", and a
// heading whose text was a vertical tab and the ">" was written as a heading
// of whitespace, which is no block.
func TestHeadingFoldKeepsAnIndentedContentMarker(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		">a\n\t>b\n>-": "## a >b\n",
		">\v\n\t>\n>-": "## \v >\n",
		// A marked continuation line's marker goes with the break.
		">a\n>b\n>-": "## a b\n",
		// A line a dropped tag leaves as whitespace joins the breaks around
		// it into one space.
		"0\n <A>\n0\n-": "## 0 0\n",
	} {
		require.Equal(t, in, roundtripWithSkeleton(t, in), "skeleton path is not byte-exact")
		assert.Equal(t, want, assertRebuildKeepsItsBlocks(t, in), "input %q", in)
	}
}
