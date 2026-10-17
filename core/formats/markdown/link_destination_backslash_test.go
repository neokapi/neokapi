package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRebuildSpellsABackslashDestination covers a link whose destination is a
// backslash. Written bare, "\)" escapes the parenthesis and the link never
// closes; between angle brackets, "\>" escapes the bracket. Neither form read
// back, so the rebuild wrote the value bare and "0[](\ )" came back as text,
// which the pass after escaped. A space before the parenthesis ends the
// destination where the value does.
func TestRebuildSpellsABackslashDestination(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"empty text, backslash destination", "0[](\\ )"},
		{"text, backslash destination", "0[a](\\ )"},
		{"destination ending in a backslash", "[a](x\\ )"},
		{"destination that is an angle bracket", "[a](<\\<>)"},
		// Controls.
		{"two backslashes", "a [x](\\\\ )"},
		{"backslash in angle brackets", "0[](<\\ >)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			out := assertRebuildKeepsItsBlocks(t, tc.input)
			assert.Equal(t, readBlockTexts(t, tc.input), readBlockTexts(t, out), "the rebuild changed the block's text")
		})
	}
}

// TestRebuildSpellsABackslashDestinationSpelling pins the spelling: bare,
// with a space before the parenthesis.
func TestRebuildSpellsABackslashDestinationSpelling(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "0[](\\ )\n", assertRebuildKeepsItsBlocks(t, "0[](\\ )"))
}
