package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRebuildChecksTheTextWithItsBlankLinesFolded covers a block whose text
// holds a blank line, which the rebuild folds away: an HTML block opened by a
// processing instruction runs to the end of the document, blank lines
// included. The checks the rebuild runs against the parser saw the text with
// the blank line in place, where it was two paragraphs and no check applied,
// and the literal tildes in "<?0>~00~~", a blank line and "0" went unescaped
// to pair as a strikethrough on the pass after.
func TestRebuildChecksTheTextWithItsBlankLinesFolded(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"literal tildes before a blank line", "<?0>~00~~\n\n0"},
		{"literal tildes and a space before a blank line", "<?0>~00~~ \n\n0"},
		{"literal asterisks around a blank line", "<?0>*a\n\nb*"},
		{"angle before a blank line", "<?0>a <b\n\n> c"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			assertRebuildKeepsItsBlocks(t, tc.input)
		})
	}
	assert.Equal(t, "\\~00\\~\\~\n0\n", assertRebuildKeepsItsBlocks(t, "<?0>~00~~\n\n0"))
}
