package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRebuildKeepsOneSpaceBeforeALineEndingInText covers text that ends a
// line with two spaces once the inline HTML between them is dropped: "0 <A> "
// and a line ending left "0  " and the line ending, which the pass after read
// as a hard break in place of the spaces, and stripped the tab that opened
// the next line with it. One space stays, which the parser keeps as text.
func TestRebuildKeepsOneSpaceBeforeALineEndingInText(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"two spaces around a dropped tag, tab after the line ending", "0 <A> \n\t#"},
		{"two spaces around a dropped tag", "a <A> \nb"},
		{"space and tab around a dropped tag", "a <A>\t\nb"},
		{"spaces around a dropped tag before a carriage return", "a <A> \r\nb"},
		// Controls.
		{"one space before the line ending", "a <A>\nb"},
		{"hard break of two spaces", "a  \nb"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			assertRebuildKeepsItsBlocks(t, tc.input)
		})
	}
	assert.Equal(t, "0 \n\t#\n", assertRebuildKeepsItsBlocks(t, "0 <A> \n\t#"))
	assert.Equal(t, "a \nb\n", assertRebuildKeepsItsBlocks(t, "a <A> \nb"))
}
