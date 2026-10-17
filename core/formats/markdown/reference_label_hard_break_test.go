package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReferenceLabelKeepsItsHardBreakBackslash covers a shortcut reference
// whose text holds a hard break spelled with a backslash. The text is the
// reference's label, and the definition's label holds the backslash too, so
// the two match only while the rebuild keeps it: spelled as a soft break, the
// reference in "[a\\" over "b]0" matched nothing, and the definition's label
// block was gone with it.
func TestReferenceLabelKeepsItsHardBreakBackslash(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"break alone as the label", "[\\\n]:0\n[\\\n]0"},
		{"break inside the label", "[a\\\nb]:0\n[a\\\nb]0"},
		// A break of spaces keeps its spaces, since the label matches its
		// definition by its spelling.
		{"break of spaces inside the label", "[R  \n]\n\n[R  \n]:0"},
		// A bare carriage return among the spaces is whitespace the label's
		// normalization drops; a backslash in its place matched nothing.
		{"break of a carriage return and spaces inside the label", "[R\r  \n]\n\n[R]:0"},
		{"break of a tab and spaces inside the label", "[R\t  \n]\n\n[R]:0"},
		// Text between bare brackets may be read as a label on the pass
		// after, so its breaks keep their backslashes too.
		{"two breaks between bare brackets", "[\\ ]:0\n[\\\n\\\n]"},
		{"text and two breaks between bare brackets", "[a]:0\n[a\\\n\\\n]"},
		// A delimiter inside the label is matched as spelled and pairs with
		// nothing outside the brackets.
		{"asterisk as the label beside a literal asterisk", "[*]:0\n*[*]"},
		// Control: a break inside an inline link's text.
		{"break inside an inline link's text", "[a\\\nb](x)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			out := assertRebuildKeepsItsBlocks(t, tc.input)
			assert.Len(t, readBlockTexts(t, out), len(readBlockTexts(t, tc.input)), "the rebuild changed the block count")
		})
	}
	assert.Equal(t, "[a\\\nb]: 0\n\n[a\\\nb]0\n", assertRebuildKeepsItsBlocks(t, "[a\\\nb]:0\n[a\\\nb]0"))
}
