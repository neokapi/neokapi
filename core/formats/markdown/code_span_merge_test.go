package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRebuildMergesCodeSpansLeftTouching covers two code spans with inline
// HTML and nothing else between them. The HTML has no spelling on the rebuild
// path and is dropped, and two code spans with nothing between them have no
// spelling either: the closing fence of one touches the opening fence of the
// next and the two read as one run. The second span continues the first, and
// one span over both contents carries the same text.
func TestRebuildMergesCodeSpansLeftTouching(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"two spans with different fences", "``<A>`0``<A>`0`"},
		{"two one-backtick spans", "`a`<A>`b`"},
		{"three spans", "`a`<A>`b`<A>`c`"},
		{"span then emphasis then span", "`a`<A>*x*<A>`b`"},
		// Control: a space between the spans keeps them apart.
		{"two spans with a space between", "`a` `b`"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			out := assertRebuildKeepsItsBlocks(t, tc.input)
			assert.Equal(t, readBlockTexts(t, tc.input), readBlockTexts(t, out), "the rebuild changed the blocks")
		})
	}
	assert.Equal(t, "``<A>`00``\n", assertRebuildKeepsItsBlocks(t, "``<A>`0``<A>`0`"))
	assert.Equal(t, "`ab`\n", assertRebuildKeepsItsBlocks(t, "`a`<A>`b`"))
}
