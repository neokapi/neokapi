package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRebuildEscapesAnyMarkerOnALazyLine is the #3224 reproducer. The parser
// ends a blockquote's or a list item's paragraph at any block marker on a lazy
// continuation line, a bullet with nothing after it included, where CommonMark
// 5.2 would let a paragraph run on. The rebuild spells a hard break as a soft
// one, so ">0\n*\\\n0" became ">0\n*\n0" and the bare bullet split the block.
func TestRebuildEscapesAnyMarkerOnALazyLine(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"bullet in a quote", ">0\n*\\\n0"},
		{"dash in a quote", ">0\n-\\\n0"},
		{"plus in a quote", ">0\n+\\\n0"},
		{"ordered marker in a quote", ">0\n2.\\\n0"},
		{"bullet with a space in a quote", ">0\n* \\\n0"},
		{"bullet in a list item", "* 0\n*\\\n0"},
		{"ordered marker in a list item", "- 0\n1.\\\n0"},
		{"bullet on an indented item line", "- a\n  *\\\n  b"},
		// A line with one marker inside a quote two deep is lazy for the
		// inner level, and the emphasis closer re-spelled as "*" there is a
		// bullet.
		{"emphasis closer on a half-marked line of a nested quote", ">>_0\n>_\n0"},
		// Closing runs that are a line of their own keep the hard break's
		// backslash, without which the line is a thematic break.
		{"closing runs on a line of their own before a break", ">***0\n>***\\\n000"},
		{"closing runs with the outer closer held before a break", ">0*0**0\n>***\\\n0"},
		{"opening runs on a line of their own before a break", ">***\\\n>***\\\n0"},
		// A literal run beside the opener is escaped, after which the line
		// is no thematic break and the break stays soft.
		{"literal run and an opener before a break", "***\\\n0*"},
		{"literal asterisk and a strong opener before a break", "***\\\n0**"},
		// A lazy line indented four spaces under a wide marker sits inside the
		// rebuilt item, two spaces past its content, where it is a marker.
		{"heading on a lazy line indented past the rebuilt marker", "*    0\n    # 0"},
		{"bullet on a lazy line indented past the rebuilt marker", "*    0\n    - 0"},
		{"heading on a lazy line under an ordered marker", "1.    0\n    # 0"},
		// A tab is four columns of indentation, inside the rebuilt item.
		{"quote marker on a tab-indented lazy line", ">* 0\n\t>0"},
		// A bullet item whose text is two dashes spells "- --", a thematic
		// break, so the first dash is escaped.
		{"item whose text is two dashes", "0) --"},
		// A lazy line of dashes under an item's text is a delimiter row once
		// the rebuild's trailing newline follows it.
		{"item whose lazy line is two dashes", "* 0\n0\n--"},
		{"item whose second line is two dashes", "- 0\n--"},
		// Controls: a marked continuation line keeps CommonMark's rule, and a
		// heading marker was already escaped (#2485).
		{"bullet on a marked line", "> 0\n> *\n> 0"},
		{"heading marker on a lazy line", ">0\n#\\\n0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			out := assertRebuildKeepsItsBlocks(t, tc.input)
			assert.Len(t, readBlockTexts(t, out), len(readBlockTexts(t, tc.input)), "the rebuild split the block")
		})
	}
}

// TestRebuildEscapesAnAdmonitionMarker covers a block whose text begins with
// an admonition marker the reader recognises: the title ":::" of a Docusaurus
// admonition was written as a paragraph and read back as the admonition's
// closer, and the block was gone; a MkDocs title "!!! x" read back as an
// opener.
func TestRebuildEscapesAnAdmonitionMarker(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"Docusaurus title that is a marker", "::: :::"},
		{"Docusaurus title that is a marker, with a type", ":::note :::"},
		{"MkDocs title that is an opener", "!!! note \"!!! x\"\n    body\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			out := assertRebuildKeepsItsBlocks(t, tc.input)
			assert.Len(t, readBlockTexts(t, out), len(readBlockTexts(t, tc.input)), "the rebuild changed the block count")
		})
	}
}

// TestRebuildEscapesAnyMarkerOnALazyLineSpelling pins the escapes.
func TestRebuildEscapesAnyMarkerOnALazyLineSpelling(t *testing.T) {
	t.Parallel()
	assert.Equal(t, ">0\n\\*\n0\n", assertRebuildKeepsItsBlocks(t, ">0\n*\\\n0"))
	assert.Equal(t, "- \\--\n", assertRebuildKeepsItsBlocks(t, "0) --"))
	assert.Equal(t, "> 0\n> *\n> 0\n", assertRebuildKeepsItsBlocks(t, "> 0\n> *\n> 0"))
}
