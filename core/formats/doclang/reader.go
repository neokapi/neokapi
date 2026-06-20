// Package doclang reads and writes DocLang (the LF AI & Data open standard for
// AI-native documents, https://doclang.ai), pinned to spec v0.6.
//
// DocLang is the standardized XML serialization of Docling's structured output:
// semantic role + layout layer + page geometry + reading structure. neokapi
// consumes it as a structured-ingestion input (the "consume Docling" boundary)
// and can re-emit it (faithful DocLang↔DocLang; native→DocLang projection),
// mapping DocLang's elements onto the content model + the WS1 structural layer
// (SemanticRole, GeometryAnnotation, LayoutLayer — see core/model/structure.go).
//
// Coverage (v0.6): document/head; the block-level semantic elements heading,
// text, footnote, page_header, page_footer, code, formula, marker; the forms
// cluster field_region/field_heading/field_item/key/value/hint/checkbox (with
// fillable + checked state); list (+ ldiv items); group/picture (picture
// subclass from class/<label>); table/index (OTSL cells fcel/ched/rhed/ecel/
// srow/corn with header sub-kinds, row terminator nl, and merged-cell spans
// lcel/ucel/xcel → ColSpan/RowSpan); the element-head properties layer, the
// 4-value location block (→ geometry, per-axis resolution), <label> (→ code
// language), and <thread> (→ RelContinues continuation edges); <page_break/>
// (→ GeometryAnnotation.Page); and inline formatting (bold/italic/underline/
// strikethrough/superscript/subscript/rtl/handwriting). Not mapped (read-through,
// declared in spec.yaml): xref/href continuity and nested table-cell semantics —
// skipped without losing surrounding content.
package doclang

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Namespace is the version-major-scoped DocLang namespace (v0.x).
const Namespace = "https://www.doclang.ai/ns/v0"

// blockRole maps a DocLang block-level element to a normalized SemanticRole.
var blockRole = map[string]string{
	"heading":       model.RoleHeading,
	"text":          model.RoleParagraph,
	"footnote":      model.RoleFootnote,
	"page_header":   model.RolePageHeader,
	"page_footer":   model.RolePageFooter,
	"code":          model.RoleCode,
	"formula":       model.RoleFormula,
	"marker":        model.RoleMarker,
	"key":           model.RoleKey,
	"value":         model.RoleValue,
	"hint":          model.RoleHint,
	"field_heading": model.RoleFieldHeading,
}

// containerElem maps a DocLang container element (wraps other semantics, emitted
// as a neokapi Group) to its normalized role. Tables/index are handled
// separately (OTSL). "" = no normalized role for the group.
var containerElem = map[string]string{
	"list":         model.RoleList,
	"group":        model.RoleSection,
	"field_region": model.RoleFieldRegion,
	"field_item":   model.RoleFieldItem,
	"picture":      model.RolePicture,
}

// headElem are element-head property elements (precede the body). label, thread
// and location carry data the reader extracts in parseBlock; the rest skip.
var headElem = map[string]bool{
	"xref": true, "href": true, "caption": true, "custom": true,
}

// fmtTag maps a DocLang inline formatting element to its run vocabulary type.
var fmtTag = map[string]string{
	"bold":          "fmt:bold",
	"italic":        "fmt:italic",
	"underline":     "fmt:underline",
	"strikethrough": "fmt:strikethrough",
	"superscript":   "fmt:superscript",
	"subscript":     "fmt:subscript",
	"rtl":           "fmt:bidi",
	"handwriting":   "fmt:handwriting",
}

// otslCellTok are the OTSL cell-starting tokens we map to a cell block + role.
var otslCellTok = map[string]string{
	"fcel": model.RoleTableCell,
	"ecel": model.RoleTableCell,
	"ched": model.RoleTableHeader,
	"rhed": model.RoleTableHeader,
	"srow": model.RoleTableHeader,
	"corn": model.RoleTableHeader,
}

