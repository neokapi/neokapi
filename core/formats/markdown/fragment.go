package markdown

import (
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/projection"
)

// FragmentMarkdown renders a single projection RenderNode (e.g. from
// projection.ProjectBlock) to a Markdown fragment — no document scaffold. It is
// the per-block projection serializer behind `kapi inspect --project md` and the
// convert-lab Blocks tab: the render AST the document writer emits, one node at
// a time.
func FragmentMarkdown(node *projection.RenderNode) string {
	if node == nil {
		return ""
	}
	switch node.Role {
	case projection.RoleDocument:
		parts := make([]string, 0, len(node.Children))
		for _, c := range node.Children {
			parts = append(parts, FragmentMarkdown(c))
		}
		return strings.Join(parts, "\n\n")
	case model.RoleTable:
		return fragmentTableMarkdown(node)
	case model.RoleList:
		var b strings.Builder
		for i, c := range node.Children {
			if i > 0 {
				b.WriteByte('\n')
			}
			marker := "- "
			if node.Ordered {
				marker = fmt.Sprintf("%d. ", i+1)
			}
			b.WriteString(marker + renderInlineMarkdown(c.Runs))
		}
		return b.String()
	case model.RoleHeading, model.RoleTitle:
		lvl := min(max(node.Level, 1), 6)
		return strings.Repeat("#", lvl) + " " + renderInlineMarkdown(node.Runs)
	case model.RoleCode:
		return "```" + node.Props[model.PropCodeLanguage] + "\n" + model.RunsText(node.Runs) + "\n```"
	default:
		// paragraph / caption / table-cell / list-item alone: inline content.
		return renderInlineMarkdown(node.Runs)
	}
}

func fragmentTableMarkdown(node *projection.RenderNode) string {
	rows := node.Children
	if len(rows) == 0 {
		return ""
	}
	width := 0
	for _, r := range rows {
		if len(r.Children) > width {
			width = len(r.Children)
		}
	}
	if width == 0 {
		return ""
	}
	cells := func(r *projection.RenderNode) []string {
		out := make([]string, width)
		for i, c := range r.Children {
			if i < width {
				out[i] = escapeTableCell(renderInlineMarkdown(c.Runs))
			}
		}
		return out
	}
	line := func(c []string) string { return "| " + strings.Join(c, " | ") + " |" }
	var b strings.Builder
	b.WriteString(line(cells(rows[0])))
	b.WriteByte('\n')
	sep := make([]string, width)
	for i := range sep {
		sep[i] = "---"
	}
	b.WriteString(line(sep))
	for _, r := range rows[1:] {
		b.WriteByte('\n')
		b.WriteString(line(cells(r)))
	}
	return b.String()
}
