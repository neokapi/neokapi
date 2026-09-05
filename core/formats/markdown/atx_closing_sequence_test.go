package markdown_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestATXClosingSequenceRoundTrips is the #2430 reproducer. An ATX heading's
// optional closing sequence (CommonMark 4.2) used to be written on a line of
// its own, so `# a #` came back as `# a` followed by `#`, which re-read as
// the heading plus an empty heading. The sequence now replays from source on
// the heading's own line, and the heading text never contains it.
func TestATXClosingSequenceRoundTrips(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		text  string // source text of the first block
	}{
		{"level one", "# a #\n", "a"},
		{"level two", "## b ##\n", "b"},
		{"longer sequence with trailing whitespace", "# c ###   \n", "c"},
		{"tab before the sequence", "# d\t#\n", "d"},
		{"at end of file without a newline", "# e #", "e"},
		{"as in the parity corpus", "### An h3 header ###\n", "An h3 header"},
		{"hashes inside the content", "# a # b #\n", "a # b"},
		{"escaped hash is content", "# a \\#\n", "a \\#"},
		{"no whitespace before the hash", "# a#\n", "a#"},
		{"followed by a paragraph", "# a #\n\nPara.\n", "a"},
		{"two headings in a row", "#### d ####\n# e #\n", "d"},
		{"in a blockquote", "> # q ##\n", "q"},
		{"in a list item", "- # li #\n", "li"},
		{"after a paragraph", "Para.\n\n# a #\n", "Para."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			blocks := assertSkeletonByteExact(t, tc.input)
			assert.Equal(t, tc.text, blocks[0].SourceText())
		})
	}
}

// TestATXClosingSequenceIsOneHeading pins the shape of the defect: the
// rebuilt document must read as one heading, with its level and text intact.
func TestATXClosingSequenceIsOneHeading(t *testing.T) {
	t.Parallel()
	out := roundtripWithSkeleton(t, "## Title ##\n")
	require.Equal(t, "## Title ##\n", out)
	blocks := readBlocks(t, out)
	require.Len(t, blocks, 1)
	assert.Equal(t, model.RoleHeading, blocks[0].SemanticRole())
	assert.Equal(t, 2, blocks[0].HeadingLevel())
	assert.Equal(t, "Title", blocks[0].SourceText())
}