// otslContinuation are the OTSL span-continuation tokens: a cell merged with the
// cell to its left (lcel), above (ucel), or both (xcel). They carry no content;
// they extend an origin cell's ColSpan/RowSpan.
var otslContinuation = map[string]bool{"lcel": true, "ucel": true, "xcel": true}

// otslHeaderKind maps an OTSL header cell token to its PropTableHeaderKind value.
var otslHeaderKind = map[string]string{
	"ched": model.TableHeaderColumn,
	"rhed": model.TableHeaderRow,
	"corn": model.TableHeaderCorner,
	"srow": model.TableHeaderSection,
}

// Reader implements DataFormatReader for DocLang documents.
type Reader struct {
	format.BaseFormatReader
	cfg          *Config
	blockCounter int
	groupCounter int
	currentPage  int            // 1-based page, advanced by <page_break/>
	threadFirst  map[int]string // thread_id -> the first block ID that declared it (for RelContinues)
}

// NewReader creates a new DocLang reader.
func NewReader() *Reader {
	cfg := &Config{}
	cfg.Reset()
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "doclang",
			FormatDisplayName: "DocLang",
			FormatMimeType:    "application/doclang+xml",
			FormatExtensions:  []string{".dclg.xml"},
			Cfg:               cfg,
		},
		cfg:         cfg,
		currentPage: 1,
		threadFirst: map[int]string{},
	}
}

// Signature returns detection metadata. The <doclang root check wins priority
// over the generic xml reader.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"application/doclang+xml"},
		Extensions: []string{".dclg.xml"},
		Sniff:      func(data []byte) bool { return strings.Contains(string(data), "<doclang") },
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("doclang: nil document or reader")
	}
	r.Doc = doc
	return nil
}

// Read returns a channel of PartResults.
func (r *Reader) Read(ctx context.Context) <-chan model.PartResult {
	ch := make(chan model.PartResult, 64)
	go func() {
		defer close(ch)
		r.readContent(ctx, ch)
	}()
	return ch
}

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	locale := r.locale()
	data, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		r.emitError(ch, fmt.Errorf("doclang: reading document: %w", err))
		return
	}

	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "doclang",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "application/doclang+xml",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	dec := xml.NewDecoder(strings.NewReader(string(data)))
	if err := advanceToRoot(dec); err != nil {
		if !errors.Is(err, io.EOF) {
			r.emitError(ch, fmt.Errorf("doclang: locating <doclang> root: %w", err))
		}
	} else if err := r.walkChildren(ctx, ch, dec, "doclang"); err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, context.Canceled) {
		r.emitError(ch, fmt.Errorf("doclang: parse: %w", err))
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// walkChildren reads tokens until the closing EndElement of the current
// container (or EOF for the document), dispatching each semantic element.
func (r *Reader) walkChildren(ctx context.Context, ch chan<- model.PartResult, dec *xml.Decoder, parent string) error {
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch el := tok.(type) {
		case xml.StartElement:
			if err := r.dispatchChild(ctx, ch, dec, el, parent); err != nil {
				return err
			}
		case xml.EndElement:
			return nil // closes the current container
		case xml.CharData:
			// whitespace between elements at container scope — ignore
		}
	}
}

