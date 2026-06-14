// Package docling reads DoclingDocument JSON — the native, lossless JSON
// serialization emitted by Docling (https://github.com/docling-project/docling),
// IBM's document-conversion library. It is the file-based "consume Docling"
// boundary alongside the DocLang XML reader (core/formats/doclang): a tool runs
// Docling, saves the DoclingDocument JSON, and neokapi reads it into the content
// model plus the WS1 structural layer (SemanticRole, GeometryAnnotation,
// LayoutLayer — see core/model/structure.go).
//
// The reader walks the document's reading order (body.children), resolving the
// $ref pointers into the texts/tables/groups/pictures arrays, and maps each
// item's DocItemLabel onto a normalized SemanticRole, its provenance bbox onto a
// GeometryAnnotation, and page-header/footer matter onto the furniture layout
// layer. Tables become a Group of row Groups of cell Blocks; lists become a
// Group of list-item Blocks; table/picture captions are emitted as caption-role
// Blocks.
//
// It is read-only — Docling owns the JSON; neokapi re-emits structure via the
// DocLang writer (faithful) or projects to Markdown/HTML (semantic). Inline
// formatting is not part of the base DoclingDocument text model (TextItem.text
// is plain), so each text item yields a single text run.
package docling

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// SchemaName is the discriminator every DoclingDocument JSON carries.
const SchemaName = "DoclingDocument"

// labelRole maps a Docling DocItemLabel to a normalized SemanticRole.
var labelRole = map[string]string{
	"title":          model.RoleTitle,
	"section_header": model.RoleHeading,
	"paragraph":      model.RoleParagraph,
	"text":           model.RoleParagraph,
	"list_item":      model.RoleListItem,
	"caption":        model.RoleCaption,
	"footnote":       model.RoleFootnote,
	"page_header":    model.RolePageHeader,
	"page_footer":    model.RolePageFooter,
	"code":           model.RoleCode,
	"formula":        model.RoleFormula,
	"reference":      model.RoleParagraph,
	"document_index": model.RoleParagraph,
}

// furnitureLabel are labels whose content belongs to the furniture layout layer
// (running headers/footers) rather than the body.
var furnitureLabel = map[string]bool{
	"page_header": true,
	"page_footer": true,
}

// Reader implements DataFormatReader for DoclingDocument JSON.
type Reader struct {
	format.BaseFormatReader
	cfg          *Config
	blockCounter int
	groupCounter int
	visited      map[string]bool
}

// NewReader creates a new DoclingDocument JSON reader.
func NewReader() *Reader {
	cfg := &Config{}
	cfg.Reset()
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "docling",
			FormatDisplayName: "DoclingDocument JSON",
			FormatMimeType:    "application/json",
			FormatExtensions:  []string{".json"},
			Cfg:               cfg,
		},
		cfg:     cfg,
		visited: map[string]bool{},
	}
}

// Signature returns detection metadata. The schema_name + DoclingDocument sniff
// disambiguates DoclingDocument JSON from generic .json (register.go gives this
// format a lower priority so a plain .json never falls through to it).
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"application/json"},
		Extensions: []string{".json"},
		Sniff:      sniff,
	}
}

