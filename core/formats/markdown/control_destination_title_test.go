package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBareDestinationHoldingAControlCharacter covers a link whose bare
// destination holds a control character and which carries a title. The
// parser ends a bare destination at a space, a tab or a line ending and keeps
// any other byte, so a bare "\x18" followed by a space and two single quotes
// is a link destination with an empty title after it.
// The reader's scan of the closer ended the destination at the control
// character, found no closer and rebuilt one without the title, and the
// rebuild path then changed the brackets around the link paired on the pass
// after.
func TestBareDestinationHoldingAControlCharacter(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"control character, empty title", "[a](\x18 '')"},
		{"control character inside, title", "[a](x\x18y 't')"},
		{"nested brackets around a control character destination", "[[][](0 '](\x18 '')')"},
		{"vertical tab in the destination", "[a](x\vy)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			out := assertRebuildKeepsItsBlocks(t, tc.input)
			assert.Equal(t, readBlockTexts(t, tc.input), readBlockTexts(t, out), "the rebuild changed the blocks")
		})
	}
	assert.Equal(t, "[a](\x18 '')\n", assertRebuildKeepsItsBlocks(t, "[a](\x18 '')"))
}

// TestControlDestinationEndingInABackslashStaysBare covers a definition whose
// destination holds a control byte and ends in a backslash, with a title after
// two spaces. Between angle brackets the backslash escaped the closing one,
// the brackets never closed, and the definition read back as a paragraph; it
// stays bare, with the two spaces the title needs after a backslash.
func TestControlDestinationEndingInABackslashStaysBare(t *testing.T) {
	t.Parallel()
	in := "0\n\n[0]:\\\x1c\\  '0'"
	require.Equal(t, in, roundtripWithSkeleton(t, in), "skeleton path is not byte-exact")
	assert.Equal(t, "0\n\n[0]: \\\x1c\\  \"0\"\n", assertRebuildKeepsItsBlocks(t, in))
}
