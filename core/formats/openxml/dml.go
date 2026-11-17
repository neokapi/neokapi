package openxml

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// dmlNamespace is the DrawingML namespace.
const dmlNamespace = "http://schemas.openxmlformats.org/drawingml/2006/main"

// dmlParser parses DrawingML XML parts (PPTX slides, notes, masters).
type dmlParser struct {
	cfg *Config
	// ns is the namespace prefixes of the package being read (nsScope).
	ns            *nsScope
	blockCounter  *int
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer
	rels          map[string]relationship
	// path addresses each block by part plus the shape it sits in; see
	// structural_name.go. shapeStep is the current shape's step, taken from
	// its <p:cNvPr id> when the part supplies one (PowerPoint keeps those
	// stable across edits) and from its position in the part otherwise.
	path      oxmlPath
	shapeStep string
	shapeSeq  int
	paraSeq   int

	// stripEmptyParaProps mirrors okapi's BlockProperties.Default.
	// getEvents (line 169-171 of okapi/filters/openxml/src/main/java/
	// net/sf/okapi/filters/openxml/BlockProperties.java) which omits
	// the entire pPr element when isEmpty() returns true (no
	// attributes, no non-empty children). Set true when parsing
	// chart/diagram parts where okapi unconditionally strips
	// scaffold-only <a:pPr><a:defRPr/></a:pPr> blocks (see
	// gold/Transimple_chart.docx). Left false for PPTX slides where
	// the existing behaviour preserved pPr verbatim.
	stripEmptyParaProps bool

	// Intrinsic slide geometry (WS2): when slideNum > 0 (a ppt/slides/slideN.xml
	// part), each text-bearing shape's bounding box is derived from its
	// DrawingML transform (<a:xfrm><a:off/><a:ext/>) and attached to the Block
	// as a GeometryAnnotation: additive stand-off metadata, never serialized
	// back. slideNum is set by the reader from the part path. The off/ext fields
	// hold the current shape's transform (EMU); they are reset at each shape
	// boundary. A placeholder without its own <a:xfrm> takes the frame its
	// layout or master gives it (placeholders); any other shape without one
	// gets no box. groupDepth tracks <p:grpSp> nesting and groups holds each
	// open group's transform, which maps its children's child-space
	// coordinates to slide space.
	slideNum   int
	hasOff     bool
	offX, offY int64
	hasExt     bool
	extCx      int64
	extCy      int64
	groupDepth int
	groups     []groupTransform
	// inGrpSpPr is set while the open group's own <p:grpSpPr> is read, so its
	// <a:off>/<a:ext>/<a:chOff>/<a:chExt> describe the group, not a shape.
	inGrpSpPr bool
	// partPath is the part being parsed; placeholders resolves an inherited
	// frame against it. phRawType and phIdx are the open placeholder's type
	// and idx attributes as written, cleared at each shape boundary.
	partPath     string
	placeholders *placeholderGeometry
	phRawType    string
	phIdx        string
	// Slide table geometry: the graphic frame's origin, the table grid's
	// column widths, the open row's top and declared height, and the column
	// the next a:tc occupies. cellBox is the open cell's frame.
	tblHas     bool
	tblX, tblY int64
	gridCols   []int64
	rowTop     int64
	rowH       int64
	colIndex   int
	cellBox    emuBox
	hasCellBox bool
	// phType is the current shape's placeholder type (<p:ph type>), cleared at
	// each shape boundary. It is the only signal PresentationML gives for a
	// paragraph's role, since a slide has no w:pStyle.
	phType string

	// emitPart surfaces a slide table's topology (a:tbl → table Group, a:tr →
	// table-row Group) beside the blocks, the shape the Word parser emits for
	// w:tbl, so cross-format writers and core/projection rebuild the grid. Nil
	// when a caller reads blocks only; the cells still carry their role.
	emitPart    func(*model.Part)
	structStack []structFrame
	// cellDepth is the a:tc nesting the parser is inside; a paragraph there is
	// a table cell. pendingColSpan and pendingRowSpan are the open cell's
	// gridSpan and rowSpan, applied to its first paragraph; pendingMerged
	// marks a cell that only continues a span from the left or above.
	cellDepth      int
	pendingColSpan int
	pendingRowSpan int
	pendingMerged  bool
	// headerRow is set by the table's <a:tblPr firstRow="1">, PowerPoint's
	// statement that the first row is a header row; rowIndex counts the
	// a:tr of the open table so that row's cells take the header role.
	headerRow bool
	rowIndex  int
}

