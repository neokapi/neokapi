package markdown_test

// The generative (cross-format) Markdown path — the one `kapi kconv --to md`,
// previews, and anything handed to a model take. Driven from HTML, which is the
// reader that emits the full canonical vocabulary, so these assert the writer's
// behaviour rather than any one reader's coverage.
//
// The skeleton path is byte-exact and separate; none of this touches it.

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	htmlfmt "github.com/neokapi/neokapi/core/formats/html"
	"github.com/neokapi/neokapi/core/formats/markdown"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/projection"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// htmlToMarkdown reads HTML and writes Markdown through the no-skeleton path.
func htmlToMarkdown(t *testing.T, src string) string {
	t.Helper()
	ctx := context.Background()

	r := htmlfmt.NewReader()
	require.NoError(t, r.Open(ctx, &model.RawDocument{
		URI:          "in.html",
		SourceLocale: model.LocaleEnglish,
		Reader:       io.NopCloser(bytes.NewReader([]byte(src))),
	}))
	var parts []*model.Part
	for pr := range r.Read(ctx) {
		require.NoError(t, pr.Error)
		if pr.Part != nil {
			parts = append(parts, pr.Part)
		}
	}
	require.NoError(t, r.Close())

	w := markdown.NewWriter()
	var buf bytes.Buffer
	require.NoError(t, w.SetOutputWriter(&buf))
	ch := make(chan *model.Part, len(parts))
	for _, p := range parts {
		ch <- p
	}
	close(ch)
	require.NoError(t, w.Write(ctx, ch))
	return buf.String()
}

// An ordered list is a sequence. Rendering it as bullets loses the one thing
// that makes it ordered.
func TestOrderedListKeepsItsNumbering(t *testing.T) {
	got := htmlToMarkdown(t, `<ol><li>first</li><li>second</li><li>third</li></ol>`)
	assert.Equal(t, "1. first\n2. second\n3. third\n", got)
}

// <ol start="N"> continues a list interrupted by other content; renumbering
// from 1 is a different document.
func TestOrderedListHonoursExplicitStart(t *testing.T) {
	got := htmlToMarkdown(t, `<ol start="7"><li>seven</li><li>eight</li></ol>`)
	assert.Equal(t, "7. seven\n8. eight\n", got)
}

func TestUnorderedListStaysBulleted(t *testing.T) {
	got := htmlToMarkdown(t, `<ul><li>a</li><li>b</li></ul>`)
	assert.Equal(t, "- a\n- b\n", got)
}

// A blank line between items makes CommonMark render each item as its own
// paragraph — a loose list, which is not what the source said.
func TestListItemsAreTight(t *testing.T) {
	got := htmlToMarkdown(t, `<ul><li>a</li><li>b</li></ul>`)
	assert.NotContains(t, got, "\n\n", "consecutive list items must not be separated by a blank line")
}

func TestNestedListIndents(t *testing.T) {
	got := htmlToMarkdown(t, `<ul><li>outer</li><ul><li>inner</li></ul></ul>`)
	assert.Contains(t, got, "- outer")
	assert.Contains(t, got, "  - inner")
}

// Without the marker a quotation is indistinguishable from the prose around it.
func TestBlockquoteKeepsItsMarker(t *testing.T) {
	got := htmlToMarkdown(t, `<blockquote><p>quoted</p></blockquote>`)
	assert.Equal(t, "> quoted\n", got)
}

// Two paragraphs in one quote need a QUOTED blank line between them; a bare
// blank line ends the quotation and starts a second one.
func TestBlockquoteWithTwoParagraphsStaysOneQuote(t *testing.T) {
	got := htmlToMarkdown(t, `<blockquote><p>first</p><p>second</p></blockquote>`)
	assert.Equal(t, "> first\n>\n> second\n", got)
}

func TestBlockquoteEndsAtItsClosingTag(t *testing.T) {
	got := htmlToMarkdown(t, `<blockquote><p>quoted</p></blockquote><p>after</p>`)
	assert.Equal(t, "> quoted\n\nafter\n", got)
}

// Inside a fence the content IS the text. Rendering the inline vocabulary there
// wraps it in a second layer of markup that re-reads as content.
func TestFencedCodeContentIsLiteral(t *testing.T) {
	got := htmlToMarkdown(t, `<pre><code class="language-go">x := 1</code></pre>`)
	assert.Equal(t, "```go\nx := 1\n```\n", got)
	assert.NotContains(t, got, "`x")
}

