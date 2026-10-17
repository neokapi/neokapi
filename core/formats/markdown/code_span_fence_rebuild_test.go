package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCodeSpanFenceFitsItsContentOnRebuild is the #3193 reproducer. The
// source opens a code span with one backtick and closes it with the single
// backtick after the space, so the run of three inside is content (CommonMark
// 6.1: the closing run must be exactly as long as the opener). The rebuild
// path spelled every code span with one backtick on each side whatever the
// content held, so the rebuilt fence merged with the backticks beside it and
// the span re-read at different positions on every pass. The sink now sizes
// the fence to the longest backtick run in the content, the way fenced code
// blocks already do (#2487), and pads it with a space where the content begins
// or ends with a backtick or with spaces the re-read would strip.
func TestCodeSpanFenceFitsItsContentOnRebuild(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"three backticks inside a one-backtick span", "` 00000``` `000000````"},
		{"one backtick inside a two-backtick span", "`` a`b `` here"},
		{"content beginning with a backtick", "`` `a `` here"},
		{"content ending with a backtick", "`` a` `` here"},
		{"content that is a backtick run", "``` `` ``` here"},
		{"content padded by spaces on both sides", "`  a  ` here"},
		{"two spans with different fences", "`a` and `` b`c `` here"},
		{"span in emphasis", "*`` a`b ``* here"},
		{"span in a heading", "# `` a`b ``"},
		{"span in a list item", "- `` a`b ``"},
		// The controls: spans whose one-backtick spelling already fits.
		{"plain span", "`a` here"},
		{"span that is one space", "` ` here"},
		{"span with a space inside", "`a b` here"},
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

// TestCodeSpanFenceRebuildSpelling pins the fences the sink chooses: one more
// backtick than the longest run inside, and a padding space when the content
// begins or ends with a backtick, or begins and ends with a space.
func TestCodeSpanFenceRebuildSpelling(t *testing.T) {
	t.Parallel()
	tests := []struct{ in, want string }{
		{"` 00000``` `000000````", "```` 00000``` ````000000````\n"},
		{"`` a`b ``", "``a`b``\n"},
		{"`` `a ``", "`` `a ``\n"},
		{"``` `` ```", "``` `` ```\n"},
		{"`  a  `", "`  a  `\n"},
		{"`a`", "`a`\n"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, roundtrip(t, tc.in))
		})
	}
}