// dispatchChild routes one already-read start element to the right handler. It
// is shared by walkChildren and parseContainer (which reads its element head
// first, then defers to this for the body).
func (r *Reader) dispatchChild(ctx context.Context, ch chan<- model.PartResult, dec *xml.Decoder, el xml.StartElement, parent string) error {
	name := el.Name.Local
	switch {
	case name == "page_break":
		// A page boundary: advance the page counter so subsequent blocks carry
		// GeometryAnnotation.Page = currentPage. The element is empty.
		r.currentPage++
		return dec.Skip()
	case name == "caption" && r.cfg.ExtractNonTranslatableContent:
		// Surface a figure/picture/container caption as a non-translatable
		// RoleCaption content block (visible to ingestion, skipped by MT) rather
		// than dropping it. The writer already round-trips RoleCaption.
		return r.parseBlock(ctx, ch, dec, el, model.RoleCaption, false)
	case name == "head" || headElem[name]:
		return dec.Skip()
	case name == "checkbox":
		return r.parseCheckbox(ctx, ch, dec, el)
	case parent == "list" && name == "ldiv":
		// <ldiv> is the list-item delimiter (holds the bullet/number marker); the
		// item content is the following <text>. Skip it.
		return dec.Skip()
	case name == "table" || name == "index":
		return r.parseTable(ctx, ch, dec, el)
	case containerElem[name] != "":
		return r.parseContainer(ctx, ch, dec, el)
	default: // block element (known role) or unknown → paragraph-ish
		role := blockRole[name]
		if parent == "list" && name == "text" {
			role = model.RoleListItem // a <text> inside a <list> is an item
		}
		return r.parseBlock(ctx, ch, dec, el, role, true)
	}
}

// containerHeadElem are element-head / payload-prefix elements that may precede
// a container's body content (DocLang element_head order + picture src/tabular).
// parseContainer consumes them before emitting the Group so their data
// (e.g. a picture's chart-kind <label>) can ride on the GroupStart.
var containerHeadElem = map[string]bool{
	"label": true, "thread": true, "xref": true, "href": true, "layer": true,
	"location": true, "caption": true, "custom": true, "src": true, "tabular": true,
}

// parseContainer emits a Group bracketing the element's children. It first
// drains the optional element head (recording a picture's subclass from its
// <label>/class), then dispatches the body via dispatchChild.
func (r *Reader) parseContainer(ctx context.Context, ch chan<- model.PartResult, dec *xml.Decoder, start xml.StartElement) error {
	r.groupCounter++
	gid := fmt.Sprintf("g%d", r.groupCounter)
	name := start.Name.Local
	props := map[string]string{}
	if class := attrValue(start, "class"); class != "" {
		props["class"] = class
		if name == "picture" && class != "undefined" {
			props[model.PropPictureSubclass] = class
		}
	}

	emitted := false
	ensure := func() bool {
		if emitted {
			return true
		}
		emitted = true
		return r.emit(ctx, ch, &model.Part{Type: model.PartGroupStart, Resource: &model.GroupStart{ID: gid, Name: name, Type: name, Properties: props}})
	}

	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch el := tok.(type) {
		case xml.StartElement:
			n := el.Name.Local
			if n == "caption" && r.cfg.ExtractNonTranslatableContent {
				// Surface the container's caption as a non-translatable RoleCaption
				// content block inside the group (visible to ingestion, skipped by
				// MT). Other head elements (e.g. a picture's <label> subclass) are
				// still drained below before the body.
				if !ensure() {
					return context.Canceled
				}
				if err := r.parseBlock(ctx, ch, dec, el, model.RoleCaption, false); err != nil {
					return err
				}
				continue
			}
			if !emitted && containerHeadElem[n] {
				// Picture chart kind rides in a leading <label value="bar_chart"/>;
				// it refines the bounded class subclass.
				if name == "picture" && n == "label" {
					if v := attrValue(el, "value"); v != "" && v != "undefined" {
						props[model.PropPictureSubclass] = v
					}
				}
				if err := dec.Skip(); err != nil {
					return err
				}
				continue
			}
			if !ensure() {
				return context.Canceled
			}
			if err := r.dispatchChild(ctx, ch, dec, el, name); err != nil {
				return err
			}
		case xml.EndElement:
			if !ensure() {
				return context.Canceled
			}
			r.emit(ctx, ch, &model.Part{Type: model.PartGroupEnd, Resource: &model.GroupEnd{ID: gid}})
			return nil
		case xml.CharData:
			// raw text directly inside a container is rare and non-translatable
			// scaffolding; ignore (cells/blocks carry the translatable text).
		}
	}
}

