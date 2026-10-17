package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRebuildKeepsNestedEmphasisPairs covers emphasis nested in emphasis of
// the same character, where the rebuild path's spelling has to read back as
// the pairs the block carries. Each input is byte-exact on the skeleton path
// and a fixed point of the rebuild path.
func TestRebuildKeepsNestedEmphasisPairs(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		// Three pairs opened by one run of five asterisks: the second opener
		// was re-spelled with underscores, which left the first unable to open
		// after an alphanumeric (#3221).
		{"three pairs in one run after a word", "0*****0*****"},
		{"three pairs in one run", "*****0*****"},
		{"two pairs in one run after a word", "0***0***"},
		{"strong in strong in one run", "0****0****"},
		// An inner pair between punctuation: its opener could close as well,
		// so spelled with the outer pair's character it closed the outer pair
		// first (#3229).
		{"inner pair between exclamation marks", "*!_!0_!*"},
		{"inner strong between exclamation marks", "**!__!0__!**"},
		{"inner pair between periods", "*a._b_.c*"},
		// The inner pair's opener touched the outer's and the two formed a run
		// of two, which the rule of three keeps from pairing with a closer of
		// one between letters.
		{"inner pair at the opener inside a word", "_*0*0_"},
		{"inner strong at the opener inside a word", "_**0**0_"},
		// An outer pair whose first child is an empty link: its inner pair's
		// opener sits between letters and would close the outer pair spelled
		// with the same character.
		{"empty link then an inner pair inside a word", "_[]()00*0*!_"},
		// Controls.
		{"inner pair after a space", "*a *b* c*"},
		{"inner pair of the other character", "*a _b_ c*"},
		{"strong around emphasis", "**_a_**"},
		{"emphasis around strong", "_**a**_"},
		{"a sentence", "An *emphasis holding *another* and _more_* here.\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			out := assertRebuildKeepsItsBlocks(t, tc.input)
			assert.Equal(t, readBlockTexts(t, tc.input), readBlockTexts(t, out),
				"the rebuild changed the block's text")
		})
	}
}

// TestRebuildSpellsNestedEmphasisAsRead pins the spellings the rebuild path
// chooses: the pairs' own characters where they read back, the other
// character where they would pair differently.
func TestRebuildSpellsNestedEmphasisAsRead(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"one run stays one run", "0*****0*****", "0*****0*****\n"},
		{"inner pair keeps the other character", "*!_!0_!*", "*!_!0_!*\n"},
		{"outer pair takes the other character", "_*0*0_", "_*0*0_\n"},
		{"touching closers re-spell the inner pair", "_!*!0*_", "*!_!0_*\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, assertRebuildKeepsItsBlocks(t, tc.input))
		})
	}
}

// TestRespelledEmphasisWeighsLiteralsAgain covers literal underscores beside
// nested pairs the rebuild re-spells. Weighed against the first spelling the
// literals could pair with nothing; against the re-spelled runs they could,
// and the pass after escaped them. The literals are weighed again against the
// spelling chosen.
func TestRespelledEmphasisWeighsLiteralsAgain(t *testing.T) {
	t.Parallel()
	for _, in := range []string{">____!\\\n>_!_ ", ">____!\\\n>_!_", ">!___!\\\n>_!_", ">___!\\\n>_ 00_"} {
		require.Equal(t, in, roundtripWithSkeleton(t, in), "skeleton path is not byte-exact")
		assertRebuildKeepsItsBlocks(t, in)
	}
}
