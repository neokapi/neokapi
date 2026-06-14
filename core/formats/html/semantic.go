package html

import (
	"fmt"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"golang.org/x/net/html"
)

// writeSemantic reconstructs HTML from the ordered block + group event stream
// (Mode 3 — no skeleton, no original content). It is the cross-format export
// path: each block's normalized SemanticRole (WS1/WS2) selects an HTML element,
// and group brackets (list/table/row/picture) drive structural nesting. A block
// carrying a fragment-based skeleton (the same-format HTML fallback case) keeps
// its captured surrounding markup instead.
//
// fmtRoleTag maps a normalized SemanticRole to the HTML element wrapping a
// block. Roles needing context (heading level, table cell vs header, captions)
// are resolved in emitBlock rather than this table.
var blockRoleTag = map[string]string{
	model.RoleTitle:      "h1",
	model.RoleParagraph:  "p",
	model.RoleListItem:   "li",
	model.RoleCode:       "code", // wrapped in <pre>
	model.RoleFormula:    "p",
	model.RoleFootnote:   "p",
	model.RolePageHeader: "p",
	model.RolePageFooter: "p",
}

// inlineFmtTag maps an inline run's vocabulary type to its HTML formatting tag.
var inlineFmtTag = map[string]string{
	"fmt:bold":          "strong",
	"fmt:italic":        "em",
	"fmt:underline":     "u",
	"fmt:strikethrough": "s",
	"fmt:superscript":   "sup",
	"fmt:subscript":     "sub",
}

// semanticState tracks the open container stack while emitting.
type semanticState struct {
	stack    []string // open HTML container tags ("ul"/"ol"/"table"/"tr"/"figure"/"" for transparent)
	autoList bool     // an auto-opened <ul> wrapping bare list-item blocks (no list group)
}

func (s *semanticState) top() string {
	if n := len(s.stack); n > 0 {
		return s.stack[n-1]
	}
	return ""
}

func (s *semanticState) inContainer(tag string) bool {
	return slices.Contains(s.stack, tag)
}

func (w *Writer) writeSemantic(events []*model.Part) error {
	st := &semanticState{}
	for _, part := range events {
		switch part.Type {
		case model.PartGroupStart:
			g, ok := part.Resource.(*model.GroupStart)
			if !ok {
				continue
			}
			if err := w.openSemGroup(st, g); err != nil {
				return err
			}
		case model.PartGroupEnd:
			if err := w.closeSemGroup(st); err != nil {
				return err
			}
		case model.PartBlock:
			b, ok := part.Resource.(*model.Block)
			if !ok {
				continue
			}
			if err := w.emitBlock(st, b); err != nil {
				return err
			}
		}
	}
	// Close any still-open auto list and containers (defensive — well-formed
	// input balances its groups).
	if err := w.closeAutoList(st); err != nil {
		return err
	}
	for range st.stack {
		if err := w.closeSemGroup(st); err != nil {
			return err
		}
	}
	return nil
}

func (w *Writer) openSemGroup(st *semanticState, g *model.GroupStart) error {
	// A group boundary terminates any auto-opened bare-list wrapper.
	if err := w.closeAutoList(st); err != nil {
		return err
	}
	var tag string
	switch g.Type {
	case "list", "ordered_list":
		tag = "ul"
		if g.Type == "ordered_list" || strings.EqualFold(g.Properties["class"], "ordered") || strings.Contains(g.Name, "ordered") {
			tag = "ol"
		}
	case "table", "index":
		tag = "table"
	case "table-row":
		tag = "tr"
	case "picture":
		tag = "figure"
	default:
		tag = "" // transparent grouping (group/field_region/…)
	}
	st.stack = append(st.stack, tag)
	if tag != "" {
		return w.print("<" + tag + ">")
	}
	return nil
}

func (w *Writer) closeSemGroup(st *semanticState) error {
	if len(st.stack) == 0 {
		return nil
	}
	tag := st.stack[len(st.stack)-1]
	st.stack = st.stack[:len(st.stack)-1]
	if tag != "" {
		return w.print("</" + tag + ">")
	}
	return nil
}

func (w *Writer) closeAutoList(st *semanticState) error {
	if st.autoList {
		st.autoList = false
		// pop the auto <ul> we pushed
		if n := len(st.stack); n > 0 && st.stack[n-1] == "ul" {
			st.stack = st.stack[:n-1]
		}
		return w.print("</ul>")
	}
	return nil
}

