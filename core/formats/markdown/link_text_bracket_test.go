package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRebuildLeavesABracketInsideLinkTextAlone covers a link whose text
// begins with "[](", the shape the rebuild escapes in plain text so it opens
// no link with an empty text. Inside a link's text the escape changes which
// "]" the parser pairs the link's own "[" with, because the opener is dropped
// after the first "](" that forms no link: "[[](0 ](( )" was rebuilt as
// "[\[](0 ](<(>)", which read as text and was escaped again on the pass after.
func TestRebuildLeavesABracketInsideLinkTextAlone(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"empty link text inside link text, unbalanced destination", "[[](0 ](( )"},
		{"empty link text inside link text", "[[](0 ](x)"},
		{"angle destination", "[[](0 ](<(>)"},
		// Control.
		{"empty brackets inside link text", "[[]](x)"},
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

// TestRebuildLeavesABracketInsideLinkTextAloneSpelling pins the rebuild.
func TestRebuildLeavesABracketInsideLinkTextAloneSpelling(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "[[](0 ](( )\n", assertRebuildKeepsItsBlocks(t, "[[](0 ](( )"))
}

// TestRebuildEscapesABracketCompletedAcrossADroppedTag covers a bracket whose
// completion into a link arrives in a later text run: the inline HTML between
// "[" and "](" in "[<A>](" is dropped, so the first pass writes "[](", which
// is text on its own and stays as written. A hard break spelled as a soft one
// completes a link: "[ ](0 \\\n)" is text because of the backslash, and
// written with a bare line ending it was a link whose text is a space, which
// is no block at all. The bracket is judged on the output as a whole, not on
// the run it came in.
func TestRebuildEscapesABracketCompletedAcrossADroppedTag(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"empty text before a destination opener", "[<A>]("},
		{"empty text before a label", "[<A>][x]"},
		{"text before an angle destination", "[a<A>](<b"},
		{"break before the closing parenthesis", "[ ](0 \\\n)"},
		{"break before the closing parenthesis, in a sentence", "x [a](0 \\\n)"},
		// Controls.
		{"brackets that complete nothing", "a [<A>] b"},
		{"empty text in one run", "[]("},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			assertRebuildKeepsItsBlocks(t, tc.input)
		})
	}
	assert.Equal(t, "[](\n", assertRebuildKeepsItsBlocks(t, "[<A>]("))
}

// TestRebuildKeepsTheBracketsAfterAReference covers "[](" right after another
// bracket, which itself follows a shortcut reference: "[R][[](" is the
// reference "[R]" and text. Escaped on its opening bracket the text read as
// "[\[]", a link label, and a shortcut reference followed by a label is no
// reference, so the definition's label block was gone. The brackets complete
// no link and are left as written.
func TestRebuildKeepsTheBracketsAfterAReference(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"[R][[](\n\n[R]:0", "[R][0[](\n\n[R]:0", "[R][0 [](\n\n[R]:0", "[R][\n[](\n\n[R]:0"} {
		require.Equal(t, in, roundtripWithSkeleton(t, in), "skeleton path is not byte-exact")
		assertRebuildKeepsItsBlocks(t, in)
	}
	assert.Equal(t, "[R][[](\n\n[R]: 0\n", assertRebuildKeepsItsBlocks(t, "[R][[](\n\n[R]:0"))
	assert.Equal(t, "[R][0[](\n\n[R]: 0\n", assertRebuildKeepsItsBlocks(t, "[R][0[](\n\n[R]:0"))
}

// TestRebuildKeepsEmptyBracketsBeforeAReference covers "[[](]]" and
// "[[][]]]" before a shortcut reference "[R]" with a definition of R: the
// brackets are text, and the inner "]" escaped on the shape "[](" or "[]["
// alone made the span from the outer "[" the text of a full reference link
// "[...][R]", so the pass after lost the definition's label block.
func TestRebuildKeepsEmptyBracketsBeforeAReference(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"[[](]][R]\n\n[R]:0":  "[[](]][R]\n\n[R]: 0\n",
		"[[][]]][R]\n\n[R]:0": "[[][]]][R]\n\n[R]: 0\n",
		// The "<" after "[](" completes a link only once the angle escape
		// takes it, and an angle before "]" is left bare.
		"[[](<]][R]\n\n[R]:0": "[[](<]][R]\n\n[R]: 0\n",
	} {
		require.Equal(t, in, roundtripWithSkeleton(t, in), "skeleton path is not byte-exact")
		assert.Equal(t, want, assertRebuildKeepsItsBlocks(t, in))
	}
}