// openTableStruct brackets a DrawingML table (a:tbl / a:tr) as a Group and
// tracks the cell (a:tc) the parser is entering, with the spans its attributes
// declare (ECMA-376-1 §21.1.3.16). Additive stand-off structure: the skeleton
// is untouched, so the byte-exact round-trip is unaffected.
func (p *dmlParser) openTableStruct(t xml.StartElement) {
	switch t.Name.Local {
	case "tbl":
		p.headerRow, p.rowIndex = false, 0
		p.tblHas, p.tblX, p.tblY = p.hasOff && p.hasExt, p.offX, p.offY
		p.gridCols, p.rowTop, p.rowH = nil, 0, 0
	case "tr":
		p.rowIndex++
		p.rowTop += p.rowH
		p.rowH, _ = strconv.ParseInt(attrVal(t, "h"), 10, 64)
		p.colIndex = 0
	case "tblPr":
		p.headerRow = dmlTrue(attrVal(t, "firstRow"))
		return
	}
	switch t.Name.Local {
	case "tbl", "tr":
		if p.emitPart == nil {
			return
		}
		kind := "table"
		if t.Name.Local == "tr" {
			kind = "table-row"
		}
		// Block ids are tu<n> off the same counter, so a group id drawn from it
		// is unique across every part of the deck.
		*p.blockCounter++
		id := fmt.Sprintf("tg%d", *p.blockCounter)
		p.structStack = append(p.structStack, structFrame{kind: kind, id: id})
		p.emitPart(&model.Part{Type: model.PartGroupStart, Resource: &model.GroupStart{ID: id, Name: kind, Type: kind}})
	case "tc":
		p.cellDepth++
		p.pendingColSpan, _ = strconv.Atoi(attrVal(t, "gridSpan"))
		p.pendingRowSpan, _ = strconv.Atoi(attrVal(t, "rowSpan"))
		p.pendingMerged = dmlTrue(attrVal(t, "hMerge")) || dmlTrue(attrVal(t, "vMerge"))
		p.openCellBox()
	}
}

// openCellBox computes the open cell's frame from the table grid: its column
// offset and width from <a:gridCol w>, its row offset and height from the
// rows' <a:tr h>. A cell spanning rows keeps the table frame, because the
// heights of the rows below are not read yet. Row heights are the declared
// minimums; PowerPoint grows a row to fit its text.
func (p *dmlParser) openCellBox() {
	col := p.colIndex
	p.colIndex++
	span := max(p.pendingColSpan, 1)
	p.hasCellBox = false
	if !p.tblHas || p.pendingRowSpan > 1 || col+span > len(p.gridCols) || p.rowH <= 0 {
		return
	}
	var x, w int64
	for _, cw := range p.gridCols[:col] {
		x += cw
	}
	for _, cw := range p.gridCols[col : col+span] {
		w += cw
	}
	p.cellBox = emuBox{x: p.tblX + x, y: p.tblY + p.rowTop, cx: w, cy: p.rowH}
	p.hasCellBox = true
}

// closeTableStruct closes the Group a:tbl / a:tr opened, or leaves the cell.
func (p *dmlParser) closeTableStruct(name string) {
	switch name {
	case "tbl", "tr":
		if p.emitPart == nil {
			return
		}
		kind := "table"
		if name == "tr" {
			kind = "table-row"
		}
		if n := len(p.structStack); n > 0 && p.structStack[n-1].kind == kind {
			f := p.structStack[n-1]
			p.structStack = p.structStack[:n-1]
			p.emitPart(&model.Part{Type: model.PartGroupEnd, Resource: &model.GroupEnd{ID: f.id}})
		}
	case "tc":
		if p.cellDepth > 0 {
			p.cellDepth--
		}
		p.pendingColSpan, p.pendingRowSpan, p.pendingMerged = 0, 0, false
		p.hasCellBox = false
	}
}

// markTableCell gives a paragraph inside an a:tc the table-cell role (the
// header role in a first row the table declares as one), and the cell's first
// paragraph its spans. A cell that only continues a merge from a
// neighbour is marked the way a Word vMerge continuation is, so the projection
// leaves it out and the originating cell's span covers the position.
func (p *dmlParser) markTableCell(block *model.Block) {
	if block.SemanticRole() != "" {
		return
	}
	role := model.RoleTableCell
	if p.headerRow && p.rowIndex == 1 {
		role = model.RoleTableHeader
	}
	block.SetSemanticRole(role, 0)
	if p.pendingMerged {
		block.Properties[model.PropTableVMerge] = "continue"
	}
	if p.pendingColSpan > 1 || p.pendingRowSpan > 1 {
		if s, ok := block.Structure(); ok && s != nil {
			if p.pendingColSpan > 1 {
				s.ColSpan = p.pendingColSpan
			}
			if p.pendingRowSpan > 1 {
				s.RowSpan = p.pendingRowSpan
			}
		}
	}
	// The spans belong to the cell's first paragraph only.
	p.pendingColSpan, p.pendingRowSpan = 0, 0
}

// dmlTrue reads an ST_Boolean attribute, which PowerPoint writes as "1".
func dmlTrue(v string) bool { return v == "1" || v == "true" }

