package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRebuildFencesACodeSpanWithTheShortestFreeRun covers a code span that
// opens a block and whose content begins with a line break and holds a run of
// two backticks. A fence one longer than the longest run inside is three
// backticks before a line ending, which is a fenced code block's opener, and
// the escape that keeps a paragraph from opening one broke the span: a
// one-backtick span whose content is a line break, two backticks and "<"
// came back fenced with three backticks, escaped, and read as something
// else. The shortest run no run inside matches is one backtick, as the source
// spelled.
func TestRebuildFencesACodeSpanWithTheShortestFreeRun(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"span opening a block, content starting with a break", "`\n``<`"},
		{"span opening a block, text then a run", "`a\n``b`"},
		{"span opening a block, content starting with a break then text", "`\n``a`"},
		{"span inside a sentence", "x `\n``<` y"},
		// Controls.
		{"space before the run", "` ``\n<`"},
		{"angle bracket then a break", "`<\n`"},
		{"run of one inside", "`` a`b ``"},
		{"runs of one and two inside", "``` a`b``c ```"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			out := assertRebuildKeepsItsBlocks(t, tc.input)
			assert.Equal(t, readBlockTexts(t, tc.input), readBlockTexts(t, out), "the rebuild changed the block's text")
		})
	}
}

// TestRebuildFencesACodeSpanWithTheShortestFreeRunSpelling pins the fences.
func TestRebuildFencesACodeSpanWithTheShortestFreeRunSpelling(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "`\n``<`\n", assertRebuildKeepsItsBlocks(t, "`\n``<`"))
	assert.Equal(t, "``a`b``\n", assertRebuildKeepsItsBlocks(t, "`` a`b ``"))
	assert.Equal(t, "```a`b``c```\n", assertRebuildKeepsItsBlocks(t, "``` a`b``c ```"))
}