func (w *Writer) emitBlock(st *semanticState, b *model.Block) error {
	// Same-format HTML fallback: a fragment-based skeleton carries the block's
	// own surrounding markup — emit it verbatim, content spliced at the ref.
	if b.Skeleton != nil && b.Skeleton.Strategy == model.SkeletonFragmentBased {
		if err := w.closeAutoList(st); err != nil {
			return err
		}
		text := w.getBlockText(b)
		for _, sp := range b.Skeleton.Parts {
			switch p := sp.(type) {
			case *model.SkeletonText:
				if err := w.print(p.Text); err != nil {
					return err
				}
			case *model.SkeletonRef:
				if err := w.print(text); err != nil {
					return err
				}
			}
		}
		return nil
	}

	role := b.SemanticRole()
	if role == "" {
		role = b.Type
	}

	// list-item auto-wrapping: bare list items (e.g. from DOCX, which has no
	// list group) get a synthesised <ul>; items inside an explicit list group
	// are already under <ul>/<ol>.
	if role == model.RoleListItem {
		if t := st.top(); t != "ul" && t != "ol" {
			if !st.autoList {
				if err := w.print("<ul>"); err != nil {
					return err
				}
				st.stack = append(st.stack, "ul")
				st.autoList = true
			}
		}
	} else if err := w.closeAutoList(st); err != nil {
		return err
	}

	body := w.renderInlineHTML(b)

	switch role {
	case model.RoleHeading:
		level := min(max(headingLevel(b), 1), 6)
		return w.print(fmt.Sprintf("<h%d>%s</h%d>", level, body, level))
	case model.RoleCode:
		return w.print("<pre><code>" + body + "</code></pre>")
	case model.RoleCaption:
		tag := "figcaption"
		if st.top() == "table" {
			tag = "caption"
		}
		return w.print("<" + tag + ">" + body + "</" + tag + ">")
	case model.RoleTableCell, model.RoleTableHeader:
		if st.inContainer("table") || st.inContainer("tr") {
			cell := "td"
			if role == model.RoleTableHeader {
				cell = "th"
			}
			return w.print("<" + cell + ">" + body + "</" + cell + ">")
		}
		return w.print("<p>" + body + "</p>") // bare cell, no table context
	default:
		tag := blockRoleTag[role]
		if tag == "" {
			tag = "p"
		}
		return w.print("<" + tag + ">" + body + "</" + tag + ">")
	}
}

// headingLevel returns a block's heading level from the structural annotation,
// falling back to the legacy "level" property; 0 when neither is present.
func headingLevel(b *model.Block) int {
	if s, ok := b.Structure(); ok && s != nil && s.Level > 0 {
		return s.Level
	}
	if lv := b.Properties["level"]; lv != "" {
		n := 0
		_, _ = fmt.Sscanf(lv, "%d", &n)
		return n
	}
	return 0
}

// renderInlineHTML renders a block's runs (target locale if set, else source)
// as escaped HTML inline content: text is HTML-escaped; inline formatting runs
// become HTML tags from their vocabulary type (so the same clean HTML results
// whatever the source format). An unrecognized PcOpen whose captured Data is
// already an HTML tag is emitted verbatim, preserving same-format inline markup.
func (w *Writer) renderInlineHTML(b *model.Block) string {
	runs := b.Source
	if !w.Locale.IsEmpty() {
		if t := b.TargetRuns(w.Locale); len(t) > 0 {
			runs = t
		}
	}
	var sb strings.Builder
	var open []string // stack of emitted closing tags (or "" for dropped)
	for _, r := range runs {
		switch {
		case r.Text != nil:
			sb.WriteString(html.EscapeString(r.Text.Text))
		case r.PcOpen != nil:
			if tag := inlineFmtTag[r.PcOpen.Type]; tag != "" {
				sb.WriteString("<" + tag + ">")
				open = append(open, "</"+tag+">")
			} else if strings.HasPrefix(strings.TrimSpace(r.PcOpen.Data), "<") {
				sb.WriteString(r.PcOpen.Data)
				open = append(open, "") // closing comes from the matching PcClose Data
			} else {
				open = append(open, "")
			}
		case r.PcClose != nil:
			if n := len(open); n > 0 {
				closer := open[n-1]
				open = open[:n-1]
				if closer != "" {
					sb.WriteString(closer)
				} else if strings.HasPrefix(strings.TrimSpace(r.PcClose.Data), "<") {
					sb.WriteString(r.PcClose.Data)
				}
			}
		case r.Ph != nil:
			if r.Ph.Equiv != "" {
				sb.WriteString(html.EscapeString(r.Ph.Equiv))
			}
		}
	}
	for i := len(open) - 1; i >= 0; i-- {
		if open[i] != "" {
			sb.WriteString(open[i])
		}
	}
	return sb.String()
}

// print writes a string to the writer's output.
func (w *Writer) print(s string) error {
	_, err := fmt.Fprint(w.Output, s)
	return err
}
