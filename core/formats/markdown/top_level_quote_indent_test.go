package markdown_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/formats/markdown"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTopLevelQuoteDropsTheIndentBeforeItsMarker covers a quote whose first
// marker sits behind two spaces at the top level, after a list item and an
// empty heading: "* 0", "#", "  >0" over ">0". The heading emits no block and
// is gone from the rebuild, and with the indent kept on the recorded marker
// the quote was written inside the item, where its second line, unindented,
// opened a quote of its own. The indent before a top-level quote's marker
// means nothing (CommonMark 5.1) and is not recorded; inside a list item it
// is the item's content column and stays.
func TestTopLevelQuoteDropsTheIndentBeforeItsMarker(t *testing.T) {
	t.Parallel()
	in := "* 0\n#\n  >0\n>0"
	require.Equal(t, in, roundtripWithSkeleton(t, in), "skeleton path is not byte-exact")
	assert.Equal(t, "- 0\n\n>0\n>0\n", assertRebuildKeepsItsBlocks(t, in))

	nested := "- a\n\n  > q\n  > r"
	require.Equal(t, nested, roundtripWithSkeleton(t, nested), "skeleton path is not byte-exact")
	assert.Equal(t, "- a\n\n  > q\n  > r\n", assertRebuildKeepsItsBlocks(t, nested))
	blocks := readBlocks(t, nested)
	require.Len(t, blocks, 2)
	assert.Equal(t, "  > ", blocks[1].Properties[markdown.BlockPropQuoteMarker])

	// A tab later in the marker reaches a column the indent sets, so the
	// indent stays: without it the tab reached the next stop and ">0" was
	// indented code inside the quote.
	tabbed := "   >   \t>0\n0"
	require.Equal(t, tabbed, roundtripWithSkeleton(t, tabbed), "skeleton path is not byte-exact")
	assert.Equal(t, "   >   \t>0\n0\n", assertRebuildKeepsItsBlocks(t, tabbed))
}
