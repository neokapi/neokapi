package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestQuoteFirstLineKeepsItsOwnMarkerAfterAHardBreak covers a blockquote
// whose line after a hard break is indented past the marker: ">a" over
// ">     0". The continuation prefix the reader records is the marker with
// that indentation, and the rebuild marked the first line with it too, so
// "a" sat four columns in and read back as indented code inside the quote.
// The first line takes the marker its own line carried.
func TestQuoteFirstLineKeepsItsOwnMarkerAfterAHardBreak(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"indented continuation after a hard break", ">a\\\n>     0"},
		{"opening break, lazy line, indented continuation", ">\\\n0  \\\n>     0"},
		// Four columns past the marker a "#" opens nothing; escaped at the
		// line's start, before the prefix went back, it drifted.
		{"opening break, lazy line, indented marker line", ">\\\n0  \\\n>     #"},
		// Control: a continuation line indented as far as the first.
		{"unindented continuation after a hard break", ">a\\\n>0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			out := assertRebuildKeepsItsBlocks(t, tc.input)
			assert.Len(t, readBlockTexts(t, out), len(readBlockTexts(t, tc.input)), "the rebuild changed the block count")
		})
	}
	assert.Equal(t, ">a\n>     0\n", assertRebuildKeepsItsBlocks(t, ">a\\\n>     0"))
}