// emuToPt converts English Metric Units (914400 EMU/inch, 12700 EMU/point) to
// points, the absolute unit the GeometryAnnotation BBox carries (Resolution 0).
func emuToPt(v int64) float64 { return float64(v) / 12700.0 }

// applyPPTXPartFacets records §8 structure facets a PPTX part implies: speaker
// notes (ppt/notesSlides/) are metadata shown only to the presenter, and slide
// masters/layouts are template furniture. Additive stand-off metadata; never
// serialized back, so byte-faithful round-trip is unaffected.
func applyPPTXPartFacets(block *model.Block, partPath string) {
	switch {
	case strings.HasPrefix(partPath, "ppt/notesSlides/"):
		block.SetLayoutLayer(model.LayerMetadata)
		block.SetVisibility(model.VisibilityScreenOnly)
	case strings.HasPrefix(partPath, "ppt/slideMasters/"),
		strings.HasPrefix(partPath, "ppt/slideLayouts/"):
		block.SetLayoutLayer(model.LayerFurniture)
	}
}

// pptxSlideNum returns the 1-based slide number for a `ppt/slides/slideN.xml`
// part, or 0 for any other PPTX part (notes, masters, layouts), which have a
// presentation-relative coordinate space, not a slide page, so they get no
// geometry. The prefix is exact: `ppt/slideLayouts/`, `ppt/slideMasters/`, and
// `ppt/notesSlides/` do not start with `ppt/slides/`.
func pptxSlideNum(partPath string) int {
	const prefix = "ppt/slides/slide"
	if !strings.HasPrefix(partPath, prefix) || !strings.HasSuffix(partPath, ".xml") {
		return 0
	}
	n, err := strconv.Atoi(partPath[len(prefix) : len(partPath)-len(".xml")])
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

// parsePart streams through a DrawingML XML part, emitting Blocks.
func (p *dmlParser) parsePart(data []byte, partPath string, emitBlock func(*model.Block)) error {
	// Root block names at this part before any shape opens. A later
	// re-rooting would drop the shape scope already pushed.
	p.path.ensurePart(partPath)
	p.partPath = partPath
	d := newRawDecoder(data)

	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("dml: parsing %s: %w", partPath, err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "txBody":
				// DrawingML text body, which holds the paragraphs
				if err := p.parseTextBody(d, partPath, emitBlock); err != nil {
					return err
				}
			case "tbl", "tr", "tc", "tblPr":
				p.openTableStruct(t)
				p.skelWriteStartElement(d, t)
			default:
				p.captureShapeGeometry(t)
				if p.cfg != nil && p.cfg.ExtractNonTranslatableContent() && isDrawingPropertyElement(t) {
					// Surface image/shape alt text (descr=) and object title
					// (title=) on <p:cNvPr>/<p:docPr> as Translatable:false
					// RoleCaption content (#928). PPTX does not extract the
					// graphic name= for translation, so name passes through.
					p.skelWriteDrawingPropElement(d, t, partPath, emitBlock)
				} else {
					p.skelWriteStartElement(d, t)
				}
			}

		case xml.EndElement:
			switch t.Name.Local {
			case "grpSp":
				p.groupDepth--
				if n := len(p.groups); n > 0 {
					p.groups = p.groups[:n-1]
				}
			case "grpSpPr":
				p.inGrpSpPr = false
			case "tbl", "tr", "tc":
				p.closeTableStruct(t.Name.Local)
			}
			p.skelWriteEndElement(d)

		case xml.CharData:
			p.skelRaw(d)

		case xml.ProcInst:
			p.skelRaw(d)

		case xml.Comment:
			p.skelRaw(d)
		}
	}
	return nil
}

