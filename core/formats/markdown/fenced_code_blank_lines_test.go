package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFencedCodeKeepsItsBlankLines is the #2509 reproducer. The reader skipped
// a blank line when it collected a fence's content, mirroring okapi's parser,
// and the skeleton refers to that content for the whole fence body: every code
// sample with a blank line in it came back with the spacing gone.
func TestFencedCodeKeepsItsBlankLines(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"blank line between statements", "```js\nconst a = 1;\n\nconst b = 2;\n```\n"},
		{"two blank lines", "```\na\n\n\nb\n```\n"},
		{"blank line with spaces", "```\na\n  \nb\n```\n"},
		{"blank first line", "```\n\na\n```\n"},
		{"blank last line", "```\na\n\n```\n"},
		{"tilde fence", "~~~\na\n\nb\n~~~\n"},
		{"fence between paragraphs", "Intro.\n\n```js\na\n\nb\n```\n\nAfter.\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
		})
	}
}

// TestFencedCodeRebuildIsAFixedPoint pins the rebuild path's side: the content
// carries its last line's ending and the closing fence opens with one, so one
// of them goes or every pass adds a blank line before the fence.
func TestFencedCodeRebuildIsAFixedPoint(t *testing.T) {
	t.Parallel()
	for _, src := range []string{"```\na\n\nb\n```\n", "```\n  indented\n```\n", "```js\na\n```\n"} {
		out := roundtrip(t, src)
		assert.Equal(t, out, roundtrip(t, out), "the rebuild is not idempotent for %q", src)
	}
}
