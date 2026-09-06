package markdown_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLinkTitleRebuildsInsideTheLink is the #2445 reproducer. The reader offers
// a link's or image's title as a text run between a second pair of codes, and
// the rebuild path rendered that pair as a link of its own: `[a](b 't')` came
// back as `[a](b "t")[t]()`, and every pass added another. The title now
// reaches the output inside the closer of the link before it.
func TestLinkTitleRebuildsInsideTheLink(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"single quotes", "[a](b 't')\n", "[a](b \"t\")\n"},
		{"double quotes", "[a](b \"t\")\n", "[a](b \"t\")\n"},
		{"parentheses", "[a](b (t))\n", "[a](b \"t\")\n"},
		{"image", "![a](b 't')\n", "![a](b \"t\")\n"},
		{"image with parentheses", "![a](b (t))\n", "![a](b \"t\")\n"},
		{"no title", "[a](b)\n", "[a](b)\n"},
		{"empty link text", "[](b 't')\n", "[](b \"t\")\n"},
		{"in a sentence", "x [a](b 't') y\n", "x [a](b \"t\") y\n"},
		{"two titled links", "[a](b 't') [c](d 'u')\n", "[a](b \"t\") [c](d \"u\")\n"},
		{"image inside a link, both titled", "[![i](s 'st')](b 'lt')\n", "[![i](s \"st\")](b \"lt\")\n"},
		{"quote inside the title", "[a](b 'say \"hi\"')\n", "[a](b \"say \\\"hi\\\"\")\n"},
		{"escaped quote inside the title", "[a](b \"say \\\"hi\\\"\")\n", "[a](b \"say \\\"hi\\\"\")\n"},
		{"emphasis in the link text", "[*a*](b 't')\n", "[*a*](b \"t\")\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			blocks := readBlocks(t, tc.in)
			require.Len(t, blocks, 1)
			assert.Equal(t, tc.want, rebuildBlocks(t, blocks[0]))
		})
	}
}

// TestLinkTitleRebuildIsAFixedPoint pins what the bug cost: the extra link was
// content the next pass read back and rendered again, so the block grew on
// every rebuild. The block count and the content keys now hold from the first
// pass on, which is the round-trip contract FuzzRoundTripMarkdown asserts.
func TestLinkTitleRebuildIsAFixedPoint(t *testing.T) {
	t.Parallel()
	inputs := []string{
		"[a](b 't')",
		"![a](b (t))",
		"x [a](b 't') y [c](d \"u\") z",
		"[![i](s 'st')](b 'lt')",
		"[a](b 'say \"hi\"')",
	}
	for i, in := range inputs {
		t.Run(fmt.Sprintf("%d %q", i, in), func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			out1, keys1, ok := tripMarkdown(ctx, []byte(in))
			require.True(t, ok, "first trip declined %q", in)

			out2, keys2, ok2 := tripMarkdown(ctx, out1)
			require.True(t, ok2, "re-reading %q failed", out1)
			assert.Equal(t, string(out1), string(out2), "rebuild is not idempotent")
			assert.Len(t, keys2, len(keys1), "block count changed on the second pass")

			_, keys3, ok3 := tripMarkdown(ctx, out2)
			require.True(t, ok3, "re-reading %q failed", out2)
			assert.Equal(t, keys2, keys3, "content identity drifted after the first pass")
		})
	}
}

// TestRebuildTitlePairWithoutItsLink covers a target whose runs were reordered
// so a title pair no longer follows the link it belongs to. Its text stays in
// the output as text; what it must not do is spell a link of its own.
func TestRebuildTitlePairWithoutItsLink(t *testing.T) {
	t.Parallel()
	block := model.NewBlock("p", "")
	block.Source = []model.Run{
		{Text: &model.TextRun{Text: "x "}},
		{PcOpen: &model.PcOpenRun{ID: "1", Type: "link:hyperlink", SubType: "md:link-title", Data: "(b '"}},
		{Text: &model.TextRun{Text: "t"}},
		{PcClose: &model.PcCloseRun{ID: "1", Type: "link:hyperlink", SubType: "md:link-title", Data: "')"}},
		{Text: &model.TextRun{Text: " y"}},
	}
	assert.Equal(t, "x t y\n", rebuildBlocks(t, block))
}
