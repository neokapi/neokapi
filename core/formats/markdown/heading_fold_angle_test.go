package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestHeadingAngleIsJudgedOnTheFoldedLine covers a setext heading whose text
// opens with "<p", a tab and a line ending. As two lines the text is a
// paragraph to the parser and the angle opens nothing; folded onto the one
// ATX line the heading is written as, "<p 0" opens an HTML block, and the
// pass after escaped the angle. The heading's lines are folded before the
// checks run.
func TestHeadingAngleIsJudgedOnTheFoldedLine(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"tag opener, tab, line ending", "<p\t\n0\n-"},
		// Control: the tag opener and the text on one line.
		{"tag opener and text on one line", "<p\t0\n-"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			assertRebuildKeepsItsBlocks(t, tc.input)
		})
	}
	assert.Equal(t, "## \\<p 0\n", assertRebuildKeepsItsBlocks(t, "<p\t\n0\n-"))
}
