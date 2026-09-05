package markdown_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// assertSkeletonByteExactTexts reads input, asserts its blocks carry exactly
// the given source texts, writes it back untranslated through the skeleton
// path and asserts the output is byte-identical and re-reads to the same
// texts. Unlike assertSkeletonByteExact it admits an input with no blocks.
func assertSkeletonByteExactTexts(t *testing.T, input string, texts []string) {
	t.Helper()
	blockTexts := func(input string) []string {
		got := testutil.BlockTexts(readBlocks(t, input))
		if len(got) == 0 {
			return nil
		}
		return got
	}
	assert.Equal(t, texts, blockTexts(input), "block texts")
	out := roundtripWithSkeleton(t, input)
	require.Equal(t, input, out, "skeleton path is not byte-exact")
	assert.Equal(t, texts, blockTexts(out), "block texts on re-read")
}

// TestAutoLinkSpellingRoundTrips is the #2429 reproducer. A paragraph that
// was only an autolink lost its angle brackets: the reader located an
// autolink from the text before it, and with nothing before it the
// bracketed form was taken for a bare URL. The autolink is now located from
// any neighbour, or from the start of its block, so the brackets ride the
// placeholder wherever the autolink sits. A block whose runs are placeholders
// alone has nothing to translate and emits no block.
func TestAutoLinkSpellingRoundTrips(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
		texts []string
	}{
		{"alone in a paragraph", "<https://example.com>\n", nil},
		{"alone without a final newline", "<https://example.com>", nil},
		{"alone in a heading", "# <https://example.com>\n", nil},
		{"alone in a list item", "- <https://example.com>\n", nil},
		{"alone in a list item with a sibling", "- <https://example.com>\n- b\n", []string{"b"}},
		{"alone in an item with a sublist", "- <https://example.com>\n  - b\n", []string{"b"}},
		{"alone in a blockquote", "> <https://example.com>\n", nil},
		{"alone in a table cell", "| a | b |\n| --- | --- |\n| <https://example.com> | c |\n", []string{"a", "b", "c"}},
		{"email alone", "<docs@example.com>\n", nil},
		{"two in a row", "<https://a.example> <https://b.example>\n", nil},
		{"adjacent", "<https://a.example><https://b.example>\n", nil},
		{"in a sentence", "See <https://example.com> now.\n", []string{"See  now."}},
		{"after a link", "[site](https://example.com) <https://example.com>\n", []string{"site "}},
		{"directly after a link", "[site](https://example.com)<https://example.com>\n", []string{"site"}},
		{"after a link with the same angle destination", "[site](<https://example.com>) <https://example.com>\n", []string{"site "}},
		{"after a code span", "`kapi` <https://example.com>\n", []string{"kapi "}},
		{"after inline html", "<b>x</b> <https://example.com>\n", []string{"x "}},
		{"first inside emphasis", "*<https://example.com> now*\n", []string{" now"}},
		{"first inside strong", "**<https://example.com> now**\n", []string{" now"}},
		{"after a task checkbox", "- [ ] <https://example.com>\n", []string{"[ ] "}},
		{"after a soft break", "a\n<https://example.com>\n", []string{"a\n"}},
		{"after a soft break in a blockquote", "> a\n> <https://example.com>\n", []string{"a\n> "}},
		{"after a hard break", "a  \n<https://example.com>\n", []string{"a\n"}},
		{"bare url is text", "https://example.com\n", []string{"https://example.com"}},
		{"bare host is text as spelled", "www.example.com\n", []string{"www.example.com"}},
		{"bare url in angle text", "<www.example.com>\n", []string{"<www.example.com>"}},
		{"bare then bracketed", "https://example.com <https://example.com>\n", []string{"https://example.com "}},
		{"bracketed then bare", "<https://example.com> https://example.com\n", []string{" https://example.com"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertSkeletonByteExactTexts(t, tc.input, tc.texts)
		})
	}
}

// TestLoneAutoLinkIsSkeleton pins the part stream for a paragraph that is one
// autolink: no block, the bytes in the skeleton, and the neighbours addressed
// as if the paragraph were there.
func TestLoneAutoLinkIsSkeleton(t *testing.T) {
	t.Parallel()
	parts := readParts(t, "First.\n\n<https://example.com>\n\nThird.\n")
	var blocks []*model.Block
	for _, p := range parts {
		if b, ok := p.Resource.(*model.Block); ok && p.Type == model.PartBlock {
			blocks = append(blocks, b)
		}
	}
	require.Len(t, blocks, 2)
	assert.Equal(t, "First.", blocks[0].SourceText())
	assert.Equal(t, "Third.", blocks[1].SourceText())
	assert.NotEqual(t, blocks[0].Name, blocks[1].Name)
	assert.Equal(t, "tu1", blocks[0].ID)
	assert.Equal(t, "tu2", blocks[1].ID, "the skipped paragraph reserves no block id")
}

// TestAutoLinkPlaceholderCarriesTheBrackets pins the run shape: the bracketed
// form is one placeholder whose data is the spelling, brackets included.
func TestAutoLinkPlaceholderCarriesTheBrackets(t *testing.T) {
	t.Parallel()
	blocks := readBlocks(t, "*<https://example.com> now*\n")
	require.Len(t, blocks, 1)
	var data []string
	for _, r := range blocks[0].Source {
		if r.Ph != nil && r.Ph.SubType == "md:autolink" {
			data = append(data, r.Ph.Data)
		}
	}
	assert.Equal(t, []string{"<https://example.com>"}, data)
}
