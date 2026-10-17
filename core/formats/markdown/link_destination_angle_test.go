package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLinkDestinationThatNeedsAngleBrackets is the #3198 reproducer. goldmark
// resolves "[](( )" as a link whose destination is "(", the way #2514
// describes for "[x](( )". The rebuild path has only the resolved destination
// and spelled it bare, as "[](()", which re-reads as a link to "((" or, once
// the bracket is escaped on the pass after, as no link at all. CommonMark 6.3
// gives a destination that cannot stand bare a second spelling, between angle
// brackets, and the rebuild path now asks spellLinkDestination, the edit
// path's answer to the same question, which form reads back as the value.
func TestLinkDestinationThatNeedsAngleBrackets(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"unbalanced opening parenthesis", "0[](( )"},
		{"unbalanced opening parenthesis in a sentence", "A link [x](( ) inside a sentence.\n"},
		{"destination spelled with angle brackets", "[a](<(>)"},
		{"space in the destination", "[a](<b c>) here"},
		{"space in an image destination", "![i](<s p>) here"},
		{"tab in the destination", "[a](<b\tc>) here"},
		{"carriage return in the destination", "[a](<b\rc>) here"},
		{"carriage return in a titled destination", "[](<0\r0>'0')"},
		{"destination opening with an angle bracket", "[a](<\\<b>) here"},
		{"escaped angle bracket inside the destination", "[a](<b\\>c>) here"},
		{"titled link with a space in the destination", "[a](<b c> 't') here"},
		{"in emphasis", "*[a](<b c>)* here"},
		// The controls: destinations that stand bare.
		{"plain destination", "[a](b) here"},
		{"balanced parentheses", "[a](b(c)d) here"},
		{"escaped parenthesis", "[a](b\\)c) here"},
		{"empty destination", "[a]() here"},
		{"empty angle destination", "[a](<>) here"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			out := assertRebuildKeepsItsBlocks(t, tc.input)
			before := readBlocks(t, tc.input)
			after := readBlocks(t, out)
			require.Len(t, after, len(before))
			for i := range before {
				assert.Equal(t, before[i].SourceText(), after[i].SourceText(),
					"the rebuild changed the block's text: %q", out)
			}
		})
	}
}

// TestLinkDestinationAngleSpelling pins the spelling: the closer the source
// spelled, where it still reads as the same destination, and otherwise angle
// brackets exactly where the destination cannot stand bare.
func TestLinkDestinationAngleSpelling(t *testing.T) {
	t.Parallel()
	tests := []struct{ in, want string }{
		{"0[](( )", "0[](( )\n"},
		{"[a](<b c>)", "[a](<b c>)\n"},
		{"[a](<b\\>c>)", "[a](<b\\>c>)\n"},
		{"[a](<\\<b>)", "[a](<\\<b>)\n"},
		{"[a](<b c> 't')", "[a](<b c> 't')\n"},
		{"[a](<b>)", "[a](<b>)\n"},
		{"[a](b(c)d)", "[a](b(c)d)\n"},
		{"[a](<>)", "[a](<>)\n"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, roundtrip(t, tc.in))
		})
	}
}
