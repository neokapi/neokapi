package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestItemChecksItsTextInsideTheItem covers a list item whose lazy line is a
// tab and "<!A": four columns in at a line's start the line is text, and two
// columns into the item rebuilt after "- " it opens an HTML block, which
// split the item. The checks on the rendered text read it inside the item
// the block path writes, where the parse is no single paragraph and the
// angle is escaped.
func TestItemChecksItsTextInsideTheItem(t *testing.T) {
	t.Parallel()
	in := "*\t 0\n\t<!A\n0"
	require.Equal(t, in, roundtripWithSkeleton(t, in), "skeleton path is not byte-exact")
	assert.Equal(t, "- 0\n\t\\<!A\n0\n", assertRebuildKeepsItsBlocks(t, in))
}