// captureShapeGeometry tracks the current shape's transform as the top-level
// token stream flows past (the parser writes everything outside <txBody> to the
// skeleton verbatim, so <a:off>/<a:ext> pass through here with their attributes
// intact). A shape-boundary element resets the pending transform so a shape
// lacking its own <a:xfrm> inherits no box; <p:grpSp> bumps the group depth.
func (p *dmlParser) captureShapeGeometry(t xml.StartElement) {
	if t.Name.Space == dmlNamespace {
		if p.inGrpSpPr && len(p.groups) > 0 {
			p.groups[len(p.groups)-1].capture(t)
			return
		}
		switch t.Name.Local {
		case "gridCol": // <a:gridCol w=>: a slide table column's width (EMU)
			w, _ := strconv.ParseInt(attrVal(t, "w"), 10, 64)
			p.gridCols = append(p.gridCols, w)
		case "off": // <a:off x= y=>: shape origin (EMU)
			p.offX, _ = strconv.ParseInt(attrVal(t, "x"), 10, 64)
			p.offY, _ = strconv.ParseInt(attrVal(t, "y"), 10, 64)
			p.hasOff = true
		case "ext": // <a:ext cx= cy=>: shape size (EMU)
			p.extCx, _ = strconv.ParseInt(attrVal(t, "cx"), 10, 64)
			p.extCy, _ = strconv.ParseInt(attrVal(t, "cy"), 10, 64)
			p.hasExt = true
		}
		return
	}
	// PresentationML shape boundaries: a new shape clears the pending transform;
	// a group raises the depth so its child shapes (child-space coords) are
	// skipped by attachShapeGeometry.
	switch t.Name.Local {
	case "grpSp":
		p.groupDepth++
		p.groups = append(p.groups, groupTransform{})
		p.hasOff, p.hasExt = false, false
	case "grpSpPr":
		// The group's own properties come first inside <p:grpSp>; the slide's
		// <p:spTree> has a <p:grpSpPr> too, with no group open.
		if n := len(p.groups); n > 0 && !p.groups[n-1].seen {
			p.groups[n-1].seen = true
			p.inGrpSpPr = true
		}
	case "sp", "pic", "graphicFrame", "cxnSp":
		p.hasOff, p.hasExt = false, false
		p.phType, p.phRawType, p.phIdx = "", "", ""
		p.openShape()
	case "ph":
		// <p:ph type="title"|"ctrTitle"|"subTitle"|"body"|…> inside
		// <p:nvSpPr><p:nvPr> is the shape's placeholder role on the slide
		// layout. PresentationML has no w:pStyle: this element is the only
		// thing that distinguishes a slide title from body text, so without it
		// a deck's whole outline is invisible. ECMA-376-1 §19.3.1.36 makes
		// "body" the default when the attribute is absent.
		p.phType = attrVal(t, "type")
		p.phRawType, p.phIdx = p.phType, attrVal(t, "idx")
		if p.phType == "" {
			p.phType = "body"
		}
	case "cNvPr", "docPr":
		// The shape's own id, which PowerPoint keeps across edits. It beats
		// the positional step reserved when the shape opened.
		if id := attrVal(t, "id"); id != "" {
			p.setShapeID(id)
		}
	}
}

// placeholderRole maps the current shape's placeholder type to a semantic role
// and heading level. A deck's outline lives entirely in these, and
// PresentationML distinguishes the deck title from a slide title precisely so a
// consumer can nest them: ctrTitle (the title slide's title) heads the
// document, an ordinary slide title heads a section under it, the subtitle
// sits with the section titles. Anything else (body text, a bare text box, a
// table cell) keeps the paragraph role it already had, so this narrows to the
// outline and changes nothing else.
func (p *dmlParser) placeholderRole() (string, int) {
	switch p.phType {
	case "ctrTitle":
		return model.RoleHeading, 1
	case "title", "subTitle":
		return model.RoleHeading, 2
	default:
		return "", 0
	}
}

// openShape starts a new shape scope: the step is positional until a
// <p:cNvPr id> inside it supplies the shape's own key.
func (p *dmlParser) openShape() {
	p.path.scopes = nil
	p.shapeSeq++
	p.shapeStep = ordinalStep("shape", p.shapeSeq)
	p.path.pushStep("shape", p.shapeStep)
	p.paraSeq = 0
}

// setShapeID replaces the open shape's positional step with its own id.
func (p *dmlParser) setShapeID(id string) {
	p.shapeStep = "shape[" + id + "]"
	p.path.scopes = nil
	p.path.pushStep("shape", p.shapeStep)
}

// blockName addresses a paragraph inside the current shape.
func (p *dmlParser) paragraphName(partPath string) string {
	p.path.ensurePart(partPath)
	p.paraSeq++
	return p.path.name(ordinalStep("p", p.paraSeq))
}

// attachShapeGeometry sets the block's page geometry on a slide part. The
// frame is the open table cell's, else the shape's own <a:xfrm>, else (for a
// placeholder outside a group) the frame its layout or master gives it. Inside
// groups the frame is mapped out through each group's transform. DrawingML's
// origin is top-left, so no flip.
func (p *dmlParser) attachShapeGeometry(b *model.Block) {
	if p.slideNum <= 0 {
		return
	}
	var box emuBox
	switch {
	case p.cellDepth > 0 && p.hasCellBox:
		box = p.cellBox
	case p.hasOff && p.hasExt:
		box = emuBox{x: p.offX, y: p.offY, cx: p.extCx, cy: p.extCy}
	case p.phType != "" && len(p.groups) == 0:
		inherited, ok := p.placeholders.resolve(p.partPath, p.phRawType, p.phIdx)
		if !ok {
			return
		}
		box = inherited
	default:
		return
	}
	x, y, w, h := float64(box.x), float64(box.y), float64(box.cx), float64(box.cy)
	for i := len(p.groups) - 1; i >= 0; i-- {
		var ok bool
		if x, y, w, h, ok = p.groups[i].apply(x, y, w, h); !ok {
			return
		}
	}
	b.SetGeometry(&model.GeometryAnnotation{
		Page:   p.slideNum,
		BBox:   model.Rect{X: x / emuPerPt, Y: y / emuPerPt, W: w / emuPerPt, H: h / emuPerPt},
		Origin: "top-left",
	})
}

