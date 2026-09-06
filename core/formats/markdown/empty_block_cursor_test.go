package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestContentLessBlockDoesNotRewindTheSkeleton is the #2495 reproducer. A block
// goldmark reports no lines for — an ATX heading with no text, an empty fenced
// code block — resolved to the range (0, 0), and assigning the skeleton cursor
// from it rewound to the start of the document: the gap that closed the file
// then wrote every byte before the block a second time.
func TestContentLessBlockDoesNotRewindTheSkeleton(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"empty heading after a heading", "# a\n#\n"},
		{"empty heading after a paragraph", "a\n#"},
		{"empty heading between paragraphs", "# Heading\n\ntext\n\n#\n\nmore text\n"},
		{"empty level-two heading", "a\n\n##\n\nb\n"},
		{"empty heading with a trailing space", "a\n\n# \n\nb\n"},
		{"empty fence at the end", "a\n```"},
		{"empty fence between paragraphs", "a\n\n```\n```\n\nb\n"},
		{"empty fence with a language", "a\n\n```js\n```\n\nb\n"},
		{"empty tilde fence", "a\n\n~~~\n~~~\n\nb\n"},
		{"empty heading opening the file", "#\n\ntext\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
		})
	}
}