// parseCheckbox emits a non-translatable RoleCheckbox block carrying the
// selected/unselected state (DocLang <checkbox class="selected|unselected"/>).
func (r *Reader) parseCheckbox(ctx context.Context, ch chan<- model.PartResult, dec *xml.Decoder, start xml.StartElement) error {
	checked := attrValue(start, "class") == "selected"
	if err := dec.Skip(); err != nil { // empty element
		return err
	}
	r.blockCounter++
	b := model.NewRunsBlock(fmt.Sprintf("b%d", r.blockCounter), nil)
	b.SourceLocale = r.locale()
	b.Type = "checkbox"
	b.Translatable = false
	b.SetSemanticRole(model.RoleCheckbox, 0)
	b.SetCheckboxChecked(checked)
	if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: b}) {
		return context.Canceled
	}
	return nil
}

// parseBlock parses a block-level element: its element head (layer + location)
// and its inline body, emitting one PartBlock with role, geometry, and layer.
func (r *Reader) parseBlock(ctx context.Context, ch chan<- model.PartResult, dec *xml.Decoder, start xml.StartElement, role string, translatable bool) error {
	elem := start.Name.Local
	level := 0
	if elem == "heading" || elem == "field_heading" {
		if l := attrValue(start, "level"); l != "" {
			level, _ = strconv.Atoi(l)
		} else {
			level = 1
		}
	}

	var (
		runs     []model.Run
		layer    string
		locs     []locVal
		idCtr    int
		fmtOpen  []string // stack of open formatting tag names
		labelVal string   // <label value="..."> — fine subclass (code language, …)
		threadID int      // <thread thread_id="N"/> — continuation linking
	)
	depth := 1
	for depth > 0 {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		switch el := tok.(type) {
		case xml.StartElement:
			n := el.Name.Local
			switch {
			case n == "layer":
				layer = attrValueOr(el, "value", model.LayerBody)
				if err := dec.Skip(); err != nil {
					return err
				}
			case n == "location":
				v, _ := strconv.Atoi(attrValue(el, "value"))
				res, _ := strconv.Atoi(attrValue(el, "resolution"))
				locs = append(locs, locVal{value: v, resolution: res})
				if err := dec.Skip(); err != nil {
					return err
				}
			case n == "label":
				labelVal = attrValue(el, "value")
				if err := dec.Skip(); err != nil {
					return err
				}
			case n == "thread":
				threadID, _ = strconv.Atoi(attrValue(el, "thread_id"))
				if err := dec.Skip(); err != nil {
					return err
				}
			case headElem[n]:
				if err := dec.Skip(); err != nil {
					return err
				}
			case fmtTag[n] != "":
				idCtr++
				runs = append(runs, model.Run{PcOpen: &model.PcOpenRun{ID: strconv.Itoa(idCtr), Type: fmtTag[n], Data: "<" + n + ">"}})
				fmtOpen = append(fmtOpen, n)
				depth++
			default:
				depth++ // unknown nested element: descend to capture its text
			}
		case xml.EndElement:
			n := el.Name.Local
			if fmtTag[n] != "" && len(fmtOpen) > 0 && fmtOpen[len(fmtOpen)-1] == n {
				fmtOpen = fmtOpen[:len(fmtOpen)-1]
				runs = append(runs, model.Run{PcClose: &model.PcCloseRun{Data: "</" + n + ">"}})
			}
			depth--
		case xml.CharData:
			if s := string(el); s != "" {
				runs = append(runs, model.Run{Text: &model.TextRun{Text: s}})
			}
		}
	}

	runs = trimRunWhitespace(runs)
	if len(runs) == 0 {
		return nil
	}
	r.blockCounter++
	block := model.NewRunsBlock(fmt.Sprintf("b%d", r.blockCounter), runs)
	block.SourceLocale = r.locale()
	block.Type = elem
	block.Translatable = translatable
	if role != "" {
		block.SetSemanticRole(role, level)
	}
	if layer != "" && layer != model.LayerBody {
		block.SetLayoutLayer(layer)
	}
	r.attachGeometry(block, locs)
	// Fine subtype: a <code>'s <label value> is its Linguist language key (DNT
	// signal); other free <label> subclasses ride on the role for now.
	if elem == "code" && labelVal != "" && labelVal != "undefined" {
		block.SetCodeLanguage(labelVal)
	}
	// A <value class="fillable"> is an editable form field (vs read_only).
	if role == model.RoleValue && attrValue(start, "class") == "fillable" {
		block.SetFieldFillable(true)
	}
	// <thread thread_id> joins fragments of one logical flow split across boxes /
	// columns / pages: the first block to declare an id is the origin; later
	// blocks with the same id continue from it (RelContinues).
	if threadID > 0 {
		if prev, ok := r.threadFirst[threadID]; ok && prev != "" {
			block.AddRelation(model.RelContinues, prev)
		} else {
			r.threadFirst[threadID] = block.ID
		}
	}
	r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
	return nil
}