// emuPerPt is the number of English Metric Units in a point.
const emuPerPt = 12700.0

// groupTransform is a <p:grpSp>'s <a:xfrm>: the group's frame on its parent
// (off, ext) and the child coordinate space its shapes are placed in (chOff,
// chExt), ECMA-376-1 §20.1.7.5.
type groupTransform struct {
	seen                               bool
	offX, offY, extCx, extCy           int64
	chOffX, chOffY, chExtCx, chExtCy   int64
	hasOff, hasExt, hasChOff, hasChExt bool
}

func (g *groupTransform) capture(t xml.StartElement) {
	num := func(name string) int64 {
		v, _ := strconv.ParseInt(attrVal(t, name), 10, 64)
		return v
	}
	switch t.Name.Local {
	case "off":
		g.offX, g.offY, g.hasOff = num("x"), num("y"), true
	case "ext":
		g.extCx, g.extCy, g.hasExt = num("cx"), num("cy"), true
	case "chOff":
		g.chOffX, g.chOffY, g.hasChOff = num("x"), num("y"), true
	case "chExt":
		g.chExtCx, g.chExtCy, g.hasChExt = num("cx"), num("cy"), true
	}
}

// apply maps a box from the group's child space to its parent's space. A group
// with no frame of its own cannot be placed, so neither can its children.
// Without chOff/chExt the child space is the group's own frame.
func (g *groupTransform) apply(x, y, w, h float64) (float64, float64, float64, float64, bool) {
	if !g.hasOff || !g.hasExt {
		return 0, 0, 0, 0, false
	}
	chOffX, chOffY, chExtCx, chExtCy := g.offX, g.offY, g.extCx, g.extCy
	if g.hasChOff {
		chOffX, chOffY = g.chOffX, g.chOffY
	}
	if g.hasChExt {
		chExtCx, chExtCy = g.chExtCx, g.chExtCy
	}
	sx, sy := 1.0, 1.0
	if chExtCx != 0 {
		sx = float64(g.extCx) / float64(chExtCx)
	}
	if chExtCy != 0 {
		sy = float64(g.extCy) / float64(chExtCy)
	}
	return float64(g.offX) + (x-float64(chOffX))*sx,
		float64(g.offY) + (y-float64(chOffY))*sy,
		w * sx, h * sy, true
}

// parseTextBody parses an <a:txBody> element.
func (p *dmlParser) parseTextBody(d *rawDecoder, partPath string, emitBlock func(*model.Block)) error {
	// The wrapper goes back as it was written, attributes and all.
	p.skelWriteString(d.RawString())

	for {
		tok, err := d.Token()
		if err != nil {
			return err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "p":
				if err := p.parseParagraph(d, partPath, emitBlock); err != nil {
					return err
				}
			case "bodyPr", "lstStyle":
				raw, err := captureRawElement(d, t)
				if err != nil {
					return err
				}
				p.skelWriteString(raw)
			default:
				p.skelWriteStartElement(d, t)
			}

		case xml.EndElement:
			if t.Name.Local == "txBody" {
				p.skelWriteEndElement(d)
				return nil
			}
			p.skelWriteEndElement(d)

		case xml.CharData:
			// The whitespace a producer indented <a:bodyPr>, <a:lstStyle> and
			// each <a:p> with. Nothing in a text body reads it, and a deck
			// saved with indented XML comes back compact without it.
			p.skelRaw(d)

		case xml.Comment, xml.ProcInst:
			p.skelRaw(d)
		}
	}
}

