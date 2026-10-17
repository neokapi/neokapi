package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReferenceDefinitionTitleLineWithText is the #3219 reproducer. A title
// on the line after a definition's destination must be followed by nothing but
// whitespace (CommonMark 4.7); otherwise the definition ends at the
// destination and the line is a paragraph. The parser emits that paragraph and
// records the quoted part as the title anyway, and the rebuild wrote both: the
// definition gained a title and the re-read a block.
func TestReferenceDefinitionTitleLineWithText(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"quoted text then more on the title line", "[R]\n\n[R]:0\n'0'0"},
		{"quoted word then a word", "[R]\n\n[R]: /x\n'T' more"},
		{"double quotes then a word", "[R]\n\n[R]: /x\n\"T\" more\n"},
		// A destination holding a control byte goes between angle brackets,
		// or the title after it on the rebuilt line is not read.
		{"destination with a control byte, then a title line with text", "[R]:\n[R]\x06\n'0'0"},
		// A colon inside the label is not the definition's colon, and the
		// destination's spelling is sniffed after the right one.
		{"colon and angle in the label", "[:<]:>\n0"},
		// A destination holding an angle bracket and a control byte cannot go
		// between angle brackets and stays bare.
		{"angle and control byte in the destination", "[0]:>\b\n0"},
		{"colon in the label, bracketed destination", "[a:b]: <c>\n\n[a:b]"},
		// Controls: a title on its own line, and on the destination's line.
		{"title on its own line", "[R]\n\n[R]: /x\n'T'\n"},
		{"title on the destination's line", "[R]\n\n[R]: /x 'T'\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			out := assertRebuildKeepsItsBlocks(t, tc.input)
			assert.Equal(t, readBlockTexts(t, tc.input), readBlockTexts(t, out),
				"the rebuild changed the blocks")
		})
	}
}

// TestReferenceDefinitionWithoutTitleRebuildsWithoutOne pins the rebuild: the
// paragraph after the definition stays a paragraph.
func TestReferenceDefinitionWithoutTitleRebuildsWithoutOne(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "[R]\n\n[R]: 0\n\n'0'0\n", assertRebuildKeepsItsBlocks(t, "[R]\n\n[R]:0\n'0'0"))
}

// TestDefinitionTitleAfterATrailingBackslashKeepsItsSpace covers a definition
// whose bare destination ends in a backslash. The parser takes that backslash
// together with the space after it, so written with one space the title was
// not read and its block was gone; the title takes a second space.
func TestDefinitionTitleAfterATrailingBackslashKeepsItsSpace(t *testing.T) {
	t.Parallel()
	in := "[R]\n\n[R]:0\\  '0'"
	require.Equal(t, in, roundtripWithSkeleton(t, in), "skeleton path is not byte-exact")
	out := assertRebuildKeepsItsBlocks(t, in)
	assert.Equal(t, "[R]\n\n[R]: 0\\  \"0\"\n", out)
}
