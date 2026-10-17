package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestContentMarkerAfterADroppedTagIsEscaped covers a paragraph whose second
// line opens with inline HTML and a ">": "***!*!*!*" over "<A>>0". The HTML
// is dropped on the rebuild and the ">" it kept from being a marker opens the
// line, so it is escaped as content: written bare, every continuation line
// carried a marker, the block was recovered as a quote, and its emphasis went
// unchecked. The same with a marker-only line between, which the block path
// would fold.
func TestContentMarkerAfterADroppedTagIsEscaped(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"***!*!*!*\n<A>>0":       "___!_!_!_\n\\>0\n",
		"***!*!*!*\n<A>>\n<A>>0": "___!_!_!_\n\\>\n\\>0\n",
		">\v\n<A>>":              ">\v\n\\>\n",
		">0\n><A>>0":             ">0\n>\\>0\n",
	} {
		require.Equal(t, in, roundtripWithSkeleton(t, in), "skeleton path is not byte-exact")
		assert.Equal(t, want, assertRebuildKeepsItsBlocks(t, in), "input %q", in)
	}
}