// parseParagraph parses an <a:p> element and emits a Block.
func (p *dmlParser) parseParagraph(d *rawDecoder, partPath string, emitBlock func(*model.Block)) error {
	var runs []textRun
	var paraProps string
	var endParaRPr string
	// The paragraph's own start tag, kept so the skeleton can put it back with
	// whatever attributes it carried, and its offset, so a paragraph holding no
	// extractable text can be replayed whole.
	paraStart := d.RawString()
	paraOff := d.Offset()
	contentOff := d.EndOffset()
	d.Pin(paraOff)
	defer d.Unpin()

	// The span the paragraph's run-bearing children occupy: from the first
	// <a:r>, <a:br> or opaque child to the last. It is the half the writer
	// rebuilds from the block, and the half the block keeps the source bytes of
	// when the two spell it differently. See dmlSourceProp.
	runsStart, runsEnd := int64(-1), int64(-1)

	for {
		tok, err := d.Token()
		if err != nil {
			return err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			childStart := d.Offset()
			switch t.Name.Local {
			case "pPr", "endParaRPr":
				raw, err := captureRawElement(d, t)
				if err != nil {
					return err
				}
				if t.Name.Local == "pPr" {
					if p.stripEmptyParaProps && isStructurallyEmptyDMLBlockProperties(raw) {
						paraProps = ""
					} else {
						paraProps = raw
					}
				} else {
					// <a:endParaRPr> holds the run properties of the paragraph
					// mark (ECMA-376 Part 1 §21.1.2.2.6). Nothing extracts it,
					// so it is carried to the end of the rebuilt paragraph.
					endParaRPr = raw
				}

			case "r":
				run, err := p.parseRun(d)
				if err != nil {
					return err
				}
				runs = append(runs, run...)
				runsStart, runsEnd = spanGrow(runsStart, childStart, d.EndOffset())

			case "br":
				// <a:br> can carry its own <a:rPr> (ECMA-376 Part 1
				// §21.1.2.2.1), which decides the height of the line it
				// starts, so the element goes back as it was read.
				raw, err := captureRawElement(d, t)
				if err != nil {
					return err
				}
				runs = append(runs, textRun{text: "\n", props: runProps{}, data: raw})
				runsStart, runsEnd = spanGrow(runsStart, childStart, d.EndOffset())

			default:
				// Every other direct child of <a:p> is markup the reader does
				// not model: <a:fld> (a slide number, a date), <a:m>, a
				// paragraph-level <mc:AlternateContent>. Capturing it as an
				// opaque placeholder keeps it in the paragraph and in its
				// place; skipping it deleted it from any paragraph that also
				// carried text.
				raw, err := captureRawElement(d, t)
				if err != nil {
					return err
				}
				runs = append(runs, textRun{text: sentinelParaOpaque, props: runProps{}, data: raw})
				runsStart, runsEnd = spanGrow(runsStart, childStart, d.EndOffset())
			}

		case xml.EndElement:
			if t.Name.Local == "p" {
				merged := mergeRuns(runs)

				if isEmptyRuns(merged) {
					// Nothing was extracted, so the paragraph goes back as the
					// source wrote it: a placeholder <a:fld>, an
					// <a:endParaRPr>, the line endings and the empty-element
					// forms all survive. A chart or diagram paragraph is the
					// exception: okapi's BlockProperties.getEvents omits a
					// structurally empty pPr there, so those are rebuilt.
					if !p.stripEmptyParaProps {
						p.skelWriteString(d.FromString(paraOff))
						return nil
					}
					p.skelWriteString(paraStart)
					if paraProps != "" {
						p.skelWriteString(paraProps)
					}
					p.skelWriteString(endParaRPr)
					p.skelWriteEndElement(d)
					return nil
				}

				*p.blockCounter++
				blockID := fmt.Sprintf("tu%d", *p.blockCounter)

				p.skelWriteString(paraStart)
				if p.stripEmptyParaProps {
					// A chart or diagram paragraph is rebuilt around the ref:
					// okapi's BlockProperties.getEvents omits a structurally
					// empty pPr, so the source bytes on either side cannot be
					// replayed whole.
					if paraProps != "" {
						p.skelWriteString(paraProps)
					}
					p.skelRef(blockID)
					p.skelWriteString(endParaRPr)
				} else {
					// Everything on either side of the runs goes back as the
					// source wrote it, so the <a:pPr>, the <a:endParaRPr> and
					// the whitespace a producer indented them with all survive.
					p.skelWriteString(d.RangeString(contentOff, runsStart))
					p.skelRef(blockID)
					p.skelWriteString(d.RangeString(runsEnd, d.Offset()))
				}
				p.skelWriteEndElement(d)

				block := p.buildBlock(blockID, merged, partPath)
				if src := dmlSourceForm(d.RangeString(runsStart, runsEnd), block.SourceRuns()); src != "" {
					block.Properties[dmlSourceProp] = src
				}
				if role, level := p.placeholderRole(); role != "" {
					block.SetSemanticRole(role, level)
				}
				if p.cellDepth > 0 {
					p.markTableCell(block)
				}
				p.attachShapeGeometry(block)
				applyPPTXPartFacets(block, partPath)
				emitBlock(block)
				return nil
			}
		}
	}
}

// parseRun parses an <a:r> element.
func (p *dmlParser) parseRun(d *rawDecoder) ([]textRun, error) {
	var props runProps
	var runs []textRun

	for {
		tok, err := d.Token()
		if err != nil {
			return nil, err
		}

		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "rPr":
				props = parseDMLRunProps(t)
				// The element travels whole: the reader names five of its
				// attributes and DrawingML states the rest of a run's
				// formatting in the others and in the children below them.
				// See runProps.dmlRPr.
				raw, err := captureRawElement(d, t)
				if err != nil {
					return nil, err
				}
				props.dmlRPr = raw
			case "t":
				text, err := readCharData(d)
				if err != nil {
					return nil, err
				}
				runs = append(runs, textRun{text: text, props: props})
			default:
				if err := skipElement(d); err != nil {
					return nil, err
				}
			}

		case xml.EndElement:
			if t.Name.Local == "r" {
				return runs, nil
			}
		}
	}
}

