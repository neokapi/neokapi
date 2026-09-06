package markdown_test

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRebuildEscapesMarkerOnEveryLine is the #2470 reproducer. An HTML block
// runs to the blank line (CommonMark 4.6), so "<div>0\n# 0" is one block whose
// text is "0\n# 0". The rebuild path has no spelling for the HTML, drops it and
// writes the text as a paragraph, where the second line read back as an ATX
// heading and one block became two. Every line of a rebuilt block starts in
// block-content position, so the leading-marker escape runs on each of them.
func TestRebuildEscapesMarkerOnEveryLine(t *testing.T) {
	t.Parallel()
	texts := []string{
		"0\n# 0",
		"0\n## deeper",
		"0\n> quoted",
		"0\n- bullet",
		"0\n+ bullet",
		"0\n* bullet",
		"0\n1. ordered",
		"0\n1) ordered",
		"0\n``` fence",
		"0\n~~~ fence",
		"0\n---",
		"0\n***",
		"0\na\n# heading on the third line",
	}
	for _, txt := range texts {
		t.Run(txt, func(t *testing.T) {
			t.Parallel()
			block := model.NewBlock("p", txt)
			out := rebuildBlocks(t, block)

			blocks := readBlocks(t, out)
			require.Len(t, blocks, 1, "paragraph %q re-read as %d blocks via %q", txt, len(blocks), out)
			assert.Empty(t, blocks[0].SemanticRole(),
				"paragraph %q re-read as role %q via %q", txt, blocks[0].SemanticRole(), out)
			assert.Equal(t, out, rebuildBlocks(t, blocks[0]), "rebuild is not idempotent for %q", txt)
		})
	}
}

// TestContinuationMarkerThatCannotInterruptIsLeftAlone pins the narrower rule a
// continuation line takes: CommonMark 5.2 lets a list interrupt a paragraph
// only when the item carries content and, for an ordered list, only when it
// starts at 1. Escaping a marker that cannot interrupt changes the document:
// "[R]:\n0)" is a paragraph, because an unmatched ")" is not a link
// destination, and "0\)" is one, so the escape turned the paragraph into a link
// reference definition and the block was gone.
func TestContinuationMarkerThatCannotInterruptIsLeftAlone(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		text string
		want string
	}{
		{"ordered list not starting at one", "[R]:\n0)", "[R]:\n0)\n"},
		{"ordered list starting at two", "a\n2. b", "a\n2. b\n"},
		{"ordered list starting at one", "a\n1. b", "a\n1\\. b\n"},
		{"ordered list with leading zeros", "a\n01. b", "a\n01\\. b\n"},
		// A lone "-" is a setext underline, which escapeInteriorBlockBars
		// escapes for its own reason (#1651); the marker rule leaves it alone.
		{"empty bullet", "a\n-", "a\n\\-\n"},
		{"empty bullet, plus sign", "a\n+", "a\n+\n"},
		{"bullet with content", "a\n- b", "a\n\\- b\n"},
		{"empty heading", "a\n#", "a\n\\#\n"},
		{"first line keeps the wider rule", "0) a\nb", "0\\) a\nb\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out := rebuildBlocks(t, model.NewBlock("p", tc.text))
			assert.Equal(t, tc.want, out)
			assert.Equal(t, out, rebuildBlocks(t, readBlocks(t, out)...),
				"rebuild is not idempotent for %q", tc.text)
		})
	}
}

// TestHTMLBlockTextRebuildsAsOneBlock drives the #2470 reproducer through the
// real read -> rebuild-write -> read harness. The fuzz reproducer is committed
// under testdata/fuzz.
func TestHTMLBlockTextRebuildsAsOneBlock(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"<div>0\n# 0", "<div>a\n- b\nc", "<span>a</span>\n> q"} {
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			out1, keys1, ok := tripMarkdown(ctx, []byte(in))
			require.True(t, ok, "first trip declined %q", in)
			_, keys2, ok2 := tripMarkdown(ctx, out1)
			require.True(t, ok2, "re-reading %q lost every block", out1)
			assert.Len(t, keys2, len(keys1), "block count changed via %q", out1)
		})
	}
}

// TestRebuildKeepsBlockquoteContinuationMarkers pins the exclusion
// escapeBlockMarkerLines relies on: a blockquote body's continuation lines
// carry the ">" the rebuild restores, and escaping those would break the quote
// back into loose paragraphs.
func TestRebuildKeepsBlockquoteContinuationMarkers(t *testing.T) {
	t.Parallel()
	block := model.NewBlock("bq", "a\n> b\n> c")
	out := rebuildBlocks(t, block)
	assert.Equal(t, "> a\n> b\n> c\n", out)
	require.Len(t, readBlocks(t, out), 1)
}
