package markdown_test

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMultiLineHeadingRebuildsAsOneBlock is the #1659 reproducer. "0\n0\n="
// is a two-line paragraph underlined into one setext heading whose text is
// "0\n0". The rebuild path writes every heading as ATX, and an ATX heading is
// single-line, so the embedded newline used to split it into a heading plus
// a loose paragraph (block count 1 -> 2). The heading now folds onto one
// line, and a second rebuild is a fixed point.
func TestMultiLineHeadingRebuildsAsOneBlock(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"0\n0\n=", "a  \nb\n===\n", "first line\nsecond line\n---\n", "> 0\n> 0\n> ===\n"} {
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			before := readBlocks(t, in)
			require.Len(t, before, 1, "source should read as one heading")
			require.Equal(t, model.RoleHeading, before[0].SemanticRole())

			out1, keys1, ok := tripMarkdown(ctx, []byte(in))
			require.True(t, ok, "first trip declined %q", in)

			after := readBlocks(t, string(out1))
			require.Len(t, after, 1, "rebuilt %q re-read as %d blocks:\n%s", in, len(after), out1)
			assert.Equal(t, model.RoleHeading, after[0].SemanticRole(), "rebuilt heading lost its role: %q", out1)
			assert.NotContains(t, after[0].SourceText(), "\n", "rebuilt heading still spans lines: %q", out1)

			out2, keys2, ok2 := tripMarkdown(ctx, out1)
			require.True(t, ok2, "re-reading %q failed", out1)
			assert.Equal(t, string(out1), string(out2), "heading rebuild is not idempotent")
			assert.Len(t, keys2, len(keys1), "block count changed on second pass")
		})
	}
}

// TestCrossFormatHeadingWithBreakStaysOneBlock covers the same defect for a
// heading that never came from Markdown: a block from another format whose
// heading text carries a line break must still rebuild as one heading.
func TestCrossFormatHeadingWithBreakStaysOneBlock(t *testing.T) {
	t.Parallel()
	block := model.NewBlock("h", "first line\n  second line")
	block.SetSemanticRole(model.RoleHeading, 2)
	out := rebuildBlocks(t, block)
	assert.Equal(t, "## first line second line\n", out)

	blocks := readBlocks(t, out)
	require.Len(t, blocks, 1)
	assert.Equal(t, model.RoleHeading, blocks[0].SemanticRole())
	assert.Equal(t, 2, blocks[0].HeadingLevel())
}
