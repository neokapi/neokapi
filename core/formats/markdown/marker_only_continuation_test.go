package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMarkerOnlyContinuationLinesAreContent covers a paragraph of a vertical
// tab whose second line is inline HTML and a ">": once the HTML is dropped
// the line is a bare ">", and a block whose continuation lines all carry a
// quote marker is otherwise written as a quote. A quote body's lines carry
// content after their markers, so a line that is a marker alone is content
// and escaped: re-marked as a quote, the marker-only line was folded away and
// the quote of a vertical tab read back as no block.
func TestMarkerOnlyContinuationLinesAreContent(t *testing.T) {
	t.Parallel()
	in := "\v\n<A>>"
	require.Equal(t, in, roundtripWithSkeleton(t, in), "skeleton path is not byte-exact")
	assert.Equal(t, "\v\n\\>\n", assertRebuildKeepsItsBlocks(t, in))
}
