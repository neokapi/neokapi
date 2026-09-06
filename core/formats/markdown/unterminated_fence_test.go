package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestBlockAtEndOfFileKeepsItsEnding is the #2498 reproducer. goldmark's
// Segment.Value appends a newline to the last line of a block that runs to the
// end of the input, which is a byte the file does not have, and the block's
// content is what the skeleton refers to.
func TestBlockAtEndOfFileKeepsItsEnding(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"fence with no closing fence", "```\na"},
		{"tilde fence with no closing fence", "~~~\n~"},
		{"fence with a language", "```js\nconst a = 1;"},
		{"html block at the end", "<div>\nx"},
		{"indented code at the end", "    code"},
		{"closed fence at the end", "```\na\n```"},
		{"fence whose file ends with a newline", "```\na\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
		})
	}
}
