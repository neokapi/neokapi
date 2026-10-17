package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDefinitionFragmentReplaysAsBytes covers a link reference definition
// whose label spans two lines and whose title line fails. The parser keeps
// the definition it registered, label, destination and title, but its node
// covers the first line alone, "[", and the lines after it are emitted again
// as a paragraph. Rebuilt from the registered values, the definition wrote
// the label and the title a second time beside that paragraph, and the
// skeleton path stopped being byte-exact. The fragment replays as its bytes,
// and a reference the paragraph holds to it is text.
func TestDefinitionFragmentReplaysAsBytes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"collapsed reference in the title line", "[\n0]:0\n'[][\n0]'0"},
		{"two-line label, collapsed reference", "[a\nb]:0\n'[][a\nb]'0"},
		{"title line with no reference", "[\n0]:0\n'x'0"},
		{"full reference to the fragment", "[\n0]:0\n'[x][\n0]'0"},
		// The reference spells the label without the line ending; labels
		// match with their whitespace collapsed.
		{"reference spelled on one line", "[ \n0]:0\n''[][0]"},
		// Control: a one-line label keeps its definition.
		{"one-line label", "[a]:0\n'[][a]'0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			out := assertRebuildKeepsItsBlocks(t, tc.input)
			assert.Len(t, readBlockTexts(t, out), len(readBlockTexts(t, tc.input)), "the rebuild changed the block count")
		})
	}
	assert.Equal(t, []string{"0]:0\n'[][\n0]'0"}, readBlockTexts(t, "[\n0]:0\n'[][\n0]'0"))
}

// TestDefinitionFragmentThenWholeDefinitionKeepsItsBytes covers a fragment
// followed by a whole definition of the same label, with a paragraph the
// parser forms over the fragment's lines and a line after them. The
// paragraph's first line is in the skeleton already, so it emits no block and
// the image reference it holds makes no label visible: the document has
// nothing to translate, and the skeleton path keeps its bytes.
func TestDefinitionFragmentThenWholeDefinitionKeepsItsBytes(t *testing.T) {
	t.Parallel()
	in := "[\n0]:0\n''[0]:0\n[![0]"
	require.Equal(t, in, roundtripWithSkeleton(t, in), "skeleton path is not byte-exact")
	assert.Empty(t, readBlockTexts(t, in))
}

// TestReferenceInAReEmittedLineMakesNothingVisible covers a definition whose
// title spans two lines and fails, where the parser emits the title's first
// line again as a paragraph after a second definition: "[0]:0" over "'[0]"
// over "'[0]:0". The paragraph's bytes are in the skeleton already and it
// emits no block, so the reference it holds makes no label visible: the
// document has nothing to translate, and both paths keep its bytes.
func TestReferenceInAReEmittedLineMakesNothingVisible(t *testing.T) {
	t.Parallel()
	in := "[0]:0\n'[0]\n'[0]:0"
	require.Equal(t, in, roundtripWithSkeleton(t, in), "skeleton path is not byte-exact")
	assert.Empty(t, readBlockTexts(t, in))
}
