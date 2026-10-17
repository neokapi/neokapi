package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRebuildCollapsesAReferenceThatOpensADefinitionLookalike covers a
// paragraph that opens with a shortcut reference link and reads, as written,
// as a link reference definition. A definition whose destination line is
// "[R]:0", followed by a line of two single quotes and a digit, is that
// definition and a paragraph that opens with a reference to it; rebuilt as
// "[R]:0" and the quote line, the paragraph was a definition, and the bracket
// escape that kept it a paragraph took the reference with it, and with the
// reference the definition's label block. The reference is spelled collapsed
// instead, "[R][]", which no definition opens with.
func TestRebuildCollapsesAReferenceThatOpensADefinitionLookalike(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"reference, colon and a destination", "[R]:\n[R]:0\n''0"},
		// Controls: a line that reads as no definition keeps the shortcut.
		{"text after the title", "[R]: x\n\n[R]:0 'y' z"},
		{"reference not at the line start", "[R]: x\n\na [R]:0"},
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

// TestRebuildCollapsesAReferenceThatOpensADefinitionLookalikeSpelling pins
// the spelling.
func TestRebuildCollapsesAReferenceThatOpensADefinitionLookalikeSpelling(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "[R]: [R]:0\n\n[R][]:0\n''0\n", assertRebuildKeepsItsBlocks(t, "[R]:\n[R]:0\n''0"))
	assert.Equal(t, "[R]: x\n\n[R]:0 'y' z\n", assertRebuildKeepsItsBlocks(t, "[R]: x\n\n[R]:0 'y' z"))
}

// TestRebuildCollapsesAReferenceAfterTheAngleEscape covers a line that reads
// as a definition only once the angle a dropped tag leaves is escaped:
// "[0]:<A>0" is text, and "[0]:\<A>0" a definition. The reference is
// spelled collapsed after that escape too; the escape itself is the known
// change to the text.
func TestRebuildCollapsesAReferenceAfterTheAngleEscape(t *testing.T) {
	t.Parallel()
	in := "[0]:0\n[0]:<<A>A>0"
	require.Equal(t, in, roundtripWithSkeleton(t, in), "skeleton path is not byte-exact")
	out := assertRebuildKeepsItsBlocks(t, in)
	assert.Len(t, readBlockTexts(t, out), len(readBlockTexts(t, in)), "the rebuild changed the block count")
}
