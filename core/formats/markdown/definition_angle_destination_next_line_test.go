package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDefinitionAngleDestinationOnTheNextLine covers a link reference
// definition whose destination, spelled between angle brackets, sits on the
// line after the label. The reader looked for the opening bracket on the
// label's line only and recorded the destination bare, so an empty "<>" was
// recorded as nothing and the rebuild wrote "[0]: ", which is no definition:
// the paragraph after it merged into one block with it.
func TestDefinitionAngleDestinationOnTheNextLine(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"empty destination on the next line", "[0]:\n<>\n0"},
		{"destination with a space on the next line", "[a]:\n<b c>\n0"},
		{"empty destination in a quote", "> [0]:\n> <>\n> 0"},
		// Control.
		{"empty destination on the same line", "[0]: <>\n0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			out := assertRebuildKeepsItsBlocks(t, tc.input)
			assert.Len(t, readBlockTexts(t, out), len(readBlockTexts(t, tc.input)), "the rebuild changed the block count")
		})
	}
	assert.Equal(t, "[0]: <>\n\n0\n", assertRebuildKeepsItsBlocks(t, "[0]:\n<>\n0"))
}

// TestDefinitionDestinationOpeningWithAMarkerStaysBare covers a definition in
// a quote whose destination is "><" on a lazy line four spaces in: the sniff
// for the angle form skipped the next line's container prefix and took the
// ">" of the destination for a quote marker, where a ">" after more than
// three columns of indent is content, so the "<" came first and the
// destination was recorded between angle brackets, where "<><>" is no
// destination at all. A destination of one space between angle brackets
// keeps them, since bare it is no destination.
func TestDefinitionDestinationOpeningWithAMarkerStaysBare(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"0\n>[0]:\n    ><": "0\n\n[0]: ><\n",
		"[0]:< >\n0":       "[0]: < >\n\n0\n",
	} {
		require.Equal(t, in, roundtripWithSkeleton(t, in), "skeleton path is not byte-exact")
		assert.Equal(t, want, assertRebuildKeepsItsBlocks(t, in), "input %q", in)
	}
}
