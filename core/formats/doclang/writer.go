package doclang

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Writer implements DataFormatWriter for DocLang documents. It serializes the
// content model + structural layer (SemanticRole, GeometryAnnotation,
// LayoutLayer, RelationAnnotation, ColSpan/RowSpan, and the Block.Properties
// conventions) back to DocLang v0.6.
//
// Inline formatting is rendered from each run's vocabulary Type (not its
// captured Data), so the writer produces the SAME DocLang regardless of the
// source format — faithful DocLang↔DocLang round-trip and native→DocLang
// projection alike. A <location> block is emitted only when a block carries
// geometry (it is optional in DocLang), so reflowable content projects to a
// valid geometry-less DocLang.
//
// The writer drains the whole Part stream before rendering: this lets it assign
// <thread> ids across RelContinues edges (the continuation graph is only fully
// known at the end) and reconstruct OTSL merged-cell spans (lcel/ucel/xcel),
// which span across rows.
type Writer struct {
	format.BaseFormatWriter
	cfg        *Config
	groupStack []string
	// layerDepth tracks layer nesting. A source format may emit nested layers
	// (e.g. openxml's root document layer plus body/header/footer sub-document
	// layers); DocLang has a single document root, so the root element is
	// emitted only at the outermost boundary and inner layers are flattened.
	layerDepth int
	// tableCaption buffers caption text inside the current <table>/<index>. The
	// schema's element_head permits at most one <caption>, but a Docling table
	// can carry several caption refs; we fold them into a single <caption>
	// emitted before the first cell (flushTableCaption).
	tableCaption []string
	// threadOf maps a block ID to the <thread> id it shares with the fragments
	// it continues from / to (assigned in assignThreads). 0 = not threaded.
	threadOf map[string]int
	// tbl is the OTSL emission state for the table/index currently open, or nil.
	tbl *tableCtx
}

// NewWriter creates a new DocLang writer.
func NewWriter() *Writer {
	cfg := &Config{}
	cfg.Reset()
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{FormatName: "doclang"},
		cfg:              cfg,
		threadOf:         map[string]int{},
	}
}

// roleElem maps a normalized SemanticRole to its DocLang block element name and
// whether the element carries a level attribute.
func roleElem(role string) (elem string, hasLevel bool) {
	switch role {
	case model.RoleHeading, model.RoleTitle:
		// DocLang has no <title> body element (a <title> is only legal inside
		// <head>); a document title projects to a level-1 <heading>, matching
		// the HTML projection (RoleTitle → <h1>).
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
	case model.RoleFieldHeading:
		return "field_heading", true
	case model.RoleKey:
		return "key", false
	case model.RoleValue:
		return "value", false
	case model.RoleHint:
		return "hint", false
	case model.RoleMarker:
		return "marker", false
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
	"fmt:bidi":          "rtl",
	"fmt:handwriting":   "handwriting",
}

var bodyEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

// Write consumes Parts and serializes a DocLang document.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	var all []*model.Part
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				w.assignThreads(all)
				var b strings.Builder
				for _, p := range all {
					w.render(&b, p)
				}
				_, err := w.Output.Write([]byte(b.String()))
				return err
			}
			all = append(all, part)
		}
	}
}

