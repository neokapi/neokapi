package markdown

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
)

// writeBlocks runs the Markdown writer in its no-skeleton (semantic export)
// mode over the given blocks and returns the produced Markdown.
func writeBlocks(t *testing.T, blocks ...*model.Block) string {
	t.Helper()
	var buf bytes.Buffer
	w := NewWriter()
	if err := w.SetOutputWriter(&buf); err != nil {
		t.Fatalf("SetOutputWriter: %v", err)
	}
	parts := make(chan *model.Part)
	go func() {
		for _, b := range blocks {
			parts <- &model.Part{Type: model.PartBlock, Resource: b}
		}
		close(parts)
	}()
	if err := w.Write(context.Background(), parts); err != nil {
		t.Fatalf("Write: %v", err)
	}
	return buf.String()
}

// WS6: a block carrying only the normalized SemanticRole (no format-specific
// block.Type) must still export correct Markdown structure — this is the
// cross-format case (e.g. an HTML/DOCX/DocLang source → clean Markdown).
func TestExportFromSemanticRole(t *testing.T) {
	heading := model.NewBlock("h", "Title")
	heading.SetSemanticRole(model.RoleHeading, 2) // no block.Type set
	para := model.NewBlock("p", "intro text")     // no role → plain paragraph
	item := model.NewBlock("i", "first")
	item.SetSemanticRole(model.RoleListItem, 0)

	out := writeBlocks(t, heading, para, item)

	if !strings.Contains(out, "## Title") {
		t.Errorf("heading not rendered from SemanticRole; got:\n%s", out)
	}
	if !strings.Contains(out, "- first") {
		t.Errorf("list item not rendered from SemanticRole; got:\n%s", out)
	}
	if !strings.Contains(out, "intro text") {
		t.Errorf("paragraph text missing; got:\n%s", out)
	}
}

// WS6: title, code, and caption roles export to their Markdown structures.
func TestExportTitleCodeCaption(t *testing.T) {
	title := model.NewBlock("t", "My Doc")
	title.SetSemanticRole(model.RoleTitle, 0)
	code := model.NewBlock("c", "fmt.Println(\"hi\")")
	code.SetSemanticRole(model.RoleCode, 0)
	caption := model.NewBlock("cap", "Figure 1")
	caption.SetSemanticRole(model.RoleCaption, 0)

	out := writeBlocks(t, title, code, caption)

	if !strings.Contains(out, "# My Doc") {
		t.Errorf("title not rendered as level-1 heading; got:\n%s", out)
	}
	if !strings.Contains(out, "```\nfmt.Println(\"hi\")\n```") {
		t.Errorf("code not rendered as fenced block; got:\n%s", out)
	}
	if !strings.Contains(out, "*Figure 1*") {
		t.Errorf("caption not rendered as emphasis; got:\n%s", out)
	}
}

// WS6: inline formatting renders from each run's vocabulary TYPE (not its
// source-format Data), so a DocLang/Docling source's "<bold>" projects to
// Markdown "**bold**" rather than leaking the literal tag.
func TestExportInlineFromType(t *testing.T) {
	b := &model.Block{ID: "p", Translatable: true, Source: []model.Run{
		{Text: &model.TextRun{Text: "see "}},
		{PcOpen: &model.PcOpenRun{ID: "1", Type: "fmt:bold", Data: "<bold>"}},
		{Text: &model.TextRun{Text: "this"}},
		{PcClose: &model.PcCloseRun{Data: "</bold>"}},
		{Text: &model.TextRun{Text: " and "}},
		{PcOpen: &model.PcOpenRun{ID: "2", Type: "fmt:italic", Data: "<italic>"}},
		{Text: &model.TextRun{Text: "that"}},
		{PcClose: &model.PcCloseRun{Data: "</italic>"}},
	}}
	b.SetSemanticRole(model.RoleParagraph, 0)

	out := writeBlocks(t, b)
	if !strings.Contains(out, "see **this** and *that*") {
		t.Errorf("inline formatting not rendered from type; got:\n%s", out)
	}
	if strings.Contains(out, "<bold>") || strings.Contains(out, "<italic>") {
		t.Errorf("source-format inline Data leaked into Markdown; got:\n%s", out)
	}
}

// WS6 fallback: a block typed the legacy way (block.Type + "level" property,
// no SemanticRole) must still render — same-format round-trips are unchanged.
func TestExportFallsBackToBlockType(t *testing.T) {
	b := model.NewBlock("h", "Legacy Heading")
	b.Type = "heading"
	b.Properties["level"] = "3"

	out := writeBlocks(t, b)
	if !strings.Contains(out, "### Legacy Heading") {
		t.Errorf("legacy block.Type heading not rendered; got:\n%s", out)
	}
}
