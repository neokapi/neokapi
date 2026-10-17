package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRebuildKeepsALiteralTildeApartFromAStrikethrough covers a literal tilde
// in the text beside a strikethrough. The rebuild spells the pair "~~", and a
// literal tilde touching it makes a run of three, which GFM pairs with
// nothing: "~0000~~ 0~" was rebuilt as "~~0000~~~ 0~", and the pass after
// read the pair as text and escaped it.
func TestRebuildKeepsALiteralTildeApartFromAStrikethrough(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"literal tilde after the closer", "~0000~~ 0~"},
		{"literal tilde after a short closer", "~a~~ b~"},
		{"literal tilde before the opener", "a~ ~~b~~"},
		{"literal tilde before a short opener", "a~~b~ c"},
		// Tildes the reader handed over as text pair with each other once the
		// HTML tag before them is dropped: "<A>~!0~~" was rebuilt as "~!0~~",
		// which the next pass read as a strikethrough.
		{"literal tildes after a dropped tag", "<A>~!0~~"},
		{"literal tildes after a dropped tag, equal runs", "<A>~~b~~"},
		// Controls.
		{"two pairs", "~a~ ~b~"},
		{"tildes inside the pair", "~~a~~~b~~"},
		{"tildes that pair with nothing", "x~a~~"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			assertRebuildKeepsItsBlocks(t, tc.input)
		})
	}
}

// TestRebuildKeepsALiteralTildeApartFromAStrikethroughSpelling pins the
// escape.
func TestRebuildKeepsALiteralTildeApartFromAStrikethroughSpelling(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "~~0000~~\\~ 0~\n", assertRebuildKeepsItsBlocks(t, "~0000~~ 0~"))
}
