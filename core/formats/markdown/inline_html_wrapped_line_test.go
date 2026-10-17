package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestInlineHTMLWrappingOntoAnIndentedLine is the #3215 reproducer. An inline
// tag may wrap onto the next line (CommonMark 6.6), and the parser records
// that line without the whitespace it strips from a continuation line, so the
// tab or bare carriage return the line began with reached neither the tag's
// bytes nor the skeleton.
func TestInlineHTMLWrappingOntoAnIndentedLine(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"tab before the closing bracket", "a <x\n\t> b"},
		{"tab after an attribute", "a <x y=\"1\"\n\t> b"},
		{"bare carriage return before the closing bracket", "a <x\n\r> b"},
		{"tab inside a blockquote", "> a <x\n> \t> b"},
		{"tab inside a list item", "- a <x\n  \t> b"},
		// Controls: a wrapped tag with no indent, and a line beginning with a
		// space, which the parser reads as text rather than a tag.
		{"no indent", "a <x\n> b"},
		{"space before the closing bracket", "a <x\n > b"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			assertRebuildKeepsItsBlocks(t, tc.input)
		})
	}
}
