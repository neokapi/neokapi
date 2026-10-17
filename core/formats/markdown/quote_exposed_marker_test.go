package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRebuildEscapesAMarkerExposedOnAQuoteLine covers a blockquote whose
// continuation line held inline HTML right after its marker, followed by a
// ">" that is content. The rebuild drops the HTML, the two markers touch, and
// the writer took both for the line's prefix: ">00\n><A>>0" was rebuilt as
// ">00\n>>0", which opens a nested quote and splits the block.
func TestRebuildEscapesAMarkerExposedOnAQuoteLine(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"marker, HTML, marker", ">00\n><A>>0"},
		{"spaced markers", "> 00\n> <A>> 0"},
		{"nested quote", ">> a\n>> <A>>b"},
		// The HTML was the line's only content, so the line it leaves holds
		// the marker alone and is folded away.
		{"marker-only line after dropped HTML", ">00\n><A>\n0"},
		{"spaced marker-only line after dropped HTML", "> a\n> <A>\n> b"},
		{"nested marker-only line after dropped HTML", ">> a\n>> <A>\n>> b"},
		// Controls.
		{"HTML on a lazy line", "> a\n<A>> b"},
		{"HTML before a bullet in a list item", "- a\n  <A>- b"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			out := assertRebuildKeepsItsBlocks(t, tc.input)
			assert.Len(t, readBlockTexts(t, out), 1)
		})
	}
}

// TestRebuildEscapesAMarkerExposedOnAQuoteLineSpelling pins the escape.
func TestRebuildEscapesAMarkerExposedOnAQuoteLineSpelling(t *testing.T) {
	t.Parallel()
	assert.Equal(t, ">00\n>\\>0\n", assertRebuildKeepsItsBlocks(t, ">00\n><A>>0"))
}
