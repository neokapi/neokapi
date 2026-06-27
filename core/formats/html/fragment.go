package html

import (
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/projection"
	"golang.org/x/net/html"
)

// FragmentHTML renders a single projection RenderNode (e.g. from
// projection.ProjectBlock) to an HTML fragment — no <html>/<body> scaffold. It
// is the per-block projection serializer behind `kapi inspect --project html`
// and the convert-lab Blocks tab: the same render AST the document writer emits,
// rendered one node at a time. A leaf renders its role's element with inline
// runs; a table/list renders its full structure; a document concatenates its
// children.
func FragmentHTML(node *projection.RenderNode) string {
	if node == nil {
		return ""
	}
	switch node.Role {
	case projection.RoleDocument:
		return fragmentChildrenHTML(node)
	case model.RoleTable:
		return fragmentTableHTML(node)
	case model.RoleList:
		tag := "ul"
		if node.Ordered {
			tag = "ol"
		}
		var b strings.Builder
		b.WriteString("<" + tag + ">")
		for _, c := range node.Children {
			b.WriteString("<li>" + renderRunsHTML(c.Runs) + fragmentChildrenHTML(c) + "</li>")
		}
		b.WriteString("</" + tag + ">")
		return b.String()
	case model.RoleHeading, model.RoleTitle:
		lvl := node.Level
		if lvl < 1 {
			lvl = 2
			if node.Role == model.RoleTitle {
				lvl = 1
			}
		}
		if lvl > 6 {
			lvl = 6
		}
		return fmt.Sprintf("<h%d>%s</h%d>", lvl, renderRunsHTML(node.Runs), lvl)
	case model.RoleCode:
		open := "<code>"
		if lang := node.Props[model.PropCodeLanguage]; lang != "" {
			open = `<code class="language-` + html.EscapeString(lang) + `">`
		}
		return "<pre>" + open + renderRunsHTML(node.Runs) + "</code></pre>"
	case model.RoleCaption:
		return "<figcaption>" + renderRunsHTML(node.Runs) + "</figcaption>"
	case model.RoleTableCell, model.RoleTableHeader:
		return fragmentCellHTML(node)
	default:
		return "<p>" + renderRunsHTML(node.Runs) + "</p>"
	}
}

func fragmentChildrenHTML(node *projection.RenderNode) string {
	var b strings.Builder
	for _, c := range node.Children {
		b.WriteString(FragmentHTML(c))
	}
	return b.String()
}

func fragmentTableHTML(node *projection.RenderNode) string {
	var b strings.Builder
	b.WriteString("<table>")
	for _, row := range node.Children {
		b.WriteString("<tr>")
		for _, cell := range row.Children {
			b.WriteString(fragmentCellHTML(cell))
		}
		b.WriteString("</tr>")
	}
	b.WriteString("</table>")
	return b.String()
}

func fragmentCellHTML(node *projection.RenderNode) string {
	tag := "td"
	if node.Header || node.Role == model.RoleTableHeader {
		tag = "th"
	}
	attrs := ""
	if node.ColSpan > 1 {
		attrs += fmt.Sprintf(` colspan="%d"`, node.ColSpan)
	}
	if node.RowSpan > 1 {
		attrs += fmt.Sprintf(` rowspan="%d"`, node.RowSpan)
	}
	return "<" + tag + attrs + ">" + renderRunsHTML(node.Runs) + "</" + tag + ">"
}
