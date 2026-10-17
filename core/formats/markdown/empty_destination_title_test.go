package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRebuildSpellsAnEmptyDestinationBeforeATitle covers a link whose
// destination is empty and which carries a title. The rebuild wrote the
// destination as nothing, and a title with no destination before it is read
// as the destination, so "[](<>'0')" came back as "[]( \"0\")", which is no
// link at all, and the title's block was gone.
func TestRebuildSpellsAnEmptyDestinationBeforeATitle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"empty text, empty destination, title", "[](<>'0')"},
		{"text, empty destination, title", "[a](<>'0')"},
		{"double-quoted title", "[a](<> \"t\")"},
		// Controls: an empty destination with no title.
		{"empty destination", "[a](<>)"},
		{"no destination", "[a]()"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			out := assertRebuildKeepsItsBlocks(t, tc.input)
			assert.Equal(t, readBlockTexts(t, tc.input), readBlockTexts(t, out), "the rebuild changed the blocks")
		})
	}
}

// TestRebuildSpellsAnEmptyDestinationBeforeATitleSpelling pins the spelling.
func TestRebuildSpellsAnEmptyDestinationBeforeATitleSpelling(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "[](<>'0')\n", assertRebuildKeepsItsBlocks(t, "[](<>'0')"))
}
