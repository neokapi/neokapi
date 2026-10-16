package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSetextHeadingKeepsItsIndentAfterAHardBreak is the #3104 reproducer. A
// paragraph whose line breaks are all hard ones hands the writer the
// continuation prefix of its lines through BlockPropLinePrefix, because each
// hard break contributes a bare "\n" and the indent the parser stripped has to
// come from somewhere. A setext heading is read from the same lines but set no
// such property, so the two-space indent of "  line two" was neither in the
// runs nor in the property and the heading came back two bytes shorter than
// the source spelled.
func TestSetextHeadingKeepsItsIndentAfterAHardBreak(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"indented continuation after a backslash break", "Line one\\\n  line two\n===\n"},
		{"indented continuation after a spaces break", "Line one  \n  line two\n---\n"},
		{"two indented continuations", "a\\\n  b\\\n  c\n===\n"},
		{"in a blockquote", "> Line one\\\n> line two\n> ===\n"},
		{"in a blockquote with an indent", "> Line one\\\n>   line two\n> ===\n"},
		{"in a list item", "- Line one\\\n  line two\n  ===\n"},
		// The controls: a soft break bakes the continuation into the runs, so
		// the property must stay unset or the indent doubles.
		{"soft break with an indent", "Line one\n  line two\n===\n"},
		{"hard and soft breaks", "Line one\\\n  line two\n  line three\n===\n"},
		{"no indent", "Line one\\\nline two\n===\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			blocks := readBlocks(t, tc.input)
			require.Len(t, blocks, 1, "source should read as one heading")
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			assertRebuildKeepsItsBlocks(t, tc.input)
		})
	}
}

// TestSetextHeadingWithAHardBreakRebuildsOnOneLine pins the rebuild path's
// side: the heading is written as ATX, the break and the indent fold to one
// space, and the output is its own rebuild.
func TestSetextHeadingWithAHardBreakRebuildsOnOneLine(t *testing.T) {
	t.Parallel()
	out := assertRebuildKeepsItsBlocks(t, "Line one\\\n  line two\n===\n")
	assert.Equal(t, "# Line one line two\n", out)
}
