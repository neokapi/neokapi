package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRebuildLeavesCodeSpanContentLiteral is the #3217 reproducer. A code
// span's content is literal, so a "*", "_" or "<" inside it is never escaped:
// a backslash there is a character (CommonMark 6.1), and "`*`" came back as
// "`\*`" with the block's text changed on the first pass.
func TestRebuildLeavesCodeSpanContentLiteral(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"asterisk in a code span before an emphasis", "`*` and *a*"},
		{"underscores in a code span before an emphasis", "`_x_` and _y_"},
		{"angle bracket in a code span", "`<`"},
		{"angle bracket inside code span content", "`a<b` c"},
		{"emphasis markup inside a code span", "a `*b*` c"},
		{"backslash and asterisk in a code span", "`\\*` and *a*"},
		{"code span inside an emphasis", "*a `*` b*"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			out := assertRebuildKeepsItsBlocks(t, tc.input)
			assert.Equal(t, readBlockTexts(t, tc.input), readBlockTexts(t, out),
				"the rebuild changed the block's text")
		})
	}
}

// TestRebuildSpellsCodeSpanContentAsRead pins the rebuilt spellings.
func TestRebuildSpellsCodeSpanContentAsRead(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"asterisk stays bare", "`*` and *a*", "`*` and *a*\n"},
		{"underscores stay bare", "`_x_` and _y_", "`_x_` and *y*\n"},
		{"angle bracket stays bare", "`<`", "`<`\n"},
		{"angle bracket in content stays bare", "`a<b` c", "`a<b` c\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, assertRebuildKeepsItsBlocks(t, tc.input))
		})
	}
}
