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
// text, footnote, page_header, page_footer, code, formula; list (+ ldiv items);
// group; table/index (OTSL cells fcel/ched/rhed/ecel/srow/corn, row terminator
// nl); the element-head properties layer and the 4-value location block (→
// geometry); and inline formatting (bold/italic/underline/strikethrough/
// superscript/subscript). Not yet mapped (read-through, declared in spec.yaml):
// OTSL span continuations (lcel/ucel/xcel — the spanned grid cells are dropped),
// picture/field/marker/checkbox constructs, and thread/xref/href continuity —
// all skipped without losing surrounding content.
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
	"heading":     model.RoleHeading,
	"text":        model.RoleParagraph,
	"footnote":    model.RoleFootnote,
	"page_header": model.RolePageHeader,
	"page_footer": model.RolePageFooter,
	"code":        model.RoleCode,
	"formula":     model.RoleFormula,
}

// containerElem are DocLang elements that wrap other semantics (emitted as a
// neokapi Group). Tables are handled separately (OTSL).
var containerElem = map[string]bool{
	"list": true, "group": true, "field_region": true, "picture": true,
}

// headElem are element-head property elements (precede the body).
var headElem = map[string]bool{
	"label": true, "thread": true, "xref": true, "href": true,
	"layer": true, "location": true, "caption": true, "custom": true,
}

// fmtTag maps a DocLang inline formatting element to its run vocabulary type.
var fmtTag = map[string]string{
	"bold":          "fmt:bold",
	"italic":        "fmt:italic",
	"underline":     "fmt:underline",
	"strikethrough": "fmt:strikethrough",
	"superscript":   "fmt:superscript",
	"subscript":     "fmt:subscript",
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

// Reader implements DataFormatReader for DocLang documents.
type Reader struct {
	format.BaseFormatReader
	cfg          *Config
	blockCounter int
	groupCounter int
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
		cfg: cfg,
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
			name := el.Name.Local
			switch {
			case name == "head" || name == "page_break" || headElem[name]:
				if err := dec.Skip(); err != nil {
					return err
				}
			case parent == "list" && name == "ldiv":
				// <ldiv> is the list-item delimiter (holds the bullet/number
				// marker); the item content is the following <text>. Skip it.
				if err := dec.Skip(); err != nil {
					return err
				}
			case name == "table" || name == "index":
				if err := r.parseTable(ctx, ch, dec, el); err != nil {
					return err
				}
			case containerElem[name]:
				if err := r.parseContainer(ctx, ch, dec, el); err != nil {
					return err
				}
			default: // block element (known role) or unknown → paragraph-ish
				role := blockRole[name]
				if parent == "list" && name == "text" {
					role = model.RoleListItem // a <text> inside a <list> is an item
				}
				if err := r.parseBlock(ctx, ch, dec, el, role); err != nil {
					return err
				}
			}
		case xml.EndElement:
			return nil // closes the current container
		case xml.CharData:
			// whitespace between elements at container scope — ignore
		}
	}
}

// parseContainer emits a Group bracketing the element's children.
func (r *Reader) parseContainer(ctx context.Context, ch chan<- model.PartResult, dec *xml.Decoder, start xml.StartElement) error {
	r.groupCounter++
	gid := fmt.Sprintf("g%d", r.groupCounter)
	name := start.Name.Local
	props := map[string]string{}
	if class := attrValue(start, "class"); class != "" {
		props["class"] = class
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartGroupStart, Resource: &model.GroupStart{ID: gid, Name: name, Type: name, Properties: props}}) {
		return context.Canceled
	}
	if err := r.walkChildren(ctx, ch, dec, name); err != nil {
		return err
	}
	r.emit(ctx, ch, &model.Part{Type: model.PartGroupEnd, Resource: &model.GroupEnd{ID: gid}})
	return nil
}

