package formats_test

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/neokapi/neokapi/core/formats/asciidoc"
	htmlfmt "github.com/neokapi/neokapi/core/formats/html"
	"github.com/neokapi/neokapi/core/formats/markdown"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Cross-format export fidelity (the no-skeleton "convert" path). When a
// document is read by one format and written by a DIFFERENT format, inline
// formatting, links, images, and code-block language must project into the
// target format's native markup — not flatten to plain text or leak the source
// format's literal syntax. These tests drive real reader→writer pairs through
// the writers' Mode-2/3 semantic-export path (no skeleton, no original bytes).

type xfReader interface {
	Open(context.Context, *model.RawDocument) error
	Read(context.Context) <-chan model.PartResult
	Close() error
}

type xfWriter interface {
	SetOutputWriter(io.Writer) error
	Write(context.Context, <-chan *model.Part) error
}

// convert reads input through r and writes it through w using w's no-skeleton
// semantic-export path, returning the produced bytes.
func convert(t *testing.T, r xfReader, w xfWriter, input string) string {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, r.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	var parts []*model.Part
	for pr := range r.Read(ctx) {
		require.NoError(t, pr.Error)
		parts = append(parts, pr.Part)
	}
	require.NoError(t, r.Close())

	var buf bytes.Buffer
	require.NoError(t, w.SetOutputWriter(&buf))
	ch := make(chan *model.Part)
	go func() {
		for _, p := range parts {
			ch <- p
		}
		close(ch)
	}()
	require.NoError(t, w.Write(ctx, ch))
	return buf.String()
}

func TestCrossFormat_MarkdownToHTML(t *testing.T) {
	out := convert(t, markdown.NewReader(), htmlfmt.NewWriter(),
		"A **bold** and *italic* and `code` and [link](https://example.com) and ![alt text](img.png) end")

	assert.Contains(t, out, "<strong>bold</strong>", "bold lost")
	assert.Contains(t, out, "<em>italic</em>", "italic lost")
	assert.Contains(t, out, "<code>code</code>", "inline code lost (the reported HTML gap)")
	assert.Contains(t, out, `<a href="https://example.com">link</a>`, "link href lost")
	assert.Contains(t, out, `<img src="img.png" alt="alt text"`, "image src/alt lost")
}

func TestCrossFormat_MarkdownToAsciidoc(t *testing.T) {
	out := convert(t, markdown.NewReader(), asciidoc.NewWriter(),
		"A **bold** and *italic* and `code` and [link](https://example.com) and ![alt text](img.png) end")

	assert.Contains(t, out, "*bold*", "bold not projected to AsciiDoc")
	assert.Contains(t, out, "_italic_", "italic not projected to AsciiDoc")
	assert.Contains(t, out, "`code`", "inline code not projected to AsciiDoc")
	assert.Contains(t, out, "link:https://example.com[link]", "link not projected to AsciiDoc macro")
	assert.Contains(t, out, "image:img.png[alt text]", "image not projected to AsciiDoc macro")
	// Markdown's literal inline syntax must not leak into AsciiDoc output.
	assert.NotContains(t, out, "**bold**", "markdown bold markers leaked into AsciiDoc")
}

func TestCrossFormat_HTMLToMarkdown(t *testing.T) {
	out := convert(t, htmlfmt.NewReader(), markdown.NewWriter(),
		`<p>A <strong>bold</strong> and <code>code</code> and `+
			`<a href="https://example.com">link</a> and `+
			`<img src="img.png" alt="alt text"> end</p>`)

	assert.Contains(t, out, "**bold**", "bold not projected to Markdown")
	assert.Contains(t, out, "`code`", "inline code not projected to Markdown")
	assert.Contains(t, out, "[link](https://example.com)", "link not projected to Markdown")
	assert.Contains(t, out, "![alt text](img.png)", "image not projected to Markdown")
}

const mdTable = "# Title\n\nIntro paragraph.\n\n| A | B |\n|---|---|\n| 1 | 2 |\n| 3 | 4 |\n"

