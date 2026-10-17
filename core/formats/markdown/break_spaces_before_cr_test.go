package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBreakSpacesBeforeACarriageReturn covers text ending in two spaces and a
// bare carriage return before a hard break. The break's own newline joins the
// carriage return into one line ending, and the two spaces before it are then
// a hard break of their own: "a  \r\\\n0" written with a bare newline came
// back as "a\n0", the spaces spent on the break. The break is spelled with a
// backslash, as it is after break spaces alone.
func TestBreakSpacesBeforeACarriageReturn(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"text, two spaces, carriage return, break", "a  \r\\\n0"},
		{"backslash, two spaces, carriage return, break", "\\  \r\\\n0"},
		// Controls.
		{"one space before the carriage return", "\\ \r\\\n0"},
		{"two spaces, break", "\\  \\\n0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			out := assertRebuildKeepsItsBlocks(t, tc.input)
			assert.Equal(t, readBlockTexts(t, tc.input), readBlockTexts(t, out), "the rebuild changed the blocks")
		})
	}
	assert.Equal(t, "a  \r\\\n0\n", assertRebuildKeepsItsBlocks(t, "a  \r\\\n0"))
}
