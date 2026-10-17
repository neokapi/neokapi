package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTableCellCharacterReferenceKeepsItsSpelling covers a table cell holding
// a character reference. The reader hands the reference over as a placeholder
// and a cell that is one reference alone has no text, so it is no block; the
// cell was rendered with the reference's characters, which the pass after read
// as text and a block the source did not have.
func TestTableCellCharacterReferenceKeepsItsSpelling(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"cell of one reference", "0\n|-\n&DD;"},
		{"cell of a reference and text", "0\n|-\n&DD; x"},
		{"header cell of a reference and text", "| &amp; a |\n|-|\n| b |"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			assertRebuildKeepsItsBlocks(t, tc.input)
		})
	}
	assert.Equal(t, "| 0 |\n| --- |\n| &DD; |\n", assertRebuildKeepsItsBlocks(t, "0\n|-\n&DD;"))
}

// TestTableCellKeepsALeadingMarkerAfterADroppedTag covers a header cell of an
// inline tag, a space and ">": the tag is dropped, the cell is written as
// "| > |", and the pass after, whose cell text begins with the ">", escaped
// it, since the sink for a cell started with the dropped-construct position
// at zero. A ">" at a cell's start opens nothing and stays bare.
func TestTableCellKeepsALeadingMarkerAfterADroppedTag(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"<A> >\n-|": "| > |\n| --- |\n",
		"<A>>\n-|":  "| > |\n| --- |\n",
	} {
		require.Equal(t, in, roundtripWithSkeleton(t, in), "skeleton path is not byte-exact")
		assert.Equal(t, want, assertRebuildKeepsItsBlocks(t, in), "input %q", in)
	}
}
