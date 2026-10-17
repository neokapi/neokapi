package markdown_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLiteralDelimiterMergingIntoMarkup is the #3191 reproducer. "_* `00*0_"
// is one emphasis whose text is "* `00*0": the "*" after the opening "_"
// cannot open, because a space follows it, and the one between the zeros has
// no partner, so both are literal. The rebuild path spells the emphasis with
// "*", and the literal "*" that begins the content lands right after that
// opener. The two form a run of two followed by a space, which CommonMark 6.2
// lets neither open nor close, so the emphasis never opened on re-read and the
// inner "*0*" paired instead. output() escaped a literal delimiter only when
// its own run could pair, and judged on the merged run it could not. A literal
// delimiter whose run also holds markup is now escaped whatever the merged
// run's flanking, so the markup stands on its own.
func TestLiteralDelimiterMergingIntoMarkup(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"literal then a space after the opener", "_* `00*0_"},
		{"literal then a space, short", "_* a_"},
		{"literal then a space, then a pair inside", "_* a *b*_"},
		{"literal after the opener", "_*a_"},
		{"literal after the opener in a sentence", "a _*b_ c"},
		{"literal after a strong opener", "__*a__"},
		{"two literals after the opener", "_**a_"},
		{"literal before the closer", "_a*_"},
		{"literal before a strong closer", "__a*__"},
		{"literal after the closer", "*a**"},
		{"literal before the opener", "a*_b_"},
		{"underscore literal after an asterisk opener", "*_a*"},
		// The pairing question is asked once the merged literals are escaped,
		// so the pass after, which reads those escapes, answers the same.
		{"literal before and after a strong pair, then more", "__*__*_*00"},
		{"literal before and after an emphasis pair", "*_*_*00"},
		// The controls: literal delimiters beside no markup.
		{"literal between words", "2 * 3 * 4"},
		{"intraword underscores", "foo_bar_baz"},
		{"plain pair", "*a* b"},
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
				assert.Equal(t, inlineCodeTypes(before[i]), inlineCodeTypes(after[i]),
					"the rebuild changed the block's inline structure: %q", out)
				assert.Equal(t, unescapeDelimiters(before[i].SourceText()), unescapeDelimiters(after[i].SourceText()),
					"the rebuild changed the block's text beyond escaping a delimiter: %q", out)
			}
		})
	}
}

// inlineCodeTypes lists the paired codes a block's source runs open, in order.
func inlineCodeTypes(b *model.Block) []string {
	var types []string
	for _, r := range b.SourceRuns() {
		if r.PcOpen != nil {
			types = append(types, r.PcOpen.Type)
		}
	}
	return types
}

// unescapeDelimiters drops the backslash before an escaped "*" or "_", which
// is the one change the first rebuild pass may make to a block's text.
func unescapeDelimiters(s string) string {
	var out []byte
	for i := range len(s) {
		if s[i] == '\\' && i+1 < len(s) && (s[i+1] == '*' || s[i+1] == '_') {
			continue
		}
		out = append(out, s[i])
	}
	return string(out)
}

// TestLiteralDelimiterMergeSpelling pins the shapes: the literal beside the
// markup is escaped, and so is any other literal the escape lets pair.
func TestLiteralDelimiterMergeSpelling(t *testing.T) {
	t.Parallel()
	tests := []struct{ in, want string }{
		{"_* `00*0_", "*\\* `00\\*0*\n"},
		{"_* a_", "*\\* a*\n"},
		{"_*a_", "*\\*a*\n"},
		{"_a*_", "*a\\**\n"},
		{"*a**", "*a*\\*\n"},
		{"2 * 3 * 4", "2 * 3 * 4\n"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, roundtrip(t, tc.in))
		})
	}
}
