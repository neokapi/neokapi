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
	"testing"

	htmlfmt "github.com/neokapi/neokapi/core/formats/html"
	"github.com/neokapi/neokapi/core/formats/markdown"
	"github.com/neokapi/neokapi/core/model"
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