// attachGeometry sets the block's GeometryAnnotation from a 4-value <location>
// block (if present) and/or the current page. A page-only geometry is attached
// for content past the first <page_break/> even when the source carries no bbox.
func (r *Reader) attachGeometry(block *model.Block, locs []locVal) {
	g := geometryFrom(locs)
	if g == nil && r.currentPage > 1 {
		g = &model.GeometryAnnotation{}
	}
	if g == nil {
		return
	}
	g.Page = r.currentPage
	block.SetGeometry(g)
}

// otslTok is one cell of the OTSL token grid: either an origin cell token
// (fcel/ecel/ched/rhed/corn/srow, carrying content + role) or a span
// continuation (lcel/ucel/xcel, no content). Spans are resolved over the full
// grid before any block is emitted.
type otslTok struct {
	name string // the cell/continuation token name
	text string // content (origin cells only)
}

// parseTable handles an OTSL table/index. It first builds the full token grid
// (rows × columns, delimited by <nl/>), then resolves merged-cell spans over
// the grid (lcel = colspan continuation to the left, ucel = rowspan to the
// origin above, xcel = the 2D interior of a span), and finally emits a Group
// wrapping per-row Groups of cell Blocks, each origin carrying ColSpan/RowSpan
// and (for headers) its OTSL sub-kind.
func (r *Reader) parseTable(ctx context.Context, ch chan<- model.PartResult, dec *xml.Decoder, start xml.StartElement) error {
	grid, caption, err := r.scanOTSLGrid(dec)
	if err != nil {
		return err
	}

	r.groupCounter++
	tid := fmt.Sprintf("g%d", r.groupCounter)
	if !r.emit(ctx, ch, &model.Part{Type: model.PartGroupStart, Resource: &model.GroupStart{ID: tid, Name: start.Name.Local, Type: start.Name.Local}}) {
		return context.Canceled
	}

	// Surface the table/index caption as a non-translatable RoleCaption content
	// block inside the group (visible to ingestion, skipped by MT).
	if caption = strings.TrimSpace(caption); caption != "" && r.cfg.ExtractNonTranslatableContent {
		r.blockCounter++
		cb := model.NewBlock(fmt.Sprintf("b%d", r.blockCounter), caption)
		cb.SourceLocale = r.locale()
		cb.Type = "caption"
		cb.Translatable = false
		cb.SetSemanticRole(model.RoleCaption, 0)
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: cb}) {
			return context.Canceled
		}
	}

	at := func(rr, cc int) string {
		if rr >= 0 && rr < len(grid) && cc >= 0 && cc < len(grid[rr]) {
			return grid[rr][cc].name
		}
		return ""
	}
	for rr := range grid {
		var rowID string
		rowOpen := false
		for cc := range grid[rr] {
			t := grid[rr][cc]
			if otslContinuation[t.name] { // covered by a span origin elsewhere
				continue
			}
			txt := strings.TrimSpace(t.text)
			if txt == "" { // empty cell (ecel / blank) — nothing translatable
				continue
			}
			// colspan = 1 + consecutive lcel to the right; rowspan = 1 +
			// consecutive ucel directly below (the rectangular OTSL span rule).
			colSpan := 1
			for at(rr, cc+colSpan) == "lcel" || at(rr, cc+colSpan) == "xcel" {
				colSpan++
			}
			rowSpan := 1
			for at(rr+rowSpan, cc) == "ucel" || at(rr+rowSpan, cc) == "xcel" {
				rowSpan++
			}

			if !rowOpen {
				r.groupCounter++
				rowID = fmt.Sprintf("g%d", r.groupCounter)
				if !r.emit(ctx, ch, &model.Part{Type: model.PartGroupStart, Resource: &model.GroupStart{ID: rowID, Name: "table-row", Type: "table-row"}}) {
					return context.Canceled
				}
				rowOpen = true
			}

			r.blockCounter++
			b := model.NewBlock(fmt.Sprintf("b%d", r.blockCounter), txt)
			b.SourceLocale = r.locale()
			b.Type = "table-cell"
			role := otslCellTok[t.name]
			b.SetStructure(&model.StructureAnnotation{Role: role, ColSpan: spanOrZero(colSpan), RowSpan: spanOrZero(rowSpan)})
			if kind := otslHeaderKind[t.name]; kind != "" {
				b.SetTableHeaderKind(kind)
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: b}) {
				return context.Canceled
			}
		}
		if rowOpen {
			r.emit(ctx, ch, &model.Part{Type: model.PartGroupEnd, Resource: &model.GroupEnd{ID: rowID}})
		}
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartGroupEnd, Resource: &model.GroupEnd{ID: tid}})
	return nil
}

