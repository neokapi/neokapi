package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRebuildEscapesBackticksThatFormACodeSpan covers raw text whose backticks
// form a code span once written as a paragraph: an HTML block's text is read
// verbatim, so "<div>` `" is one block whose text is "` `", and its rebuild
// re-read as a code span holding one space, which is no block at all.
func TestRebuildEscapesBackticksThatFormACodeSpan(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"code span of a space in an HTML block", "<div>` `"},
		{"code span in an HTML block", "<div>a `b` c"},
		{"double backticks in an HTML block", "<div>``a`` b"},
		// Controls: backticks the parser leaves as text stay bare.
		{"lone backtick in an HTML block", "<div>a ` b"},
		{"lone backtick in a paragraph", "a ` b"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			out := assertRebuildKeepsItsBlocks(t, tc.input)
			assert.Len(t, readBlockTexts(t, out), len(readBlockTexts(t, tc.input)), "the rebuild changed the block count")
		})
	}
}

// TestRebuildEscapesBackticksThatFormACodeSpanSpelling pins the escape.
func TestRebuildEscapesBackticksThatFormACodeSpanSpelling(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "\\` `\n", assertRebuildKeepsItsBlocks(t, "<div>` `"))
	assert.Equal(t, "a ` b\n", assertRebuildKeepsItsBlocks(t, "a ` b"))
}