// A GFM table must rebuild as a real grid when converted to another format,
// not collapse to standalone paragraphs (preview-fidelity evidence #2). The
// markdown reader now emits the canonical table/table-row group shape, so the
// existing generative HTML/AsciiDoc writers reconstruct the grid.
func TestCrossFormat_MarkdownTableToHTML(t *testing.T) {
	out := convert(t, markdown.NewReader(), htmlfmt.NewWriter(), mdTable)

	assert.Contains(t, out, "<table", "table collapsed — no <table> emitted (the reported gap)")
	assert.Contains(t, out, "<th", "header cells not emitted as <th>")
	assert.Contains(t, out, "<td", "body cells not emitted as <td>")
	// Every cell's text must survive.
	for _, cell := range []string{"A", "B", "1", "2", "3", "4"} {
		assert.Contains(t, out, ">"+cell+"<", "cell %q lost", cell)
	}
	// Cells must not leak as standalone paragraphs.
	assert.NotContains(t, out, "<p>A</p>", "cell leaked as a standalone paragraph")
}

func TestCrossFormat_MarkdownTableToAsciidoc(t *testing.T) {
	out := convert(t, markdown.NewReader(), asciidoc.NewWriter(), mdTable)

	assert.Contains(t, out, "|===", "table not wrapped in an AsciiDoc table block")
	for _, cell := range []string{"A", "B", "1", "2", "3", "4"} {
		assert.Contains(t, out, "| "+cell, "cell %q lost", cell)
	}
}

// An HTML table must rebuild as a real grid when converted to another format.
// The html reader now emits the canonical table/table-row group shape (and
// carries colspan/rowspan), so the AsciiDoc/Markdown writers reconstruct it.
func TestCrossFormat_HTMLTableToAsciidoc(t *testing.T) {
	out := convert(t, htmlfmt.NewReader(), asciidoc.NewWriter(),
		`<table><tr><th>A</th><th>B</th></tr><tr><td>1</td><td>2</td></tr></table>`)

	assert.Contains(t, out, "|===", "html table not wrapped in an AsciiDoc table block")
	for _, cell := range []string{"A", "B", "1", "2"} {
		assert.Contains(t, out, "| "+cell, "cell %q lost", cell)
	}
}

func TestCrossFormat_HTMLTableToMarkdown(t *testing.T) {
	out := convert(t, htmlfmt.NewReader(), markdown.NewWriter(),
		`<table><tr><th>A</th><th>B</th></tr><tr><td>1</td><td>2</td></tr></table>`)

	// GFM table: a header row, a separator, and a body row.
	assert.Contains(t, out, "| A | B |", "html table header not projected to GFM")
	assert.Regexp(t, `\|\s*-+\s*\|`, out, "GFM separator row missing")
	assert.Contains(t, out, "| 1 | 2 |", "html table body not projected to GFM")
}

// codeBlock builds a non-translatable RoleCode block tagged with a language,
// the shape every reader produces for a fenced/listing code block.
func codeBlock(body, lang string) *model.Block {
	b := model.NewBlock("code", body)
	b.Translatable = false
	b.SetSemanticRole(model.RoleCode, 0)
	if lang != "" {
		b.SetCodeLanguage(lang)
	}
	return b
}

func writeOneBlock(t *testing.T, w xfWriter, b *model.Block) string {
	t.Helper()
	var buf bytes.Buffer
	require.NoError(t, w.SetOutputWriter(&buf))
	ch := make(chan *model.Part)
	go func() {
		ch <- &model.Part{Type: model.PartBlock, Resource: b}
		close(ch)
	}()
	require.NoError(t, w.Write(context.Background(), ch))
	return buf.String()
}

func TestCrossFormat_CodeBlockLanguage(t *testing.T) {
	body := "fmt.Println(\"hi\")"

	md := writeOneBlock(t, markdown.NewWriter(), codeBlock(body, "go"))
	assert.Contains(t, md, "```go\n"+body+"\n```", "markdown code fence dropped the language")

	adoc := writeOneBlock(t, asciidoc.NewWriter(), codeBlock(body, "go"))
	assert.Contains(t, adoc, "[source,go]\n----\n"+body, "asciidoc listing dropped the language")
	assert.Contains(t, adoc, "----\n", "asciidoc code not wrapped in a listing block")

	html := writeOneBlock(t, htmlfmt.NewWriter(), codeBlock(body, "go"))
	assert.Contains(t, html, `<pre><code class="language-go">`, "html code dropped the language class")
}

// Compile-time assertion that the writers satisfy the minimal xfWriter surface
// the cross-format harness needs.
var (
	_ xfWriter = (*markdown.Writer)(nil)
	_ xfWriter = (*asciidoc.Writer)(nil)
	_ xfWriter = (*htmlfmt.Writer)(nil)
)