// render serializes one Part into the document builder.
func (w *Writer) render(b *strings.Builder, part *model.Part) {
	switch part.Type {
	case model.PartLayerStart:
		if w.layerDepth == 0 {
			b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
			fmt.Fprintf(b, "<doclang xmlns=%q version=\"0.6\">\n", Namespace)
		}
		w.layerDepth++
	case model.PartLayerEnd:
		if w.layerDepth > 0 {
			w.layerDepth--
		}
		if w.layerDepth == 0 {
			b.WriteString("</doclang>\n")
		}
	case model.PartGroupStart:
		if g, ok := part.Resource.(*model.GroupStart); ok {
			w.openGroup(b, g)
		}
	case model.PartGroupEnd:
		w.closeGroup(b)
	case model.PartBlock:
		if blk, ok := part.Resource.(*model.Block); ok {
			// Inside a table row, buffer cells: spans are resolved at row close.
			if w.tbl != nil && w.parentGroup() == "table-row" {
				w.tbl.rowCells = append(w.tbl.rowCells, blk)
				return
			}
			w.writeBlock(b, blk)
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
	// A nested group/list that is a direct child of a <list> is the body of a
	// list_item and must be preceded by <ldiv/> (schema: list = element_head +
	// list_item*, each item starting with <ldiv/>). Text children get their
	// <ldiv/> in writeBlock; group/list/table children get it here.
	if w.parentGroup() == "list" {
		b.WriteString("<ldiv/>")
	}
	// A table/index caption must precede its cells; flush the buffered caption
	// before the first OTSL row opens.
	if g.Type == "table-row" {
		w.flushTableCaption(b)
	}
	w.groupStack = append(w.groupStack, g.Type)
	switch g.Type {
	case "list":
		class := g.Properties["class"]
		if class == "" {
			class = "unordered"
		}
		fmt.Fprintf(b, "<list class=%q>\n", class)
	case "table", "index":
		w.tbl = &tableCtx{}
		fmt.Fprintf(b, "<%s>\n", g.Type)
	case "group", "field_region", "field_item":
		fmt.Fprintf(b, "<%s>\n", g.Type)
	case "picture":
		if sub := g.Properties[model.PropPictureSubclass]; sub == "chart" || g.Properties["class"] == "chart" {
			b.WriteString("<picture class=\"chart\">\n")
		} else {
			b.WriteString("<picture>\n")
		}
		// A finer chart kind (bar_chart/pie_chart/…) rides as a <label>.
		if sub := g.Properties[model.PropPictureSubclass]; sub != "" && sub != "chart" && sub != "undefined" {
			fmt.Fprintf(b, "<label value=%q/>", sub)
		}
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
		w.emitOTSLRow(b)
	case "list", "group", "field_region", "field_item", "picture":
		fmt.Fprintf(b, "</%s>\n", t)
	case "table", "index":
		// A table/index with a caption but no rows still emits its caption.
		w.flushTableCaption(b)
		fmt.Fprintf(b, "</%s>\n", t)
		w.tbl = nil
	}
}

// flushTableCaption emits the buffered caption(s) of the current table/index as
// a single <caption> (the schema permits at most one), then clears the buffer.
func (w *Writer) flushTableCaption(b *strings.Builder) {
	if len(w.tableCaption) == 0 {
		return
	}
	fmt.Fprintf(b, "<caption>%s</caption>\n", strings.Join(w.tableCaption, " "))
	w.tableCaption = nil
}

// typeToRole bridges a few common Block.Type values to a normalized role, so a
// source format that records a kind on Block.Type but does not set the
// SemanticRole standoff layer still projects to the right DocLang element.
var typeToRole = map[string]string{
	"code-block": model.RoleCode,
	"code":       model.RoleCode,
	"codeblock":  model.RoleCode,
	"pre":        model.RoleCode,
}

// formulaLaTeX returns bare LaTeX for a formula block sourced from a foreign
// format — a placeholder run carrying it in Disp. Empty when the block already
// holds the LaTeX as text (native doclang), so the normal body is used.
func formulaLaTeX(blk *model.Block) string {
	for _, r := range blk.Source {
		if r.Ph != nil && r.Ph.Disp != "" {
			return r.Ph.Disp
		}
	}
	return ""
}

func (w *Writer) writeBlock(b *strings.Builder, blk *model.Block) {
	// "omml-nor" blocks are an OpenXML equation's translatable prose spans
	// (surfaced for docx write-back); the prose is already in the formula's
	// LaTeX, so skip them in cross-format output to avoid duplication.
	if blk.Type == "omml-nor" {
		return
	}
	role := blk.SemanticRole()
	if role == "" {
		role = typeToRole[blk.Type]
	}

	// A <table>/<index> may contain only element-head content + OTSL cell
	// tokens — never a body <text>/<heading>. A caption block is buffered and
	// emitted as a single <caption> in the element head (flushTableCaption);
	// any other (non-caption) block directly under a table/index is dropped
	// rather than emitted as illegal content. No reader produces the latter.
	if pg := w.parentGroup(); pg == "table" || pg == "index" {
		if role == model.RoleCaption {
			w.tableCaption = append(w.tableCaption, renderXMLBody(w.blockRuns(blk)))
		}
		return
	}

	// A checkbox is an empty selection control, not translatable text.
	if role == model.RoleCheckbox {
		if w.parentGroup() == "list" {
			b.WriteString("<ldiv/>")
		}
		class := "unselected"
		if blk.CheckboxChecked() {
			class = "selected"
		}
		fmt.Fprintf(b, "<checkbox class=%q/>\n", class)
		return
	}

	body := renderXMLBody(w.blockRuns(blk))
	elem, hasLevel := roleElem(role)

	// A RoleFormula block sourced from a foreign format (e.g. OpenXML OMML)
	// carries its math as a placeholder run whose Disp holds bare LaTeX. DocLang
	// mandates LaTeX inside <formula> (no $…$ wrapping), so use that bare LaTeX
	// for the body rather than the markdown-wrapped Equiv. Native doclang formula
	// blocks hold the LaTeX as text and keep the normal body.
	if role == model.RoleFormula {
		if latex := formulaLaTeX(blk); latex != "" {
			body = bodyEscaper.Replace(latex)
		}
	}

	// Every direct child of a <list> is a list_item and MUST begin with
	// <ldiv/> (per the schema's list content model), whatever its role.
	if w.parentGroup() == "list" {
		b.WriteString("<ldiv/>")
	}

	openAttrs := ""
	if hasLevel {
		level := 1
		if s, ok := blk.Structure(); ok && s != nil && s.Level > 0 {
			level = s.Level
		}
		openAttrs = fmt.Sprintf(" level=\"%d\"", level)
	}
	// A fillable value field carries class="fillable" (default read_only).
	if role == model.RoleValue && blk.FieldFillable() {
		openAttrs += " class=\"fillable\""
	}
	fmt.Fprintf(b, "<%s%s>", elem, openAttrs)
	w.writeHead(b, blk)
	b.WriteString(body)
	fmt.Fprintf(b, "</%s>\n", elem)
}

// writeHead emits the element-head properties we model, in DocLang's fixed
// element_head order: <label> (code language), <thread> (continuation), <layer>,
// then the 4-value <location> block (geometry).
func (w *Writer) writeHead(b *strings.Builder, blk *model.Block) {
	// Code language → recommended Linguist <label> (DocLang Recommendations), for
	// code blocks only, from the canonical code.language convention.
	if blk.SemanticRole() == model.RoleCode || typeToRole[blk.Type] == model.RoleCode {
		if lang := blk.CodeLanguage(); lang != "" {
			fmt.Fprintf(b, "<label value=%q/>", lang)
		}
	}
	if id := w.threadOf[blk.ID]; id > 0 {
		fmt.Fprintf(b, "<thread thread_id=\"%d\"/>", id)
	}
	if layer := blk.LayoutLayer(); layer != "" && layer != model.LayerBody {
		fmt.Fprintf(b, "<layer value=%q/>", layer)
	}
	if !w.cfg.EmitGeometry {
		return
	}
	g, ok := blk.Geometry()
	if !ok || g == nil || (g.BBox == model.Rect{}) {
		return // page-only geometry carries no <location> block
	}
	x0 := int(g.BBox.X)
	y0 := int(g.BBox.Y)
	x1 := int(g.BBox.X + g.BBox.W)
	y1 := int(g.BBox.Y + g.BBox.H)
	resX, resY := g.Resolution, g.ResolutionY
	if resY == 0 {
		resY = resX
	}
	// An asymmetric grid must spell out BOTH axes (even a default 512), or the
	// reader would infer the missing axis from the other and lose the asymmetry.
	force := resX != resY
	// The 4 <location> values alternate axes: x0, y0, x1, y1.
	w.writeLocation(b, x0, resX, force)
	w.writeLocation(b, y0, resY, force)
	w.writeLocation(b, x1, resX, force)
	w.writeLocation(b, y1, resY, force)
}

// writeLocation emits one <location> value, attaching resolution when the axis
// grid is non-default (DocLang's default_resolution is 512) or when force is set
// (an asymmetric grid, where the default must be made explicit to round-trip).
func (w *Writer) writeLocation(b *strings.Builder, value, res int, force bool) {
	if res != 0 && (force || res != 512) {
		fmt.Fprintf(b, "<location value=\"%d\" resolution=\"%d\"/>", value, res)
		return
	}
	fmt.Fprintf(b, "<location value=\"%d\"/>", value)
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

// --- OTSL span-aware table emission ---

// tableCtx tracks merged-cell span state across the rows of one table/index.
type tableCtx struct {
	rowCells []*model.Block // cells of the row currently being assembled
	pending  []pendSpan     // remaining rowspans per absolute column
}

// pendSpan is a rowspan still occupying a column in rows below its origin.
type pendSpan struct {
	rows   int  // rows of the span still to be covered
	origin bool // true at the span's origin column (→ ucel), false in its colspan tail (→ xcel)
}

func (t *tableCtx) pendingAt(col int) bool {
	return col < len(t.pending) && t.pending[col].rows > 0
}

func (t *tableCtx) setPending(col, rows int, origin bool) {
	for len(t.pending) <= col {
		t.pending = append(t.pending, pendSpan{})
	}
	t.pending[col] = pendSpan{rows: rows, origin: origin}
}

// roleToOTSLToken maps a cell block's role + header sub-kind to its OTSL token.
func roleToOTSLToken(blk *model.Block) string {
	if blk.SemanticRole() != model.RoleTableHeader {
		return "fcel"
	}
	switch blk.TableHeaderKind() {
	case model.TableHeaderColumn:
		return "ched"
	case model.TableHeaderRow:
		return "rhed"
	case model.TableHeaderCorner:
		return "corn"
	case model.TableHeaderSection:
		return "srow"
	default:
		return "ched"
	}
}

// emitOTSLRow renders the buffered cells of the current row, interleaving
// span-continuation tokens (ucel/xcel) for rowspans inherited from rows above
// and lcel for each cell's own colspan, then the <nl/> row terminator.
func (w *Writer) emitOTSLRow(b *strings.Builder) {
	t := w.tbl
	cells := t.rowCells
	t.rowCells = nil

	col, ci := 0, 0
	for ci < len(cells) || t.pendingAt(col) {
		if t.pendingAt(col) {
			if t.pending[col].origin {
				b.WriteString("<ucel/>")
			} else {
				b.WriteString("<xcel/>")
			}
			t.pending[col].rows--
			col++
			continue
		}
		cell := cells[ci]
		ci++
		colSpan, rowSpan := 1, 1
		if s, ok := cell.Structure(); ok && s != nil {
			if s.ColSpan > 1 {
				colSpan = s.ColSpan
			}
			if s.RowSpan > 1 {
				rowSpan = s.RowSpan
			}
		}
		fmt.Fprintf(b, "<%s/>%s", roleToOTSLToken(cell), renderXMLBody(w.blockRuns(cell)))
		for k := 1; k < colSpan; k++ {
			b.WriteString("<lcel/>")
		}
		if rowSpan > 1 {
			t.setPending(col, rowSpan-1, true)
			for k := 1; k < colSpan; k++ {
				t.setPending(col+k, rowSpan-1, false)
			}
		}
		col += colSpan
	}
	b.WriteString("<nl/>\n")
}

// --- thread assignment ---

// assignThreads scans the drained Part stream for RelContinues edges and assigns
// a shared <thread> id to every block in each continuation component (the origin
// plus its continuations), so the round-trip re-links them. Components of one
// block (no continuation) get no id.
func (w *Writer) assignThreads(parts []*model.Part) {
	uf := newUnionFind()
	seen := map[string]bool{}
	for _, p := range parts {
		blk, ok := p.Resource.(*model.Block)
		if !ok || p.Type != model.PartBlock {
			continue
		}
		seen[blk.ID] = true
		rel, has := blk.Relations()
		if !has || rel == nil {
			continue
		}
		for _, e := range rel.Relations {
			if e.Type == model.RelContinues && e.Target != "" {
				uf.union(blk.ID, e.Target)
			}
		}
	}
	// Group members by root, then assign ids to multi-member components in a
	// deterministic order (sorted roots) so output is stable.
	groups := map[string][]string{}
	for id := range seen {
		if uf.has(id) {
			root := uf.find(id)
			groups[root] = append(groups[root], id)
		}
	}
	roots := make([]string, 0, len(groups))
	for root := range groups {
		roots = append(roots, root)
	}
	sort.Strings(roots)
	next := 1
	for _, root := range roots {
		if len(groups[root]) < 2 {
			continue
		}
		for _, id := range groups[root] {
			w.threadOf[id] = next
		}
		next++
	}
}

// unionFind is a tiny union-find over block IDs for grouping RelContinues edges.
type unionFind struct{ parent map[string]string }

func newUnionFind() *unionFind { return &unionFind{parent: map[string]string{}} }

func (u *unionFind) has(x string) bool { _, ok := u.parent[x]; return ok }

func (u *unionFind) find(x string) string {
	if _, ok := u.parent[x]; !ok {
		u.parent[x] = x
	}
	for u.parent[x] != x {
		u.parent[x] = u.parent[u.parent[x]]
		x = u.parent[x]
	}
	return x
}

func (u *unionFind) union(a, b string) {
	ra, rb := u.find(a), u.find(b)
	if ra != rb {
		u.parent[ra] = rb
	}
}
