package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRebuildKeepsAControlOnlyLine covers a paragraph line holding only a form
// feed or a vertical tab. Such a line is content to the parser, where a line
// of spaces and tabs is blank, so the paragraph runs on through it. The
// rebuild folded it away with the blank lines, and "<A\n\f\nA>" became
// "<A\nA>", an inline tag with no block around it.
func TestRebuildKeepsAControlOnlyLine(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"form feed between two halves of a tag", "<A\n\f\nA>"},
		{"vertical tab line", "a\n\v\nb"},
		{"form feed line in a quote", "> a\n>\f\n> b"},
		{"form feed line in an item", "- a\n\f\n  b"},
		// Control: a line of spaces and a tab is blank and ends the paragraph.
		{"spaces and a tab", "a\n \t\nb"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			out := assertRebuildKeepsItsBlocks(t, tc.input)
			assert.Equal(t, readBlockTexts(t, tc.input), readBlockTexts(t, out), "the rebuild changed the blocks")
		})
	}
}