func sniff(data []byte) bool {
	return strings.Contains(string(data), `"schema_name"`) &&
		strings.Contains(string(data), SchemaName)
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("docling: nil document or reader")
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
	data, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		r.emitError(ch, fmt.Errorf("docling: reading document: %w", err))
		return
	}

	var doc doclingDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		r.emitError(ch, fmt.Errorf("docling: parsing DoclingDocument JSON: %w", err))
		return
	}
	if doc.SchemaName != "" && doc.SchemaName != SchemaName {
		r.emitError(ch, fmt.Errorf("docling: not a DoclingDocument (schema_name=%q)", doc.SchemaName))
		return
	}

	idx := doc.index()

	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "docling",
		Locale:   r.locale(),
		Encoding: r.Doc.Encoding,
		MimeType: "application/json",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	// Walk the reading order. body is the canonical root; furniture (running
	// headers/footers, deprecated in newer schema) follows so its content is
	// not lost.
	for _, root := range []*docNode{doc.Body, doc.Furniture} {
		if root == nil {
			continue
		}
		for _, c := range root.Children {
			if !r.walkRef(ctx, ch, idx, c.Ref) {
				r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
				return
			}
		}
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// walkRef dispatches a single child reference to the right emitter. Returns
// false if the context was cancelled (caller should stop).
func (r *Reader) walkRef(ctx context.Context, ch chan<- model.PartResult, idx *docIndex, ref string) bool {
	if ref == "" || r.visited[ref] {
		return true
	}
	r.visited[ref] = true

	switch {
	case idx.texts[ref] != nil:
		return r.emitText(ctx, ch, idx, idx.texts[ref])
	case idx.tables[ref] != nil:
		return r.emitTable(ctx, ch, idx, idx.tables[ref])
	case idx.pictures[ref] != nil:
		return r.emitPicture(ctx, ch, idx, idx.pictures[ref])
	case idx.groups[ref] != nil:
		return r.emitGroup(ctx, ch, idx, idx.groups[ref])
	default:
		return true // dangling ref — skip without losing surrounding content
	}
}

// emitText emits one Block for a text item, carrying its role, level, layout
// layer, and geometry — then recurses into the item's own children. In a
// DoclingDocument the tree hangs off every node, not just groups/body: a
// list_item's sub-bullets are nested groups referenced from the item's
// `children`, and a container node may carry no text of its own. Walking
// children here is what keeps nested lists (and any child content) from being
// dropped.
func (r *Reader) emitText(ctx context.Context, ch chan<- model.PartResult, idx *docIndex, t *docText) bool {
	text := t.Text
	if text == "" {
		text = t.Orig
	}
	if strings.TrimSpace(text) != "" {
		r.blockCounter++
		block := model.NewBlock(fmt.Sprintf("b%d", r.blockCounter), text)
		block.SourceLocale = r.locale()
		block.Type = t.Label

		role := labelRole[t.Label]
		if role != "" {
			level := 0
			if role == model.RoleHeading {
				level = t.Level
				if level == 0 {
					level = 1
				}
			}
			block.SetSemanticRole(role, level)
		}
		if furnitureLabel[t.Label] {
			block.SetLayoutLayer(model.LayerFurniture)
		}
		if g := geometryFromProv(t.Prov, t.SelfRef); g != nil {
			block.SetGeometry(g)
		}
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return false
		}
	}
	// Recurse into nested children (sub-lists, grouped sub-content) even when
	// this node has no text of its own — it may be a pure container.
	for _, c := range t.Children {
		if !r.walkRef(ctx, ch, idx, c.Ref) {
			return false
		}
	}
	return true
}

// emitGroup emits a Group bracketing the group's children (lists, inline groups).
func (r *Reader) emitGroup(ctx context.Context, ch chan<- model.PartResult, idx *docIndex, g *docGroup) bool {
	r.groupCounter++
	gid := fmt.Sprintf("g%d", r.groupCounter)
	name := g.Name
	if name == "" {
		name = g.Label
	}
	if name == "" {
		name = "group"
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartGroupStart, Resource: &model.GroupStart{ID: gid, Name: name, Type: name}}) {
		return false
	}
	for _, c := range g.Children {
		if !r.walkRef(ctx, ch, idx, c.Ref) {
			return false
		}
	}
	return r.emit(ctx, ch, &model.Part{Type: model.PartGroupEnd, Resource: &model.GroupEnd{ID: gid}})
}

