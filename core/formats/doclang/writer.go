package doclang

import (
	"context"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Writer implements DataFormatWriter for DocLang documents. It serializes the
// content model + structural layer (SemanticRole, GeometryAnnotation,
// LayoutLayer) back to DocLang v0.6.
//
// Inline formatting is rendered from each run's vocabulary Type (not its
// captured Data), so the writer produces the SAME DocLang regardless of the
// source format — faithful DocLang↔DocLang round-trip and native→DocLang
// projection alike. A <location> block is emitted only when a block carries
// geometry (it is optional in DocLang), so reflowable content projects to a
// valid geometry-less DocLang.
type Writer struct {
	format.BaseFormatWriter
	cfg        *Config
	groupStack []string
}

// NewWriter creates a new DocLang writer.
func NewWriter() *Writer {
	cfg := &Config{}
	cfg.Reset()
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{FormatName: "doclang"},
		cfg:              cfg,
	}
}

// roleElem maps a normalized SemanticRole to its DocLang block element name and
// whether the element carries a level attribute.
func roleElem(role string) (elem string, hasLevel bool) {
	switch role {
	case model.RoleHeading:
		return "heading", true
	case model.RoleFootnote:
		return "footnote", false
	case model.RoleCode:
		return "code", false
	case model.RoleFormula:
		return "formula", false
	case model.RolePageHeader:
		return "page_header", false
	case model.RolePageFooter:
		return "page_footer", false
	case model.RoleTitle:
		return "title", false
	default: // paragraph, list-item, or unset
		return "text", false
	}
}

// typeToDocTag maps an inline run's vocabulary type to a DocLang formatting tag.
var typeToDocTag = map[string]string{
	"fmt:bold":          "bold",
	"fmt:italic":        "italic",
	"fmt:underline":     "underline",
	"fmt:strikethrough": "strikethrough",
	"fmt:superscript":   "superscript",
	"fmt:subscript":     "subscript",
}

var bodyEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

// Write consumes Parts and serializes a DocLang document.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	var b strings.Builder
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				if _, err := w.Output.Write([]byte(b.String())); err != nil {
					return err
				}
				return nil
			}
			switch part.Type {
			case model.PartLayerStart:
				b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
				fmt.Fprintf(&b, "<doclang xmlns=%q version=\"0.6\">\n", Namespace)
			case model.PartLayerEnd:
				b.WriteString("</doclang>\n")
			case model.PartGroupStart:
				if g, ok := part.Resource.(*model.GroupStart); ok {
					w.openGroup(&b, g)
				}
			case model.PartGroupEnd:
				w.closeGroup(&b)
			case model.PartBlock:
				if blk, ok := part.Resource.(*model.Block); ok {
					w.writeBlock(&b, blk)
				}
			}
		}
	}
}

func (w *Writer) parentGroup() string {
	if n := len(w.groupStack); n > 0 {
		return w.groupStack[n-1]
	}
	return ""
}

func (w *Writer) openGroup(b *strings.Builder, g *model.GroupStart) {
	w.groupStack = append(w.groupStack, g.Type)
	switch g.Type {
	case "list":
		class := g.Properties["class"]
		if class == "" {
			class = "unordered"
		}
		fmt.Fprintf(b, "<list class=%q>\n", class)
	case "table", "index", "group", "field_region":
		fmt.Fprintf(b, "<%s>\n", g.Type)
	case "picture":
		b.WriteString("<picture>\n")
	case "table-row":
		// OTSL rows are delimited by <nl/>; no opening element.
	}
}

func (w *Writer) closeGroup(b *strings.Builder) {
	if len(w.groupStack) == 0 {
		return
	}
	t := w.groupStack[len(w.groupStack)-1]
	w.groupStack = w.groupStack[:len(w.groupStack)-1]
	switch t {
	case "table-row":
		b.WriteString("<nl/>\n")
	case "list", "table", "index", "group", "field_region", "picture":
		fmt.Fprintf(b, "</%s>\n", t)
	}
}

func (w *Writer) writeBlock(b *strings.Builder, blk *model.Block) {
	role := blk.SemanticRole()
	body := renderXMLBody(w.blockRuns(blk))

	// OTSL table cell.
	if w.parentGroup() == "table-row" {
		tok := "fcel"
		if role == model.RoleTableHeader {
			tok = "ched"
		}
		fmt.Fprintf(b, "<%s/>%s", tok, body)
		return
	}

	elem, hasLevel := roleElem(role)

	// A list item is delimited by <ldiv/> then a <text>.
	if w.parentGroup() == "list" && role == model.RoleListItem {
		b.WriteString("<ldiv/>")
	}

	if hasLevel {
		level := 1
		if s, ok := blk.Structure(); ok && s != nil && s.Level > 0 {
			level = s.Level
		}
		fmt.Fprintf(b, "<%s level=\"%d\">", elem, level)
	} else {
		fmt.Fprintf(b, "<%s>", elem)
	}
	w.writeHead(b, blk)
	b.WriteString(body)
	fmt.Fprintf(b, "</%s>\n", elem)
}

// writeHead emits the element-head properties we model: <layer> then the
// 4-value <location> block (geometry).
func (w *Writer) writeHead(b *strings.Builder, blk *model.Block) {
	if layer := blk.LayoutLayer(); layer != "" && layer != model.LayerBody {
		fmt.Fprintf(b, "<layer value=%q/>", layer)
	}
	if !w.cfg.EmitGeometry {
		return
	}
	g, ok := blk.Geometry()
	if !ok || g == nil {
		return
	}
	x0 := int(g.BBox.X)
	y0 := int(g.BBox.Y)
	x1 := int(g.BBox.X + g.BBox.W)
	y1 := int(g.BBox.Y + g.BBox.H)
	resAttr := ""
	if g.Resolution != 0 && g.Resolution != 512 {
		resAttr = fmt.Sprintf(" resolution=\"%d\"", g.Resolution)
	}
	for _, v := range []int{x0, y0, x1, y1} {
		fmt.Fprintf(b, "<location value=\"%d\"%s/>", v, resAttr)
	}
}

// blockRuns returns the runs to serialize: the target for the writer's locale
// when present, else the source.
func (w *Writer) blockRuns(blk *model.Block) []model.Run {
	if !w.Locale.IsEmpty() {
		if t := blk.TargetRuns(w.Locale); t != nil {
			return t
		}
	}
	return blk.Source
}

// renderXMLBody serializes a run sequence as DocLang inline content: text is
// XML-escaped; inline formatting runs become DocLang tags derived from their
// vocabulary type (balanced via a tag stack, so the same output results whether
// the runs came from DocLang or any other source).
func renderXMLBody(runs []model.Run) string {
	var sb strings.Builder
	var open []string
	for _, r := range runs {
		switch {
		case r.Text != nil:
			sb.WriteString(bodyEscaper.Replace(r.Text.Text))
		case r.PcOpen != nil:
			tag := typeToDocTag[r.PcOpen.Type]
			open = append(open, tag)
			if tag != "" {
				sb.WriteString("<" + tag + ">")
			}
		case r.PcClose != nil:
			if n := len(open); n > 0 {
				tag := open[n-1]
				open = open[:n-1]
				if tag != "" {
					sb.WriteString("</" + tag + ">")
				}
			}
		case r.Ph != nil:
			if r.Ph.Equiv != "" {
				sb.WriteString(bodyEscaper.Replace(r.Ph.Equiv))
			}
		}
	}
	for i := len(open) - 1; i >= 0; i-- {
		if open[i] != "" {
			sb.WriteString("</" + open[i] + ">")
		}
	}
	return sb.String()
}
