package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCodeSpanPaddingAroundControlWhitespace covers a code span whose content
// is spaces around a vertical tab or a form feed. The parser strips one space
// from each end of a span that begins and ends with a space unless the content
// is spaces alone, and a vertical tab is not a space to it, so the rebuild
// has to pad such content as it pads any other; it judged the content all
// whitespace and wrote it bare, and each pass lost a space from each end.
func TestCodeSpanPaddingAroundControlWhitespace(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"spaces around a vertical tab", "0`   \v   `0"},
		{"spaces around a form feed", "0`   \f   `0"},
		{"one space around a vertical tab", "0` \v `0"},
		// Controls: a carriage return is a line ending the parser turns into a
		// space, so spaces around one are spaces alone.
		{"spaces around a carriage return", "0` \r `0"},
		{"spaces around a tab", "0` \t `0"},
		{"spaces around a letter", "0`  a  `0"},
		{"spaces alone", "0`   `0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			out := assertRebuildKeepsItsBlocks(t, tc.input)
			assert.Equal(t, readBlockTexts(t, tc.input), readBlockTexts(t, out), "the rebuild changed the blocks")
		})
	}
	assert.Equal(t, "0`   \v   `0\n", assertRebuildKeepsItsBlocks(t, "0`   \v   `0"))
}
