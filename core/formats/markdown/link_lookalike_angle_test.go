package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRebuildKeepsALinkLookalikeAsText covers text shaped like a link whose
// destination is a bare "<", which is no destination, so the whole thing is
// text to the reader. The rebuild escapes a literal "<", and "\<" IS a
// destination: "[0](< 't')" came back as a link with a title, which the pass
// after re-spelled, and with a quote in the title the block's identity moved.
func TestRebuildKeepsALinkLookalikeAsText(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"quoted quote as the title", "[0](< '\"')"},
		{"quoted title", "[0](< 't')"},
		{"no title", "[0](<)"},
		{"space before the angle", "[0]( <)"},
		{"in a sentence", "See [the docs](< here) for more.\n"},
		// A bracket before the lookalike: escaping the inner bracket would
		// let the outer one close as a link, so the "<" before a space stays
		// bare and the whole stays text.
		{"nested brackets", "[[](< '\"')"},
		{"nested brackets, plain title", "[[](< 't')"},
		{"nested brackets, digit after the angle", "[[](<0 '\"')"},
		{"nested brackets, parenthesis after the angle", "[[](<(x) 't')"},
		// An angle before whitespace that ends the text is escaped, since the
		// block path trims that whitespace.
		{"angle before a trailing space in an HTML block", "<p>< "},
		{"angle before a trailing space", "a < "},
		{"angle before a trailing vertical tab", "<\v"},
		// Leading whitespace the block path trims put a declaration opener at
		// the line start, where it opens an HTML block.
		{"form feed before a declaration opener", "\f<!A0"},
		{"form feed before a tag opener", "\f<div>x"},
		{"angle before spaces and a trailing form feed", "a <  \f"},
		// Controls: real links, and brackets that open nothing.
		{"angle destination", "[0](<a>)"},
		{"plain destination", "[0](a 't')"},
		{"bracket span then text", "[0] (< 1)"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			assertRebuildKeepsItsBlocks(t, tc.input)
		})
	}
}

// TestRebuildKeepsALinkLookalikeAsTextSpelling pins the rebuild: the angle
// before a space opens nothing and stays bare, and with the angle bare the
// brackets complete nothing and stay bare too.
func TestRebuildKeepsALinkLookalikeAsTextSpelling(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "[0](< 't')\n", assertRebuildKeepsItsBlocks(t, "[0](< 't')"))
	assert.Equal(t, "[[](< 't')\n", assertRebuildKeepsItsBlocks(t, "[[](< 't')"))
}
