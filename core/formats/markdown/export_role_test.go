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
