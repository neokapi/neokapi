package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRebuildEscapesABackslashBeforeABreak covers a literal backslash at the
// end of a line whose hard break is spelled by spaces. The rebuild path writes
// the break as a bare newline, and a backslash before a line ending is a hard
// break of its own (CommonMark 6.7): "#\  \n0" came back with the backslash
// read as the break and the block's text one character shorter.
func TestRebuildEscapesABackslashBeforeABreak(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"hash and backslash before a spaces break", "#\\  \n0"},
		{"word and backslash before a spaces break", "a\\  \nb"},
		{"backslash before dropped inline HTML and a line ending", "a\\<A>\nb"},
		{"backslash in a quote before a spaces break", "> a\\  \n> b"},
		// A bare carriage return between the backslash and the break's line
		// ending still leaves the backslash before a line ending.
		{"backslash, carriage return, backslash break", "a\\\r\\\n0"},
		{"hash, backslash, carriage return, CRLF backslash break", "#\\\r\\\r\n0"},
		// An HTML block's text is one run holding the line ending, and a
		// backslash before it inside the run is a literal one too.
		{"backslash before a line ending in an HTML block", "<div>#\\\n0"},
		{"letter and backslash before a line ending in an HTML block", "<div>a\\\nb"},
		// Controls: an escaped backslash, and the backslash form of the break.
		{"escaped backslash before a soft break", "a\\\\\nb"},
		{"backslash break", "a\\\nb"},
		{"spaces then a backslash break", "a  \\\nb"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			assertRebuildKeepsItsBlocks(t, tc.input)
		})
	}
}

// TestRebuildEscapesABackslashBeforeABreakSpelling pins the rebuild: the
// literal backslash is escaped, and a break the sink spells with a backslash
// is not.
func TestRebuildEscapesABackslashBeforeABreakSpelling(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "#\\\\\n0\n", assertRebuildKeepsItsBlocks(t, "#\\  \n0"))
	assert.Equal(t, "a  \\\nb\n", assertRebuildKeepsItsBlocks(t, "a  \\\nb"))
}
