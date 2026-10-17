package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRebuildLeavesADefinitionLookalikeOnALazyLineAlone covers a lazy
// continuation line that reads as a link reference definition. A definition
// cannot interrupt a paragraph, so the line is the paragraph's text, and in
// "[0]:0" over a quote whose lazy line is "[0]:0" its bracket opens a
// reference to the definition above, whose label is a block of its own. The
// rebuild escaped the bracket as it escapes any marker on a lazy line, and
// the reference and the label block were gone.
func TestRebuildLeavesADefinitionLookalikeOnALazyLineAlone(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"reference on a lazy quote line", "[0]:0\n>0\n[0]:0"},
		{"lookalike on a lazy quote line", ">0\n[a]:0"},
		{"lookalike on an item continuation line", "- 0\n  [a]:0"},
		{"reference on an item continuation line", "[0]:0\n- 0\n  [0]:0"},
		// Control: a definition lookalike on the quote's first line is a
		// definition inside the quote and stays escaped.
		{"lookalike on the quote's first line", ">[a]:0\n>b"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			out := assertRebuildKeepsItsBlocks(t, tc.input)
			assert.Equal(t, readBlockTexts(t, tc.input), readBlockTexts(t, out), "the rebuild changed the blocks")
		})
	}
	assert.Equal(t, "[0]: 0\n\n>0\n[0]:0\n", assertRebuildKeepsItsBlocks(t, "[0]:0\n>0\n[0]:0"))
}
