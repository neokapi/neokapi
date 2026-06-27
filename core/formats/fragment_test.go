package formats_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// boldPara builds a paragraph block with "a <bold>b</bold>" inline content.
func boldPara(id string) *model.Block {
	b := model.NewBlock("", "")
	b.ID = id
	b.SetSemanticRole(model.RoleParagraph, 0)
	b.Source = []model.Run{
		{Text: &model.TextRun{Text: "a "}},
		{PcOpen: &model.PcOpenRun{ID: "1", Type: "fmt:bold"}},
		{Text: &model.TextRun{Text: "b"}},
		{PcClose: &model.PcCloseRun{ID: "1", Type: "fmt:bold"}},
	}
	return b
}

func TestRenderBlockFragment_InlineAcrossFormats(t *testing.T) {
	b := boldPara("p1")

	html, ok := formats.RenderBlockFragment(b, "html")
	require.True(t, ok)
	assert.Equal(t, "<p>a <strong>b</strong></p>", html)

	md, ok := formats.RenderBlockFragment(b, "markdown")
	require.True(t, ok)
	assert.Equal(t, "a **b**", md)

	adoc, ok := formats.RenderBlockFragment(b, "asciidoc")
	require.True(t, ok)
	assert.Equal(t, "a *b*", adoc)
}

func TestRenderBlockFragment_Heading(t *testing.T) {
	h := model.NewBlock("h", "Title")
	h.SetSemanticRole(model.RoleHeading, 2)

	html, _ := formats.RenderBlockFragment(h, "html")
	assert.Equal(t, "<h2>Title</h2>", html)
	md, _ := formats.RenderBlockFragment(h, "markdown")
	assert.Equal(t, "## Title", md)
	adoc, _ := formats.RenderBlockFragment(h, "asciidoc")
	assert.Equal(t, "== Title", adoc)
}

func TestRenderBlockFragment_TableCell(t *testing.T) {
	c := model.NewBlock("c", "X")
	c.SetSemanticRole(model.RoleTableHeader, 0)
	html, _ := formats.RenderBlockFragment(c, "html")
	assert.Equal(t, "<th>X</th>", html)
	adoc, _ := formats.RenderBlockFragment(c, "asciidoc")
	assert.Equal(t, "| X", adoc)
}

func TestRenderBlockFragment_Code(t *testing.T) {
	code := model.NewBlock("code", "fmt.Println(1)")
	code.SetSemanticRole(model.RoleCode, 0)
	code.SetCodeLanguage("go")
	html, _ := formats.RenderBlockFragment(code, "html")
	assert.Contains(t, html, `<pre><code class="language-go">fmt.Println(1)</code></pre>`)
	md, _ := formats.RenderBlockFragment(code, "markdown")
	assert.Equal(t, "```go\nfmt.Println(1)\n```", md)
}

func TestRenderBlockFragment_UnknownFormat(t *testing.T) {
	_, ok := formats.RenderBlockFragment(boldPara("p"), "pdf")
	assert.False(t, ok)
	assert.Equal(t, []string{"asciidoc", "html", "markdown"}, formats.BlockFragmentFormats())
}
