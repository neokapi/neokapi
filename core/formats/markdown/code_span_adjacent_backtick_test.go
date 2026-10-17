package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRebuildKeepsALiteralBacktickApartFromACodeSpan covers a literal backtick
// in the text around a code span. CommonMark 6.1 opens a span on any backtick
// run that a run of the same length follows, so a one-backtick fence after a
// literal backtick paired with that literal instead: a literal backtick, "0"
// and a two-backtick span holding "<" were rebuilt as a one-backtick span
// holding "0" and stray text. The fence avoids the lengths of the literal runs
// before it, and a literal run touching a fence is escaped.
func TestRebuildKeepsALiteralBacktickApartFromACodeSpan(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"literal backtick before the span", "`0``<``"},
		{"literal backtick, space, span", "0` ``<``"},
		{"span then a literal backtick", "``<`` `0"},
		{"two spans with different fences", "`a` and ``b`c``"},
		// The fences of a span already written are markup, not literal runs
		// the next span's fence has to avoid.
		{"two spans after a literal run", "*``0`0`\n`*\n`0*0"},
		{"span after a span", "`a` `b`"},
		// Controls: an escaped backtick, and runs that are all text.
		{"escaped backtick before a span", "x\\` `y`"},
		{"backtick runs that pair with nothing", "0```x``"},
		{"run inside the span", "`a``b`"},
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

// TestRebuildKeepsALiteralBacktickApartFromACodeSpanSpelling pins the fences.
func TestRebuildKeepsALiteralBacktickApartFromACodeSpanSpelling(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "`0``<``\n", assertRebuildKeepsItsBlocks(t, "`0``<``"))
	assert.Equal(t, "`<` `0\n", assertRebuildKeepsItsBlocks(t, "``<`` `0"))
}

// TestRebuildEscapesEveryBacktickOfATouchingRun covers a literal run of two
// backticks that the dropped inline HTML after it leaves touching a code
// span's one-backtick fence. A backslash escapes one character, so each
// backtick of the run takes its own; escaped as one, the rest of the run merged
// with the fence into a run of three, and the span was gone on the pass after.
func TestRebuildEscapesEveryBacktickOfATouchingRun(t *testing.T) {
	t.Parallel()
	in := "``<A>`<A>`0`"
	require.Equal(t, in, roundtripWithSkeleton(t, in), "skeleton path is not byte-exact")
	assert.Equal(t, "\\`\\``<A>`0`\n", assertRebuildKeepsItsBlocks(t, in))
}