// scanOTSLGrid reads the OTSL token stream of the current table/index up to its
// closing EndElement, returning the row-major token grid. Cell content (text and
// the inner text of a <text> wrapper) attaches to the most recent origin cell.
func (r *Reader) scanOTSLGrid(dec *xml.Decoder) ([][]otslTok, string, error) {
	var grid [][]otslTok
	var row []otslTok
	var caption strings.Builder
	last := -1 // index of the current content-bearing cell in row
	flushRow := func() {
		grid = append(grid, row)
		row = nil
		last = -1
	}
	for {
		tok, err := dec.Token()
		if err != nil {
			return nil, "", err
		}
		switch el := tok.(type) {
		case xml.StartElement:
			n := el.Name.Local
			switch {
			case otslCellTok[n] != "":
				row = append(row, otslTok{name: n})
				last = len(row) - 1
				if err := dec.Skip(); err != nil {
					return nil, "", err
				}
			case otslContinuation[n]:
				row = append(row, otslTok{name: n})
				last = -1 // continuations carry no content
				if err := dec.Skip(); err != nil {
					return nil, "", err
				}
			case n == "nl":
				flushRow()
				if err := dec.Skip(); err != nil {
					return nil, "", err
				}
			case n == "text":
				if last >= 0 {
					row[last].text += readInnerText(dec)
				} else if err := dec.Skip(); err != nil {
					return nil, "", err
				}
			case n == "caption":
				// Capture the table/index caption so parseTable can surface it as
				// non-translatable content (it would otherwise be dropped here).
				caption.WriteString(readInnerText(dec))
			default: // element head / nested — skip
				if err := dec.Skip(); err != nil {
					return nil, "", err
				}
			}
		case xml.EndElement: // </table>/</index>
			if len(row) > 0 {
				flushRow() // a final row not terminated by <nl/>
			}
			return grid, caption.String(), nil
		case xml.CharData:
			if last >= 0 {
				row[last].text += string(el)
			}
		}
	}
}

// spanOrZero normalizes a span of 1 (a normal single cell) to 0 so the
// StructureAnnotation field stays omitempty; >1 is preserved.
func spanOrZero(span int) int {
	if span <= 1 {
		return 0
	}
	return span
}

