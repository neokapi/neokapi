package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRebuildChecksTheTextWithItsLineMarkersEscaped covers a block whose
// continuation line opens with a block marker the rebuild escapes: an HTML
// block opened by a processing instruction runs on through a line holding
// "#". The checks the rebuild runs against the parser saw the text before the
// escape, where the "#" line was a heading and the text no single paragraph,
// so no check applied and the literal tildes in "<?0>~0~~" went unescaped, to
// pair as a strikethrough on the pass after.
func TestRebuildChecksTheTextWithItsLineMarkersEscaped(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"literal tildes before a heading marker line", "<?0>~0~~\n#"},
		{"literal tildes before a quote marker line", "<?0>~0~~\n>"},
		{"literal tildes before a setext underline", "<?0>~0~~\n-"},
		// A bullet with a form feed after it has content to the parser and
		// interrupts the paragraph.
		{"bullet with a form feed on a continuation line", "<p>0\n* \f\n0"},
		{"literal tildes before a delimiter row", "<?0>|~0~~\n-|"},
		{"literal asterisks around a bullet line", "<?0>*a\n- b*"},
		{"angle before a quote marker line", "<?0>a <b\n> c"},
		// Control: a continuation line that opens no block.
		{"literal tildes before text", "<?0>~0~~\nx"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			assertRebuildKeepsItsBlocks(t, tc.input)
		})
	}
	assert.Equal(t, "\\~0\\~\\~\n\\#\n", assertRebuildKeepsItsBlocks(t, "<?0>~0~~\n#"))
}
