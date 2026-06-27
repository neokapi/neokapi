package html

import (
	"fmt"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/projection"
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

// htmlInlineTag maps a canonical inline run Type to its HTML open/close tags.
// This is the HTML format's own projection of the shared canonical type
// vocabulary (core/model/vocabularies); link:hyperlink and media:image are
// handled separately in renderInlineHTML because they carry attributes.
// TestHTMLInlineTagCoverage asserts this table covers every attribute-free
// formatting type the vocabulary defines, so a newly added type cannot silently
// fall through to plain text. The tags are the semantic document forms
// (<strong>/<em>), distinct from the terser <b>/<i> the vocabulary's `html`
// rendering uses for the MT wire (core/model/run_semantic_html.go).
var htmlInlineTag = map[string][2]string{
	"fmt:bold":          {"<strong>", "</strong>"},
	"fmt:italic":        {"<em>", "</em>"},
	"fmt:underline":     {"<u>", "</u>"},
	"fmt:strikethrough": {"<s>", "</s>"},
	"fmt:highlight":     {"<mark>", "</mark>"},
	"fmt:code":          {"<code>", "</code>"},
	"fmt:superscript":   {"<sup>", "</sup>"},
	"fmt:subscript":     {"<sub>", "</sub>"},
	"fmt:bidi":          {`<bdi dir="rtl">`, "</bdi>"},
	"fmt:handwriting":   {`<span class="handwriting">`, "</span>"},
}

// htmlAttr renders ` name="value"` (HTML-escaped) for a non-empty value, or ""
// when the value is empty — so absent attributes leave no trace in the markup.
func htmlAttr(name, val string) string {
	if val == "" {
		return ""
	}
	return " " + name + `="` + html.EscapeString(val) + `"`
}

// semanticState tracks the open container stack while emitting.
type semanticState struct {
	stack    []string // open HTML container tags ("ul"/"ol"/"table"/"tr"/"figure"/"" for transparent)
	autoList bool     // an auto-opened <ul> wrapping bare list-item blocks (no list group)

	// Table grid tracking for column-gap padding. CSV cells carry an explicit
	// "column" property and skip empty cells, so a row can have holes; the
	// first row establishes the width and later rows pad missing columns with
	// empty cells to keep the grid rectangular.
	tableCols int // column count from the first row; 0 until known
	rowsSeen  int // table-rows opened in the current table
	colCursor int // next column index within the current row
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

// indentDepth is the number of indentation levels for a block placed on its own
// line at the current point in the stream. Only block-level containers
// (lists, tables, figures) increase it; a transparent group ("") and a table
// row ("tr", whose cells stay inline on the row's line) do not.
func (s *semanticState) indentDepth() int {
	d := 0
	for _, t := range s.stack {
		switch t {
		case "ul", "ol", "table", "figure":
			d++
		}
	}
	return d
}

func (w *Writer) writeSemantic(events []*model.Part, sourceLocale model.LocaleID) error {
	// The cross-format export path projects a foreign document (CSV, DocLang,
	// Docling, DOCX, …) to clean HTML. The role/group stream carries only
	// body-level content — no <html>/<head>/<body> scaffold ever appears in it
	// — so wrap the rendered body in a minimal, valid HTML5 document. (DocLang's
	// writer emits its own <?xml?>/<doclang> root for the same reason; Markdown
	// needs no wrapper.) Without this, `kconv x.csv --to html` produced a bare
	// fragment with no document element.
	if err := w.openDocument(events, sourceLocale); err != nil {
		return err
	}

	st := &semanticState{}
	for i := 0; i < len(events); i++ {
		part := events[i]
		switch part.Type {
		case model.PartGroupStart:
			g, ok := part.Resource.(*model.GroupStart)
			if !ok {
				continue
			}
			// A table/index is rendered as a unit from the shared assembly
			// (rows of cells + lead caption), so its table-row groups and cells
			// are consumed here and the loop skips to the matching GroupEnd.
			if g.Type == "table" || g.Type == "index" {
				end, err := w.renderTableSemantic(st, events, i)
				if err != nil {
					return err
				}
				i = end
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
	return w.closeDocument()
}

// openDocument writes the HTML5 document scaffold up to (and including) the
// opening <body> tag. The <html> element carries a lang attribute derived from
// the writer's target locale (when retargeting) or the document's declared
// source locale; <title> is taken from the document's first title/heading block
// so the output is a self-describing standalone page.
func (w *Writer) openDocument(events []*model.Part, sourceLocale model.LocaleID) error {
	lang := sourceLocale
	if !w.Locale.IsEmpty() {
		lang = w.Locale
	}
	openHTML := "<html>"
	if !lang.IsEmpty() {
		openHTML = `<html lang="` + html.EscapeString(lang.String()) + `">`
	}
	title := html.EscapeString(documentTitle(events))
	return w.print("<!DOCTYPE html>\n" + openHTML + "\n<head>\n<meta charset=\"utf-8\">\n<title>" + title + "</title>\n</head>\n<body>\n")
}

// closeDocument writes the closing </body></html> tags that balance
// openDocument. In pretty mode each body block already ends in a newline, so the
// closing tags follow directly; in compact mode the body content has no trailing
// newline, so one is inserted to keep </body> on its own line.
func (w *Writer) closeDocument() error {
	if w.prettySemantic() {
		return w.print("</body>\n</html>\n")
	}
	return w.print("\n</body>\n</html>\n")
}

// documentTitle returns the page title for the exported document: the plain
// text of the first title or top-level heading block, or "Document" when the
// content has no heading. Inline formatting is flattened to text since <title>
// holds character data only.
func documentTitle(events []*model.Part) string {
	for _, part := range events {
		if part.Type != model.PartBlock {
			continue
		}
		b, ok := part.Resource.(*model.Block)
		if !ok {
			continue
		}
		switch b.SemanticRole() {
		case model.RoleTitle, model.RoleHeading:
			if t := strings.TrimSpace(b.SourceText()); t != "" {
				return t
			}
		}
	}
	return "Document"
}

// renderTableSemantic renders a whole <table> from the shared assembly
// (projection.AssembleTable). It drives the same openSemGroup / emitBlock /
// closeSemGroup the streamed path uses — opening the real table group, emitting
// any lead caption, then a synthetic table-row group per assembled row with its
// cells — so the column-padding, indentation and <th>/<td> output are identical;
// only the row/cell structure now comes from one assembler. Returns the index of
// the table's GroupEnd.
func (w *Writer) renderTableSemantic(st *semanticState, events []*model.Part, start int) (end int, err error) {
	tableG, _ := events[start].Resource.(*model.GroupStart)
	if err = w.openSemGroup(st, tableG); err != nil {
		return start, err
	}
	end, table := projection.AssembleTable(events, start)
	for _, b := range table.Lead {
		if err = w.emitBlock(st, b); err != nil {
			return end, err
		}
	}
	rowG := &model.GroupStart{Type: "table-row"}
	for _, row := range table.Rows {
		if err = w.openSemGroup(st, rowG); err != nil {
			return end, err
		}
		for _, c := range row.Cells {
			if err = w.emitBlock(st, c.Block); err != nil {
				return end, err
			}
		}
		if err = w.closeSemGroup(st); err != nil { // closes the <tr>
			return end, err
		}
	}
	if err = w.closeSemGroup(st); err != nil { // closes the <table>
		return end, err
	}
	return end, nil
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
		st.tableCols = 0
		st.rowsSeen = 0
	case "table-row":
		tag = "tr"
		st.rowsSeen++
		st.colCursor = 0
	case "picture":
		tag = "figure"
	default:
		tag = "" // transparent grouping (group/field_region/…)
	}
	// Emit the open tag at the parent's depth (before the new container is
	// pushed), so its children indent one level deeper. A table row keeps its
	// cells inline on the same line; other containers open on their own line.
	switch tag {
	case "":
		// transparent grouping — no markup
	case "tr":
		if err := w.semOpenInline(st, "<tr>"); err != nil {
			return err
		}
	default:
		if err := w.semLine(st, "<"+tag+">"); err != nil {
			return err
		}
	}
	st.stack = append(st.stack, tag)
	return nil
}

func (w *Writer) closeSemGroup(st *semanticState) error {
	if len(st.stack) == 0 {
		return nil
	}
	tag := st.stack[len(st.stack)-1]
	st.stack = st.stack[:len(st.stack)-1]
	if tag == "tr" {
		// Pad any trailing columns missing from this row, then close it. The
		// first row establishes the table width. Padding cells stay inline.
		if st.tableCols > 0 {
			for st.colCursor < st.tableCols {
				if err := w.print("<td></td>"); err != nil {
					return err
				}
				st.colCursor++
			}
		}
		if err := w.semCloseInline("</tr>"); err != nil {
			return err
		}
		if st.rowsSeen == 1 && st.tableCols == 0 {
			st.tableCols = st.colCursor
		}
		return nil
	}
	if tag != "" {
		// The container is already popped, so the close tag lines up with its
		// matching open tag at the parent's depth.
		return w.semLine(st, "</"+tag+">")
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
		return w.semLine(st, "</ul>")
	}
	return nil
}

func (w *Writer) emitBlock(st *semanticState, b *model.Block) error {
	// Same-format HTML fallback: a fragment-based skeleton carries the block's
	// own surrounding markup — emit it verbatim, content spliced at the ref. The
	// assembled fragment is placed on its own line (treated atomically: its
	// captured whitespace is preserved as-is).
	if b.Skeleton != nil && b.Skeleton.Strategy == model.SkeletonFragmentBased {
		if err := w.closeAutoList(st); err != nil {
			return err
		}
		text := w.getBlockText(b)
		var frag strings.Builder
		for _, sp := range b.Skeleton.Parts {
			switch p := sp.(type) {
			case *model.SkeletonText:
				frag.WriteString(p.Text)
			case *model.SkeletonRef:
				frag.WriteString(text)
			}
		}
		return w.semLine(st, frag.String())
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
				if err := w.semLine(st, "<ul>"); err != nil {
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
		level := min(max(b.HeadingLevel(), 1), 6)
		return w.semLine(st, fmt.Sprintf("<h%d>%s</h%d>", level, body, level))
	case model.RoleCode:
		openTag := "<code>"
		if lang := b.CodeLanguage(); lang != "" {
			openTag = `<code class="language-` + html.EscapeString(lang) + `">`
		}
		// Emitted atomically: the indent precedes <pre>, but the code body's own
		// (significant) whitespace inside <pre>…</pre> is never touched.
		return w.semLine(st, "<pre>"+openTag+body+"</code></pre>")
	case model.RoleCaption:
		tag := "figcaption"
		if st.top() == "table" {
			tag = "caption"
		}
		return w.semLine(st, "<"+tag+">"+body+"</"+tag+">")
	case model.RoleTableCell, model.RoleTableHeader:
		if st.inContainer("table") || st.inContainer("tr") {
			// Pad the gap before this cell when it carries an explicit column
			// index (CSV skips empty cells), so the grid stays aligned. Cells
			// stay inline on their row's line.
			if col, ok := cellColumn(b); ok {
				for st.colCursor < col {
					if err := w.print("<td></td>"); err != nil {
						return err
					}
					st.colCursor++
				}
			}
			cell := "td"
			if role == model.RoleTableHeader {
				cell = "th"
			}
			// Merged cells carry their extent as colspan / rowspan; advance the
			// column cursor by the column span so following cells stay aligned.
			attrs, colSpan := cellSpanAttrs(b)
			st.colCursor += colSpan
			return w.print("<" + cell + attrs + ">" + body + "</" + cell + ">")
		}
		return w.semLine(st, "<p>"+body+"</p>") // bare cell, no table context
	default:
		tag := blockRoleTag[role]
		if tag == "" {
			tag = "p"
		}
		return w.semLine(st, "<"+tag+">"+body+"</"+tag+">")
	}
}

// cellColumn returns a table cell's explicit 0-based column index from its
// "column" property, or (0, false) when none is set (e.g. DocLang/Docling
// cells, which are emitted sequentially with no holes).
func cellColumn(b *model.Block) (int, bool) {
	v, ok := b.Properties["column"]
	if !ok {
		return 0, false
	}
	n := 0
	if _, err := fmt.Sscanf(v, "%d", &n); err != nil {
		return 0, false
	}
	return n, true
}

// cellSpanAttrs returns the colspan / rowspan attribute string for a merged
// table cell (empty when unmerged) and the cell's column span (≥1), used to
// advance the column cursor.
func cellSpanAttrs(b *model.Block) (attrs string, colSpan int) {
	colSpan = 1
	s, ok := b.Structure()
	if !ok || s == nil {
		return "", colSpan
	}
	if s.ColSpan > 1 {
		colSpan = s.ColSpan
		attrs += fmt.Sprintf(` colspan="%d"`, s.ColSpan)
	}
	if s.RowSpan > 1 {
		attrs += fmt.Sprintf(` rowspan="%d"`, s.RowSpan)
	}
	return attrs, colSpan
}

// headingLevel returns a block's heading level from the structural annotation,
// falling back to the legacy "level" property; 0 when neither is present.

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
	sink := &htmlInlineSink{}
	projection.WalkInline(runs, sink)
	sink.flush()
	return sink.sb.String()
}

// htmlInlineSink maps the shared inline-run stream (projection.WalkInline) to
// HTML, owning the open-tag stack the paired-code close behavior needs. It
// replaces the writer's former bespoke run loop; WalkInline now handles run
// decoding + plural/select 'other'-branch resolution.
type htmlInlineSink struct {
	sb   strings.Builder
	open []string // stack of emitted closing tags (or "" for dropped)
}

func (s *htmlInlineSink) Text(t string) { s.sb.WriteString(html.EscapeString(t)) }

func (s *htmlInlineSink) Open(r *model.PcOpenRun) {
	switch r.Type {
	case "link:hyperlink":
		s.sb.WriteString("<a" + htmlAttr("href", r.Attr(model.AttrHref)) + htmlAttr("title", r.Attr(model.AttrTitle)) + ">")
		s.open = append(s.open, "</a>")
	case "media:image", "link:image":
		// Paired image (e.g. read from Markdown): the alt text is the content.
		// Open the alt attribute; the content fills it; the matching close
		// finishes the void element.
		s.sb.WriteString("<img" + htmlAttr("src", r.Attr(model.AttrSrc)) + ` alt="`)
		s.open = append(s.open, `"`+htmlAttr("title", r.Attr(model.AttrTitle))+"/>")
	default:
		if tag, ok := htmlInlineTag[r.Type]; ok {
			s.sb.WriteString(tag[0])
			s.open = append(s.open, tag[1])
		} else if strings.HasPrefix(strings.TrimSpace(r.Data), "<") {
			s.sb.WriteString(r.Data)
			s.open = append(s.open, "") // closing comes from the matching PcClose Data
		} else {
			s.open = append(s.open, "")
		}
	}
}

func (s *htmlInlineSink) Close(r *model.PcCloseRun) {
	if n := len(s.open); n > 0 {
		closer := s.open[n-1]
		s.open = s.open[:n-1]
		if closer != "" {
			s.sb.WriteString(closer)
		} else if strings.HasPrefix(strings.TrimSpace(r.Data), "<") {
			s.sb.WriteString(r.Data)
		}
	}
}

func (s *htmlInlineSink) Placeholder(r *model.PlaceholderRun) {
	switch r.Type {
	case "media:image", "link:image":
		// Self-closing image (e.g. read from HTML <img>): alt lives in the run
		// attributes, not as paired content.
		s.sb.WriteString("<img" +
			htmlAttr("src", r.Attr(model.AttrSrc)) +
			htmlAttr("alt", r.Attr(model.AttrAlt)) +
			htmlAttr("title", r.Attr(model.AttrTitle)) + "/>")
	default:
		if r.Equiv != "" {
			s.sb.WriteString(html.EscapeString(r.Equiv))
		}
	}
}

// flush emits any trailing unclosed tags (defensive — well-formed runs balance).
func (s *htmlInlineSink) flush() {
	for i := len(s.open) - 1; i >= 0; i-- {
		if s.open[i] != "" {
			s.sb.WriteString(s.open[i])
		}
	}
}

// print writes a string to the writer's output.
func (w *Writer) print(s string) error {
	_, err := fmt.Fprint(w.Output, s)
	return err
}

// prettySemantic reports whether the semantic export path pretty-prints
// (indents block elements onto their own lines). On by default.
func (w *Writer) prettySemantic() bool {
	return w.cfg == nil || !w.cfg.CompactOutput
}

// indentUnit is the per-level indent string for pretty-printed semantic output.
func (w *Writer) indentUnit() string {
	if w.cfg != nil && w.cfg.IndentString != "" {
		return w.cfg.IndentString
	}
	return "  "
}

// semLine emits one block-level element on its own line, indented to the
// current structural depth, with a trailing newline. In compact mode it writes
// the element verbatim with no surrounding whitespace (legacy single-line
// output). The element string is treated atomically — any whitespace inside it
// (e.g. the significant newlines of a <pre> code block) is left untouched.
func (w *Writer) semLine(st *semanticState, s string) error {
	if !w.prettySemantic() {
		return w.print(s)
	}
	return w.print(strings.Repeat(w.indentUnit(), st.indentDepth()) + s + "\n")
}

// semOpenInline emits an open tag whose children stay inline on the same line
// (a table row): indented, but with no trailing newline.
func (w *Writer) semOpenInline(st *semanticState, s string) error {
	if !w.prettySemantic() {
		return w.print(s)
	}
	return w.print(strings.Repeat(w.indentUnit(), st.indentDepth()) + s)
}

// semCloseInline closes an inline-children element (a table row): the close tag
// trails its inline content directly, then a newline ends the line.
func (w *Writer) semCloseInline(s string) error {
	if !w.prettySemantic() {
		return w.print(s)
	}
	return w.print(s + "\n")
}
