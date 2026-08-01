package markdown_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/formats/markdown"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// rebuildBlocks runs the writer's no-skeleton (cross-format rebuild) path over
// the given blocks and returns the Markdown it produced.
func rebuildBlocks(t *testing.T, blocks ...*model.Block) string {
	t.Helper()
	var buf bytes.Buffer
	w := markdown.NewWriter()
	require.NoError(t, w.SetOutputWriter(&buf))
	w.SetLocale(model.LocaleEnglish)
	parts := make([]*model.Part, 0, len(blocks))
	for _, b := range blocks {
		parts = append(parts, &model.Part{Type: model.PartBlock, Resource: b})
	}
	require.NoError(t, w.Write(context.Background(), testutil.PartsToChannel(parts)))
	w.Close()
	return buf.String()
}

// TestRebuildEscapesLeadingBlockMarker is the #1632 reproducer. A paragraph
// whose text begins with a bare block marker (the residue of a dropped inline
// construct — e.g. "#<A>" leaves "#") must be emitted so it re-reads as the
// same paragraph, not as a heading/list/quote/fence/thematic-break. Before the
// fix the writer emitted the marker unescaped and the block re-read as a
// different kind, breaking round-trip idempotence.
func TestRebuildEscapesLeadingBlockMarker(t *testing.T) {
	// Every text here would OPEN a block construct if emitted verbatim at the
	// start of a line. The writer must escape the leading marker.
	texts := []string{
		"#", "# heading", "## h", "### h", "###### h", // ATX headings 1-6
		"> quote",                          // blockquote
		"- bullet", "+ bullet", "* bullet", // bullet lists
		"1. ordered", "1) ordered", "12. ordered", "2. loose start", // ordered lists
		"---", "***", "___", "- - -", // thematic breaks
		"``` fence", "~~~ fence", "```", // fenced code
	}
	for _, txt := range texts {
		t.Run(txt, func(t *testing.T) {
			block := model.NewBlock("p", txt)
			out := rebuildBlocks(t, block)

			blocks := readBlocks(t, out)
			require.Len(t, blocks, 1, "escaped output %q must re-read as one block", out)
			got := blocks[0]
			assert.Empty(t, got.SemanticRole(),
				"paragraph %q re-read as role %q via %q", txt, got.SemanticRole(), out)
			assert.Empty(t, got.Type,
				"paragraph %q re-read as block type %q via %q", txt, got.Type, out)

			// Idempotent: re-emitting the re-read block yields the same bytes.
			out2 := rebuildBlocks(t, got)
			assert.Equal(t, out, out2, "rebuild is not idempotent for %q", txt)
		})
	}
}

// TestRebuildBlockMarkerHarnessIdempotent drives the #1632 case through the
// real read -> rebuild-write -> read harness: "#<A>" is a paragraph whose text
// is "#" followed by inline HTML the rebuild path drops. trip(trip(x)) must
// equal trip(x), and the block count must not change.
func TestRebuildBlockMarkerHarnessIdempotent(t *testing.T) {
	for _, in := range []string{"#<A>", "#<A> heading"} {
		t.Run(in, func(t *testing.T) {
			ctx := context.Background()
			out1, texts1, ok := tripMarkdown(ctx, []byte(in))
			require.True(t, ok, "first trip declined %q", in)
			out2, texts2, ok2 := tripMarkdown(ctx, out1)
			require.True(t, ok2, "re-reading %q failed", out1)
			assert.Equal(t, string(out1), string(out2), "trip(trip(x)) != trip(x)")
			assert.Equal(t, len(texts1), len(texts2), "block count changed")
		})
	}
}

// TestRebuildBlockquoteLinePrefix is the #1633 reproducer. "> a\n> b" is ONE
// blockquote block; the rebuild path used to drop the blockquote's own line
// prefix, so the emitted text re-read as two loose paragraphs (block count
// 1 -> 2). The prefix must be re-established on every line.
func TestRebuildBlockquoteLinePrefix(t *testing.T) {
	for _, in := range []string{"> a\n> b", ">0\n>0", "> a\n> b\n> c"} {
		t.Run(in, func(t *testing.T) {
			ctx := context.Background()
			before := readBlocks(t, in)
			require.Len(t, before, 1, "source %q should read as one blockquote block", in)

			out1, texts1, ok := tripMarkdown(ctx, []byte(in))
			require.True(t, ok, "first trip declined %q", in)

			after := readBlocks(t, string(out1))
			require.Len(t, after, 1,
				"rebuilt %q lost the blockquote; re-read as %d blocks:\n%s", in, len(after), out1)

			out2, texts2, ok2 := tripMarkdown(ctx, out1)
			require.True(t, ok2, "re-reading %q failed", out1)
			assert.Equal(t, string(out1), string(out2), "blockquote rebuild is not idempotent")
			assert.Equal(t, len(texts1), len(texts2), "block count changed on second pass")
		})
	}
}