// A text file ends with a newline.
func TestOutputEndsWithNewline(t *testing.T) {
	for _, src := range []string{
		`<p>plain</p>`,
		`<h1>heading</h1>`,
		`<ul><li>item</li></ul>`,
		`<table><tr><td>cell</td></tr></table>`,
	} {
		got := htmlToMarkdown(t, src)
		assert.True(t, len(got) > 0 && got[len(got)-1] == '\n', "%q should end with a newline", got)
	}
}

// An empty document produces an empty file, not a stray newline.
func TestEmptyDocumentProducesNoOutput(t *testing.T) {
	assert.Empty(t, htmlToMarkdown(t, `<html><body></body></html>`))
}

// A list item with no enclosing list bracket — a reader that emits bare items,
// or one item quoted out of context — still renders as something valid.
func TestBareListItemFallsBackToBullet(t *testing.T) {
	ctx := context.Background()
	block := model.NewBlock("b1", "orphan")
	block.SetSemanticRole(model.RoleListItem, 0)

	w := markdown.NewWriter()
	var buf bytes.Buffer
	require.NoError(t, w.SetOutputWriter(&buf))
	ch := make(chan *model.Part, 1)
	ch <- &model.Part{Type: model.PartBlock, Resource: block}
	close(ch)
	require.NoError(t, w.Write(ctx, ch))

	assert.Equal(t, "- orphan\n", buf.String())
}

// A spreadsheet carries two kinds of block that are storage rather than
// document flow: the shared-string table, and an Excel ListObject's column-name
// definitions. Both restate content the worksheet grid already places at a real
// address, so rendering them appends the sheet — or its header row — to the
// output a second time. They remain the right extraction unit for translation;
// only the generative path skips them.
func TestStorageBlocksAreNotDocumentFlow(t *testing.T) {
	for _, tc := range []struct{ name, blockType string }{
		{"shared string table", "shared-string"},
		{"ListObject column definition", "table-column"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			block := model.NewBlock("b1", "Category")
			block.Type = tc.blockType

			w := markdown.NewWriter()
			var buf bytes.Buffer
			require.NoError(t, w.SetOutputWriter(&buf))
			ch := make(chan *model.Part, 1)
			ch <- &model.Part{Type: model.PartBlock, Resource: block}
			close(ch)
			require.NoError(t, w.Write(context.Background(), ch))

			assert.Empty(t, buf.String(),
				"a %s block must not render as document flow", tc.blockType)
		})
	}
}

// A worksheet has no table container in the source, so the writer assembles a
// grid from a run of bare cell blocks. That run has to end when the source part
// changes: two sheets merged into one table put rows from one grid under
// another grid's header, which is worse than no table at all. The boundary must
// hold on the part path alone — a workbook whose sheet names cannot be resolved
// still has sheets.
func TestCellsFromDifferentPartsFormSeparateTables(t *testing.T) {
	cell := func(id, part, row, text string) *model.Part {
		b := model.NewBlock(id, text)
		b.SetSemanticRole(model.RoleTableCell, 0)
		b.Properties = map[string]string{"partPath": part, projection.PropFlatRow: row}
		return &model.Part{Type: model.PartBlock, Resource: b}
	}
	parts := []*model.Part{
		cell("a1", "xl/worksheets/sheet1.xml", "1", "Product"),
		cell("a2", "xl/worksheets/sheet1.xml", "2", "Honey"),
		cell("b1", "xl/worksheets/sheet2.xml", "1", "Region"),
		cell("b2", "xl/worksheets/sheet2.xml", "2", "North"),
	}

	w := markdown.NewWriter()
	var buf bytes.Buffer
	require.NoError(t, w.SetOutputWriter(&buf))
	ch := make(chan *model.Part, len(parts))
	for _, p := range parts {
		ch <- p
	}
	close(ch)
	require.NoError(t, w.Write(context.Background(), ch))

	got := buf.String()
	assert.Equal(t, 2, strings.Count(got, "| --- |"),
		"two source parts should produce two tables, got:\n%s", got)
	assert.NotContains(t, got, "| Product |\n| --- |\n| Honey |\n| Region |",
		"sheet 2's rows must not continue sheet 1's table")
}
