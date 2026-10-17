package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLiteralDelimitersBesideAStrayByte covers literal asterisks beside a
// byte that is no UTF-8 sequence of its own. The parser reads such a byte
// with the character before it, so before "*" in a digit, the byte 0xAA and
// "*" it sees the digit and the run closes; decoded alone the byte is the
// replacement character, a symbol, and the rebuild judged the run unable to
// pair and left it bare, so the pass after read an emphasis and escaped.
func TestLiteralDelimitersBesideAStrayByte(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"stray byte before a closer", "<A>\n***0\xaa*0"},
		{"stray byte after an opener", "<A>\n0*\xaa0*"},
		// Control: a valid non-ASCII letter in the same place.
		{"letter before a closer", "<A>\n***0\u00e9*0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			assertRebuildKeepsItsBlocks(t, tc.input)
		})
	}
	assert.Equal(t, "\\*\\*\\*0\xaa\\*0\n", assertRebuildKeepsItsBlocks(t, "<A>\n***0\xaa*0"))
}