func (r *Reader) emit(ctx context.Context, ch chan<- model.PartResult, part *model.Part) bool {
	select {
	case ch <- model.PartResult{Part: part}:
		return true
	case <-ctx.Done():
		return false
	}
}

func (r *Reader) emitError(ch chan<- model.PartResult, err error) {
	ch <- model.PartResult{Error: err}
}

func (r *Reader) locale() model.LocaleID {
	if r.Doc != nil && !r.Doc.SourceLocale.IsEmpty() {
		return r.Doc.SourceLocale
	}
	return model.LocaleEnglish
}

// Close releases resources.
func (r *Reader) Close() error {
	if r.Doc != nil && r.Doc.Reader != nil {
		return r.Doc.Reader.Close()
	}
	return nil
}

// --- helpers ---

type locVal struct {
	value      int
	resolution int
}

// advanceToRoot consumes tokens up to and including the <doclang> start element,
// so walkChildren then reads the document body (returning at </doclang>).
func advanceToRoot(dec *xml.Decoder) error {
	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "doclang" {
			return nil
		}
	}
}

func attrValue(e xml.StartElement, name string) string {
	for _, a := range e.Attr {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

func attrValueOr(e xml.StartElement, name, def string) string {
	if v := attrValue(e, name); v != "" {
		return v
	}
	return def
}

// readInnerText consumes a just-started element's content (until its matching
// EndElement) and returns the concatenated character data, ignoring nested tags.
func readInnerText(dec *xml.Decoder) string {
	var sb strings.Builder
	depth := 1
	for depth > 0 {
		t, err := dec.Token()
		if err != nil {
			break
		}
		switch el := t.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
		case xml.CharData:
			sb.Write(el)
		}
	}
	return sb.String()
}

// geometryFrom builds a GeometryAnnotation from a location block of exactly 4
// values (x0, y0, x1, y1, top-left). Returns nil otherwise. The location values
// alternate axes, so locations 0/2 carry the X-axis resolution and 1/3 the
// Y-axis resolution (DocLang location@resolution defaults to
// default_resolution@width|height, which may differ). ResolutionY is set only
// when the grid is non-square.
func geometryFrom(locs []locVal) *model.GeometryAnnotation {
	if len(locs) != 4 {
		return nil
	}
	x0, y0, x1, y1 := locs[0].value, locs[1].value, locs[2].value, locs[3].value
	resX := firstNonZero(locs[0].resolution, locs[2].resolution, 512)
	resY := firstNonZero(locs[1].resolution, locs[3].resolution, resX)
	g := &model.GeometryAnnotation{
		BBox:       model.Rect{X: float64(x0), Y: float64(y0), W: float64(x1 - x0), H: float64(y1 - y0)},
		Resolution: resX,
		Origin:     "top-left",
	}
	if resY != resX {
		g.ResolutionY = resY
	}
	return g
}

// firstNonZero returns the first non-zero argument, or the last argument as the
// default.
func firstNonZero(vals ...int) int {
	for _, v := range vals[:len(vals)-1] {
		if v != 0 {
			return v
		}
	}
	return vals[len(vals)-1]
}

// trimRunWhitespace trims the indentation whitespace around an element body
// (the leading/trailing text runs) without disturbing inline runs.
func trimRunWhitespace(runs []model.Run) []model.Run {
	for len(runs) > 0 && runs[0].Text != nil && strings.TrimSpace(runs[0].Text.Text) == "" {
		runs = runs[1:]
	}
	for len(runs) > 0 && runs[len(runs)-1].Text != nil && strings.TrimSpace(runs[len(runs)-1].Text.Text) == "" {
		runs = runs[:len(runs)-1]
	}
	if len(runs) > 0 && runs[0].Text != nil {
		runs[0].Text.Text = strings.TrimLeft(runs[0].Text.Text, " \t\r\n")
	}
	if n := len(runs); n > 0 && runs[n-1].Text != nil {
		runs[n-1].Text.Text = strings.TrimRight(runs[n-1].Text.Text, " \t\r\n")
	}
	return runs
}
