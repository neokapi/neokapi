package markdown_test

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRebuildEscapesContainerLeadingMarker is the #2469 reproducer. The
// rebuild path drops inline HTML, and the residue of "* <A0A>#" is a list item
// whose text is a bare "#". Written after "- " unescaped it read as an item
// holding an empty ATX heading, which carries no content, so the item was gone.
// The leading-marker escape #1632 applies to a paragraph now applies wherever
// the text lands in block-content position: after a list item's marker, and
// after a rebuilt blockquote's.
func TestRebuildEscapesContainerLeadingMarker(t *testing.T) {
	t.Parallel()
	// Every text here opens a block construct when it follows "- " or "> ".
	texts := []string{
		"#", "# heading", "###### h",
		"> quote",
		"- bullet", "+ bullet", "* bullet",
		"1. ordered", "1) ordered",
		"---", "***", "___",
		"``` fence", "~~~ fence",
	}
	for _, txt := range texts {
		t.Run(txt, func(t *testing.T) {
			t.Parallel()

			item := model.NewBlock("li", txt)
			item.SetSemanticRole(model.RoleListItem, 0)
			out := rebuildBlocks(t, item)

			blocks := readBlocks(t, out)
			require.Len(t, blocks, 1, "item %q re-read as %d blocks via %q", txt, len(blocks), out)
			assert.Equal(t, model.RoleListItem, blocks[0].SemanticRole(),
				"item %q re-read as role %q via %q", txt, blocks[0].SemanticRole(), out)
			assert.NotEmpty(t, blocks[0].SourceText(), "item %q lost its text via %q", txt, out)
			assert.Equal(t, out, rebuildBlocks(t, blocks[0]), "rebuild is not idempotent for %q", txt)

			quote := model.NewBlock("bq", txt+"\nb")
			quote.Properties["md:line-prefix"] = "> "
			qout := rebuildBlocks(t, quote)
			qblocks := readBlocks(t, qout)
			require.Len(t, qblocks, 1, "quote %q re-read as %d blocks via %q", txt, len(qblocks), qout)
			assert.Contains(t, qblocks[0].SourceText(), "b", "quote %q lost its body via %q", txt, qout)
		})
	}
}

// TestListItemMarkerResidueSurvivesRebuild drives the #2469 reproducer through
// the real read -> rebuild-write -> read harness. The fuzz reproducer is
// committed under testdata/fuzz.
func TestListItemMarkerResidueSurvivesRebuild(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"* <A0A>#", "- <A>#", "* <A>#\n* b", "> #<A>\n> b"} {
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
