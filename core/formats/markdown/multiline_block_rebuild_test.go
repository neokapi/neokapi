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
