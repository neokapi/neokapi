package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRebuildEscapesAHeadingClosingSequenceAfterACarriageReturn covers an ATX
// heading whose text ends in a bare carriage return and a hash. The parser
// keeps the "\r" inside the line as whitespace, so the hash after it is a
// closing sequence (CommonMark 4.2); the escape tested for a space or a tab
// alone and "# a\r# #" came back as "# a\r#", which re-read as "a".
func TestRebuildEscapesAHeadingClosingSequenceAfterACarriageReturn(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"hash, carriage return, hash, closing sequence", "# #\r# #"},
		{"text, carriage return, hash, closing sequence", "# a\r# #"},
		{"text, carriage return, hash run", "# a\r##"},
		// Controls.
		{"hash after a space", "# a #\r#"},
		{"carriage return inside the text", "# a\rb #"},
		{"hash then text", "# #\r# b"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			out := assertRebuildKeepsItsBlocks(t, tc.input)
			assert.Len(t, readBlockTexts(t, out), 1)
		})
	}
}

// TestRebuildEscapesAHeadingClosingSequenceAfterACarriageReturnSpelling pins
// the escape.
func TestRebuildEscapesAHeadingClosingSequenceAfterACarriageReturnSpelling(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "# a\r\\#\n", assertRebuildKeepsItsBlocks(t, "# a\r# #"))
}
