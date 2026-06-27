package asciidoc

import (
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/projection"
)

// FragmentAsciidoc renders a single projection RenderNode (e.g. from
// projection.ProjectBlock) to an AsciiDoc fragment — no document scaffold. It is
// the per-block projection serializer behind `kapi inspect --project asciidoc`
// and the convert-lab Blocks tab.
func FragmentAsciidoc(node *projection.RenderNode) string {
	if node == nil {
		return ""
	}
	switch node.Role {
	case projection.RoleDocument:
		parts := make([]string, 0, len(node.Children))
		for _, c := range node.Children {
			parts = append(parts, FragmentAsciidoc(c))
		}
		return strings.Join(parts, "\n\n")
	case model.RoleTable:
		var b strings.Builder
		b.WriteString("|===\n")
		for _, row := range node.Children {
			for _, cell := range row.Children {
				b.WriteString("| " + renderInlineAsciidoc(cell.Runs) + "\n")
			}
		}
		b.WriteString("|===")
		return b.String()
	case model.RoleList:
		var b strings.Builder
		for i, c := range node.Children {
			if i > 0 {
				b.WriteByte('\n')
			}
			b.WriteString("* " + renderInlineAsciidoc(c.Runs))
		}
		return b.String()
	case model.RoleHeading, model.RoleTitle:
		lvl := max(node.Level, 1)
		return strings.Repeat("=", lvl) + " " + renderInlineAsciidoc(node.Runs)
	case model.RoleCode:
		body := model.RunsText(node.Runs)
		if lang := node.Props[model.PropCodeLanguage]; lang != "" {
			return "[source," + lang + "]\n----\n" + body + "\n----"
		}
		return "----\n" + body + "\n----"
	case model.RoleCaption:
		return "." + renderInlineAsciidoc(node.Runs)
	case model.RoleTableCell, model.RoleTableHeader:
		return "| " + renderInlineAsciidoc(node.Runs)
	default:
		return renderInlineAsciidoc(node.Runs)
	}
}
