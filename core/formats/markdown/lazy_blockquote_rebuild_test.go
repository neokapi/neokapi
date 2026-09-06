package markdown_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLazyContinuationBlockquoteRebuildsAsOneBlock is the #2434 reproducer,
// found by FuzzRoundTripMarkdown. "> a\nb\n> c" is one blockquote whose second
// line is a lazy continuation (CommonMark 5.1). The rebuild path judged the
// block by its first continuation line, found no marker there, and wrote the
// block back as a paragraph followed by a blockquote, so the block count
// changed. The marker is now recovered from the first marked continuation
// line, and a second rebuild is a fixed point.
func TestLazyContinuationBlockquoteRebuildsAsOneBlock(t *testing.T) {
	t.Parallel()
	inputs := []string{
		"> a\nb\n> c",
		"> a\nb\n> c\n",
		"> a\nb\nc\n> d",
		">a\nb\n>c",
		">\xb4\xea000\xe7\n0\n>0", // the fuzz reproducer
	}
	for i, in := range inputs {
		t.Run(fmt.Sprintf("%d %q", i, in), func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			before := readBlocks(t, in)
			require.Len(t, before, 1, "source should read as one blockquote block")

			out1, keys1, ok := tripMarkdown(ctx, []byte(in))
			require.True(t, ok, "first trip declined %q", in)

			after := readBlocks(t, string(out1))
			require.Len(t, after, 1, "rebuilt %q re-read as %d blocks:\n%s", in, len(after), out1)
			assert.Equal(t, before[0].SourceText(), after[0].SourceText(), "block text changed on re-read")

			out2, keys2, ok2 := tripMarkdown(ctx, out1)
			require.True(t, ok2, "re-reading %q failed", out1)
			assert.Equal(t, string(out1), string(out2), "blockquote rebuild is not idempotent")
			assert.Len(t, keys2, len(keys1), "block count changed on second pass")
		})
	}
}

// TestRebuildBlockquoteMarkerFromLaterLine pins the marker recovery on a block
// built directly, which is the only place it answers: a markdown quote body
// carries the marker property instead. EVERY continuation line must be marked.
// A paragraph that merely holds a ">" line — what the rebuild path leaves when
// it drops an inline construct that occupied one — was re-marked as a quote and
// split in two on the pass after (#2505).
func TestRebuildBlockquoteMarkerFromLaterLine(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "> a\n> b\n", rebuildBlocks(t, model.NewBlock("q", "a\n> b")))
	assert.Equal(t, ">a\n>b\n", rebuildBlocks(t, model.NewBlock("q", "a\n>b")))
	assert.Equal(t, "a\nb\n\\> c\n", rebuildBlocks(t, model.NewBlock("p", "a\nb\n> c")))
	assert.Equal(t, "a\nb\n", rebuildBlocks(t, model.NewBlock("p", "a\nb")))
}