// parseBlock parses a block-level element: its element head (layer + location)
// and its inline body, emitting one PartBlock with role, geometry, and layer.
func (r *Reader) parseBlock(ctx context.Context, ch chan<- model.PartResult, dec *xml.Decoder, start xml.StartElement, role string) error {
	level := 0
	if start.Name.Local == "heading" {
		if l := attrValue(start, "level"); l != "" {
			level, _ = strconv.Atoi(l)
		} else {
			level = 1
		}
	}

	var (
		runs    []model.Run
		layer   string
		locs    []locVal
		idCtr   int
		fmtOpen []string // stack of open formatting tag names
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
	block.Type = start.Name.Local
	if role != "" {
		block.SetSemanticRole(role, level)
	}
	if layer != "" && layer != model.LayerBody {
		block.SetLayoutLayer(layer)
	}
	if g := geometryFrom(locs); g != nil {
		block.SetGeometry(g)
	}
	r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block})
	return nil
}

// parseTable handles an OTSL table/index as a flat token state machine: a Group
// wraps per-row Groups whose children are cell Blocks. Each cell-starting token
// begins a cell; its content is the following text/<text> until the next cell
// token or <nl/>. Span continuations (lcel/ucel/xcel) are skipped.
func (r *Reader) parseTable(ctx context.Context, ch chan<- model.PartResult, dec *xml.Decoder, start xml.StartElement) error {
	r.groupCounter++
	tid := fmt.Sprintf("g%d", r.groupCounter)
	if !r.emit(ctx, ch, &model.Part{Type: model.PartGroupStart, Resource: &model.GroupStart{ID: tid, Name: start.Name.Local, Type: start.Name.Local}}) {
		return context.Canceled
	}

	var (
		cell     strings.Builder
		cellRole string
		inCell   bool
		rowID    string
		rowOpen  bool
	)
	flushCell := func() {
		if inCell {
			if txt := strings.TrimSpace(cell.String()); txt != "" {
				r.blockCounter++
				b := model.NewBlock(fmt.Sprintf("b%d", r.blockCounter), txt)
				b.SourceLocale = r.locale()
				b.Type = "table-cell"
				b.SetSemanticRole(cellRole, 0)
				r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: b})
			}
		}
		cell.Reset()
		inCell = false
	}
	openRow := func() {
		if !rowOpen {
			r.groupCounter++
			rowID = fmt.Sprintf("g%d", r.groupCounter)
			r.emit(ctx, ch, &model.Part{Type: model.PartGroupStart, Resource: &model.GroupStart{ID: rowID, Name: "table-row", Type: "table-row"}})
			rowOpen = true
		}
	}
	closeRow := func() {
		flushCell()
		if rowOpen {
			r.emit(ctx, ch, &model.Part{Type: model.PartGroupEnd, Resource: &model.GroupEnd{ID: rowID}})
			rowOpen = false
		}
	}

	for {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		stop := false
		switch el := tok.(type) {
		case xml.StartElement:
			n := el.Name.Local
			switch {
			case otslCellTok[n] != "":
				flushCell()
				openRow()
				inCell, cellRole = true, otslCellTok[n]
				if err := dec.Skip(); err != nil { // consume the empty cell token
					return err
				}
			case n == "nl":
				closeRow()
				if err := dec.Skip(); err != nil {
					return err
				}
			case n == "text":
				cell.WriteString(readInnerText(dec))
			default: // head/caption/span continuations/nested — skip
				if err := dec.Skip(); err != nil {
					return err
				}
			}
		case xml.EndElement:
			stop = true // </table>
		case xml.CharData:
			if inCell {
				cell.WriteString(string(el))
			}
		}
		if stop {
			break
		}
	}
	closeRow()
	r.emit(ctx, ch, &model.Part{Type: model.PartGroupEnd, Resource: &model.GroupEnd{ID: tid}})
	return nil
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
// values (x0, y0, x1, y1, top-left). Returns nil otherwise.
func geometryFrom(locs []locVal) *model.GeometryAnnotation {
	if len(locs) != 4 {
		return nil
	}
	x0, y0, x1, y1 := locs[0].value, locs[1].value, locs[2].value, locs[3].value
	res := locs[0].resolution
	if res == 0 {
		res = 512
	}
	return &model.GeometryAnnotation{
		BBox:       model.Rect{X: float64(x0), Y: float64(y0), W: float64(x1 - x0), H: float64(y1 - y0)},
		Resolution: res,
		Origin:     "top-left",
	}
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
