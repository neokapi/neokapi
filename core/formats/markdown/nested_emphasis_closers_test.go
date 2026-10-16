package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNestedEmphasisClosersThatTouch is the #3106 reproducer. "_!*!0*_" is
// one emphasis holding another, and the inner one closes where the outer one
// does. The rebuild path spelled both with asterisks, so the two closers formed
// a run of two, and CommonMark 6.2's rule of three kept it from pairing with
// the inner opener: the structure moved on the pass after and the text with it.
// The sink now holds an emphasis closer that would touch the one before it
// until the next byte is known, and re-spells the inner pair, or failing that
// the outer one, with underscores where those can pair.
func TestNestedEmphasisClosersThatTouch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		// escapes marks a shape with a literal delimiter the rebuild escapes,
		// which the reader keeps as text: the first pass re-represents the
		// block (#2527), and only the passes after it are held to its text.
		escapes bool
	}{
		{name: "inner opener between punctuation", input: "_!*!0*_"},
		{name: "inner opener inside a word", input: "_0*0*_"},
		{name: "literal underscore before the pair", input: "__!_0__", escapes: true},
		{name: "strong inside strong", input: "__a**b**__"},
		{name: "emphasis inside strong", input: "__a*b*__"},
		{name: "strong inside emphasis", input: "_a**b**_"},
		{name: "text after the pair", input: "_!*!0*_ then more"},
		{name: "three levels", input: "_a*b*_ and *c*"},
		{name: "in a sentence", input: "An _emphasis holding *another*_ here.\n"},
		// The controls: pairs that do not touch, and the touching openers
		// #2499 already spells apart.
		{name: "inner closes before the outer", input: "_*a* b_"},
		{name: "adjacent pairs", input: "_0__*0*"},
		{name: "nested at the opener", input: "_*a*_"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			out := assertRebuildKeepsItsBlocks(t, tc.input)
			if tc.escapes {
				return
			}
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

// TestNestedEmphasisClosersSpelling pins the shapes the sink chooses: the inner
// pair takes underscores where they can pair, and the outer one does when the
// inner sits inside a word, where an underscore cannot open.
func TestNestedEmphasisClosersSpelling(t *testing.T) {
	t.Parallel()
	tests := []struct{ in, want string }{
		{"_!*!0*_", "*!_!0_*\n"},
		{"_0*0*_", "_0*0*_\n"},
		{"__a**b**__", "__a**b**__\n"},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, roundtrip(t, tc.in))
		})
	}
}
