package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRebuildLeavesADefinitionLookalikeAlone covers a paragraph that begins
// like a link reference definition but is not one: its title is not closed,
// or text follows the title (CommonMark 4.7). The rebuild escaped the opening
// bracket as if the line were a definition, and when a definition of the label
// exists, that bracket is a reference link whose use keeps the definition's
// label translatable: "[R]:0\n[R]:0 \"" read as two blocks and its rebuild as
// one. The paragraph that becomes a definition once its angle bracket is
// escaped has its opening bracket escaped as well.
func TestRebuildLeavesADefinitionLookalikeAlone(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"definition then a lookalike with an unclosed title", "[R]:0\n[R]:0 \""},
		// A backslash before a space is a character of the destination, not
		// an escape, so the space ends it and "0" is no title: the line is a
		// paragraph holding a reference to the definition above it.
		{"definition then a lookalike with a backslash before a space", "[R]:0\n[R]:\\ 0"},
		{"lookalike with a backslash before a space", "[R]:\\ 0"},
		// A label holds no unescaped bracket, so a line opening with "[[]:" is
		// no definition: the first bracket opens a link whose text holds the
		// rest, and escaping it lost the link.
		{"bracket inside the label, link around it", "[[]:[](]()"},
		{"bracket inside the label", "[[]:x"},
		{"unclosed double-quoted title", "[R]:0 \""},
		{"unclosed single-quoted title", "[R]: /u 't"},
		{"unclosed parenthesised title", "[R]: /u (t"},
		{"text after the title", "[R]: /u \"t\" x"},
		{"text after a single-quoted title", "[R]: /u 't' x\n"},
		// A control character that is not whitespace stays in a bare
		// destination for the parser, so the escaped angle bracket before it
		// still spells a definition and the bracket is escaped with it.
		{"bare angle then a bell character", "[0]:<\a"},
		{"bare angle then a control character", "[0]:<\x01"},
		{"bare angle then a vertical tab", "[0]:<\v0"},
		// A label may span lines, so the definition this becomes once its
		// angle is escaped has its bracket escaped too.
		{"label spanning a line, bare angle", "[\n0]:<"},
		{"label spanning a line, destination", "[a\nb]:<"},
		// A vertical tab or a form feed is a label to the parser.
		{"label that is a vertical tab", "[\v]:<"},
		{"label that is a form feed", "[\f]:<"},
		{"bare angle then a form feed", "[0]:<\f0"},
		// Controls: the shapes #2526 escapes.
		{"label colon and a bare angle", "[a]:<"},
		{"label colon and a destination with a title", "[a]:< 'title'"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			out := assertRebuildKeepsItsBlocks(t, tc.input)
			assert.Len(t, readBlockTexts(t, out), len(readBlockTexts(t, tc.input)), "the rebuild changed the block count")
		})
	}
}

// TestRebuildLeavesADefinitionLookalikeAloneSpelling pins the rebuild: the
// lookalike's bracket stays bare.
func TestRebuildLeavesADefinitionLookalikeAloneSpelling(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "[R]: 0\n\n[R]:0 \"\n", assertRebuildKeepsItsBlocks(t, "[R]:0\n[R]:0 \""))
	assert.Equal(t, "[R]: /u \"t\" x\n", assertRebuildKeepsItsBlocks(t, "[R]: /u \"t\" x"))
}