// dmlPropsEqual reports whether two DrawingML runs would produce the same
// <a:rPr>. The whole element counts, so a run that differs only in size or
// colour keeps its own boundary.
func dmlPropsEqual(a, b runProps) bool {
	return a.equal(b) && a.dmlRPr == b.dmlRPr
}

// parseDMLRunProps extracts run properties from DrawingML <a:rPr> attributes.
func parseDMLRunProps(el xml.StartElement) runProps {
	var props runProps
	for _, a := range el.Attr {
		applyDMLRunPropAttr(&props, a.Name.Local, a.Value)
	}
	return props
}

// applyDMLRunPropAttr reads one <a:rPr> attribute into the model. It is the
// single statement of what each of the five named attributes means, so the
// writer's reconciliation and the reader agree on when an attribute is already
// saying what the codes ask for.
func applyDMLRunPropAttr(props *runProps, name, value string) {
	switch name {
	case "b":
		props.bold = dmlToggleOn(value)
	case "i":
		props.italic = dmlToggleOn(value)
	case "u":
		if value != "" && value != "none" {
			props.underline = value
		}
	case "strike":
		if value != "" && value != "noStrike" {
			props.strike = true
		}
	case "baseline":
		props.vertAlign = dmlBaselineVertAlign(value)
	}
}

// dmlToggleOn reads a DrawingML boolean attribute (ECMA-376 Part 1 §22.9.2.7,
// ST_OnOff).
func dmlToggleOn(value string) bool {
	return value == "1" || value == "true"
}

// dmlBaselineVertAlign reads the baseline percentage a run is raised or lowered
// by (ECMA-376 Part 1 §21.1.2.3.9). A negative offset lowers the text.
func dmlBaselineVertAlign(value string) string {
	switch {
	case strings.HasPrefix(value, "-"):
		return "subscript"
	case value == "" || value == "0":
		return ""
	default:
		return "superscript"
	}
}

// buildBlock creates a model.Block from text runs.
func (p *dmlParser) buildBlock(id string, runs []textRun, partPath string) *model.Block {
	b := &runBuilder{}
	ids := &spanIDs{}
	var activeProps *runProps

	closeActive := func() {
		if activeProps != nil {
			dmlRunPropsProjection.appendClosing(*activeProps, b, ids)
			activeProps = nil
		}
	}

	for _, run := range runs {
		if run.text == "\n" {
			closeActive()
			data := run.data
			if data == "" {
				data = "<a:br/>"
			}
			b.AddPh(ids.placeholder(),
				TypeBreak, SubTypeBreak,
				data, "\n", "",
				false, false, false)
			continue
		}

		if run.text == sentinelParaOpaque {
			closeActive()
			subType := SubTypeDMLParaChild
			if strings.HasPrefix(run.data, "<a:fld") {
				subType = SubTypeDMLField
			}
			b.AddPh(ids.placeholder(),
				TypeOpaqueParaChild, subType,
				run.data, "", "",
				false, false, false)
			continue
		}

		if activeProps == nil || !dmlPropsEqual(*activeProps, run.props) {
			closeActive()
			dmlRunPropsProjection.appendOpening(run.props, b, ids)
			propsCopy := run.props
			activeProps = &propsCopy
		}

		b.AddText(run.text)
	}

	closeActive()

	block := model.NewRunsBlock(id, b.Runs())
	block.Name = p.paragraphName(partPath)
	block.Type = "paragraph"
	block.Properties["partPath"] = partPath
	return block
}

