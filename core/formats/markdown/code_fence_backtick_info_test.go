package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRebuildFencesACodeBlockWhoseInfoStringHoldsABacktick covers a tilde
// fence whose info string holds a backtick, which a tilde fence allows and a
// backtick fence does not (CommonMark 4.5). The rebuild spelled every fence
// with backticks, so "~~~0`\n\n0" came back as a paragraph and a stray fence,
// two blocks for one.
func TestRebuildFencesACodeBlockWhoseInfoStringHoldsABacktick(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"unterminated tilde fence, backtick in the info string", "~~~0`\n\n0"},
		{"closed tilde fence, backtick in the info string", "~~~a`b\nx\n~~~"},
		// Controls.
		{"tilde fence holding a backtick", "~~~\n`\n~~~"},
		{"backtick fence with a language", "```go\nx\n```"},
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

// TestRebuildFencesACodeBlockWhoseInfoStringHoldsABacktickSpelling pins the
// tilde fence.
func TestRebuildFencesACodeBlockWhoseInfoStringHoldsABacktickSpelling(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "~~~a`b\nx\n~~~\n", assertRebuildKeepsItsBlocks(t, "~~~a`b\nx\n~~~"))
}
