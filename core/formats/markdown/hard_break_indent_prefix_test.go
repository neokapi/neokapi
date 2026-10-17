package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRebuildKeepsTheIndentOfHardBreakContinuationLines covers a paragraph
// whose lines after hard breaks share an indentation, which the reader takes
// out of the text and records as the lines' prefix. The rebuild dropped it,
// so a "#" four columns in stood at the line's start and was escaped there,
// while a pass that read the indented form kept the indentation in its text
// and escaped nothing: a block opening with a break, "0  ", a break and
// "    #" moved between its second and third rebuild.
func TestRebuildKeepsTheIndentOfHardBreakContinuationLines(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"opening break, then an indented marker line", "\\\n0  \\\n    #"},
		{"indented marker line", "0  \\\n    #"},
		{"indented text lines", "a\\\n    b\\\n    c"},
		{"tab-indented marker line", "a\\\n\t>b"},
		// A bare carriage return before the marker is whitespace to the
		// parser and the lines' prefix to the reader.
		{"carriage return before a marker line", "0  \\\n\r#"},
		{"opening break, then a carriage return before a marker line", "\\\n0  \\\n\r#"},
		{"carriage return before a quote marker line", "a\\\n\r>"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			assertRebuildKeepsItsBlocks(t, tc.input)
		})
	}
	assert.Equal(t, "0  \\\n    #\n", assertRebuildKeepsItsBlocks(t, "0  \\\n    #"))
	assert.Equal(t, "a\n    b\n    c\n", assertRebuildKeepsItsBlocks(t, "a\\\n    b\\\n    c"))
	assert.Equal(t, "0  \\\n\r#\n", assertRebuildKeepsItsBlocks(t, "0  \\\n\r#"))
}