// skelWriteDrawingPropElement writes a <p:cNvPr>/<p:docPr> (or pic:/wps:
// variant) drawing-property start element to the skeleton, surfacing its descr=
// (accessibility alt text) and title= (object title) attribute values as
// Translatable:false RoleCaption "property" blocks (#928). Each surfaced value
// is replaced with a skeleton ref so the writer restores it via renderBlock
// ("property" → escaped text); an untranslated block restores the source value,
// keeping the round-trip byte-exact. All other attributes, including name=,
// which the PPTX path does not extract for translation, pass through verbatim.
//
// Mirrors the WML drawing-name path (writeDrawingPropertyElementTo) and the
// SpreadsheetML table-column path (skelWriteTableColumn). Only called when
// ExtractNonTranslatableContent is on; otherwise the caller writes the element
// verbatim so the part stream stays byte-identical to upstream Okapi. The
// skeleton helpers are no-ops when no skeleton store is wired (inspection-only
// reads), but the alt-text blocks are still emitted so an in-memory consumer
// sees them.
func (p *dmlParser) skelWriteDrawingPropElement(d *rawDecoder, t xml.StartElement, partPath string, emitBlock func(*model.Block)) {
	p.names().register(t.Attr)
	raw := d.Raw()

	// Every byte outside the extracted attribute values is replayed from the
	// source, so the element keeps its self-closing form, attribute order and
	// quoting. A tag whose attribute list will not parse byte for byte leaves
	// nowhere to put a ref, so it passes through whole.
	type refSpan struct {
		start, end int
		attr       xml.Attr
	}
	var spans []refSpan
	for i, a := range t.Attr {
		if a.Name.Space != "" || strings.TrimSpace(a.Value) == "" {
			continue
		}
		if a.Name.Local != "descr" && a.Name.Local != "title" {
			continue
		}
		vs, ve, ok := rawAttrValueSpan(raw, i)
		if !ok {
			p.skelWriteStartElement(d, t)
			return
		}
		spans = append(spans, refSpan{vs, ve, a})
	}
	if len(spans) == 0 {
		p.skelWriteStartElement(d, t)
		return
	}

	prev := 0
	for _, sp := range spans {
		p.skelWriteString(string(raw[prev:sp.start]))
		p.skelRef(p.emitDrawingProp(sp.attr, partPath, emitBlock))
		prev = sp.end
	}
	p.skelWriteString(string(raw[prev:]))
}

// emitDrawingProp allocates the next block id, emits a Translatable:false
// RoleCaption "property" block carrying the drawing-property attribute value as
// a single verbatim run, and returns the block id (for the skeleton ref).
func (p *dmlParser) emitDrawingProp(a xml.Attr, partPath string, emitBlock func(*model.Block)) string {
	*p.blockCounter++
	id := fmt.Sprintf("tu%d", *p.blockCounter)
	element := propElementDrawingDescr
	if a.Name.Local == "title" {
		element = propElementDrawingTitle
	}
	p.path.ensurePart(partPath)
	block := model.NewRunsBlock(id, []model.Run{{Text: &model.TextRun{Text: a.Value}}})
	block.Name = p.path.name("@" + a.Name.Local)
	block.Type = "property"
	block.Translatable = false
	block.Properties["partPath"] = partPath
	block.Properties["element"] = element
	// Alt text / object title is descriptive prose for an image or shape;
	// RoleCaption lets semantic export and the editor identify it without
	// treating it as MT input.
	block.SetSemanticRole(model.RoleCaption, 0)
	emitBlock(block)
	return id
}

// emitPPTXCommentData scans a legacy PowerPoint comment part
// (ppt/comments/comment*.xml) for <p:cm><p:text> bodies and surfaces each as an
// informational Data part (#928). The comment part itself is parsed for skeleton
// by parsePart (everything verbatim), so this is purely additive and never
// affects the round-trip. Best-effort: a malformed part yields no Data rather
// than failing the read. Modern comment parts (modernComment_*.xml, which use a
// txBody body, not <p:text>) and the non-translatable position/author metadata
// are left untouched.
func emitPPTXCommentData(data []byte, emitData func(name, text, ref string)) {
	d := newRawDecoder(data)
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			return
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "text" {
			text, err := readCharData(d)
			if err != nil {
				return
			}
			if strings.TrimSpace(text) != "" {
				emitData("comment", text, "")
			}
		}
	}
}

// Skeleton helpers

func (p *dmlParser) skelRef(id string) {
	if p.skeletonStore != nil {
		if p.skelBuf.Len() > 0 {
			p.skeletonStore.WriteText(p.skelBuf.Bytes())
			p.skelBuf.Reset()
		}
		p.skeletonStore.WriteRef(id)
	}
}

func (p *dmlParser) skelFlush() {
	if p.skeletonStore != nil && p.skelBuf.Len() > 0 {
		p.skeletonStore.WriteText(p.skelBuf.Bytes())
		p.skelBuf.Reset()
	}
}

// skelRaw appends the source bytes of the token the decoder last returned.
func (p *dmlParser) skelRaw(d *rawDecoder) {
	if p.skeletonStore != nil {
		p.skelBuf.Write(d.Raw())
	}
}

func (p *dmlParser) skelWriteStartElement(d *rawDecoder, t xml.StartElement) {
	if p.skeletonStore == nil {
		return
	}
	p.names().register(t.Attr)
	p.skelBuf.Write(d.Raw())
}

// skelWriteEndElement appends an end tag. A self-closing element's synthetic end
// carries no source bytes, so its `/>` came through with the start element.
func (p *dmlParser) skelWriteEndElement(d *rawDecoder) {
	if p.skeletonStore == nil {
		return
	}
	p.skelBuf.Write(d.Raw())
}

func (p *dmlParser) skelWriteString(s string) {
	if p.skeletonStore != nil {
		p.skelBuf.WriteString(s)
	}
}