// emitTable emits a table Group: its caption blocks, then per-row Groups whose
// children are cell Blocks (header cells carry the table-header role).
func (r *Reader) emitTable(ctx context.Context, ch chan<- model.PartResult, idx *docIndex, t *docTable) bool {
	r.groupCounter++
	tid := fmt.Sprintf("g%d", r.groupCounter)
	if !r.emit(ctx, ch, &model.Part{Type: model.PartGroupStart, Resource: &model.GroupStart{ID: tid, Name: "table", Type: "table"}}) {
		return false
	}
	if !r.emitCaptions(ctx, ch, idx, t.Captions) {
		return false
	}

	cells := t.Data.Cells
	for row := range t.Data.NumRows {
		rowCells := cellsForRow(cells, row)
		if len(rowCells) == 0 {
			continue
		}
		r.groupCounter++
		rowID := fmt.Sprintf("g%d", r.groupCounter)
		if !r.emit(ctx, ch, &model.Part{Type: model.PartGroupStart, Resource: &model.GroupStart{ID: rowID, Name: "table-row", Type: "table-row"}}) {
			return false
		}
		for _, c := range rowCells {
			if strings.TrimSpace(c.Text) == "" {
				continue
			}
			role := model.RoleTableCell
			if c.isHeader() {
				role = model.RoleTableHeader
			}
			r.blockCounter++
			b := model.NewBlock(fmt.Sprintf("b%d", r.blockCounter), c.Text)
			b.SourceLocale = r.locale()
			b.Type = "table-cell"
			b.SetSemanticRole(role, 0)
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: b}) {
				return false
			}
		}
		if !r.emit(ctx, ch, &model.Part{Type: model.PartGroupEnd, Resource: &model.GroupEnd{ID: rowID}}) {
			return false
		}
	}
	return r.emit(ctx, ch, &model.Part{Type: model.PartGroupEnd, Resource: &model.GroupEnd{ID: tid}})
}

// emitPicture emits a picture Group carrying its caption blocks. The image
// itself is non-translatable; the captions are.
func (r *Reader) emitPicture(ctx context.Context, ch chan<- model.PartResult, idx *docIndex, p *docPicture) bool {
	r.groupCounter++
	gid := fmt.Sprintf("g%d", r.groupCounter)
	if !r.emit(ctx, ch, &model.Part{Type: model.PartGroupStart, Resource: &model.GroupStart{ID: gid, Name: "picture", Type: "picture"}}) {
		return false
	}
	if !r.emitCaptions(ctx, ch, idx, p.Captions) {
		return false
	}
	return r.emit(ctx, ch, &model.Part{Type: model.PartGroupEnd, Resource: &model.GroupEnd{ID: gid}})
}

// emitCaptions emits the referenced caption text items as caption-role blocks.
func (r *Reader) emitCaptions(ctx context.Context, ch chan<- model.PartResult, idx *docIndex, caps []ref) bool {
	for _, c := range caps {
		t := idx.texts[c.Ref]
		if t == nil || r.visited[c.Ref] {
			continue
		}
		r.visited[c.Ref] = true
		text := t.Text
		if text == "" {
			text = t.Orig
		}
		if strings.TrimSpace(text) == "" {
			continue
		}
		r.blockCounter++
		b := model.NewBlock(fmt.Sprintf("b%d", r.blockCounter), text)
		b.SourceLocale = r.locale()
		b.Type = t.Label
		b.SetSemanticRole(model.RoleCaption, 0)
		if g := geometryFromProv(t.Prov, t.SelfRef); g != nil {
			b.SetGeometry(g)
		}
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: b}) {
			return false
		}
	}
	return true
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

// cellsForRow returns the cells whose start row offset is the given row, ordered
// by start column offset.
func cellsForRow(cells []tableCell, row int) []tableCell {
	var out []tableCell
	for _, c := range cells {
		if c.StartRow == row {
			out = append(out, c)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].StartCol < out[j].StartCol })
	return out
}

// geometryFromProv builds a GeometryAnnotation from a provenance entry's bbox.
// Docling bboxes carry an explicit coord_origin (TOPLEFT or BOTTOMLEFT); X/W are
// origin-independent (l, r-l) while Y/H are normalized to the lower coordinate +
// magnitude, with Origin recorded so a layout consumer can interpret them.
func geometryFromProv(provs []prov, selfRef string) *model.GeometryAnnotation {
	if len(provs) == 0 || provs[0].BBox == nil {
		return nil
	}
	p := provs[0]
	bb := p.BBox
	y := bb.T
	h := bb.B - bb.T
	if h < 0 {
		y = bb.B
		h = bb.T - bb.B
	}
	origin := "top-left"
	if strings.EqualFold(bb.CoordOrigin, "BOTTOMLEFT") {
		origin = "bottom-left"
	}
	return &model.GeometryAnnotation{
		Page:      p.PageNo,
		BBox:      model.Rect{X: bb.L, Y: y, W: bb.R - bb.L, H: h},
		Origin:    origin,
		SourceRef: selfRef,
	}
}
