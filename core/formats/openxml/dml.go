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
	"github.com/neokapi/neokapi/core/internal/xmlesc"
	"github.com/neokapi/neokapi/core/model"
)

// dmlNamespace is the DrawingML namespace.
const dmlNamespace = "http://schemas.openxmlformats.org/drawingml/2006/main"

// dmlParser parses DrawingML XML parts (PPTX slides, notes, masters).
type dmlParser struct {
	cfg           *Config
	blockCounter  *int
	skeletonStore *format.SkeletonStore
	skelBuf       bytes.Buffer
	rels          map[string]relationship
	// path addresses each block by part plus the shape it sits in — see
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
	// as a GeometryAnnotation — additive stand-off metadata, never serialized
	// back. slideNum is set by the reader from the part path. The off/ext fields
	// hold the current shape's transform (EMU); they are reset at each shape
	// boundary so a shape without its own <a:xfrm> inherits no box. groupDepth
	// tracks <p:grpSp> nesting: child shapes inside a group carry child-space
	// coordinates that need an affine remap to slide space, so geometry is
	// omitted inside groups (v1) rather than emitting wrong absolute boxes.
	slideNum   int
	hasOff     bool
	offX, offY int64
	hasExt     bool
	extCx      int64
	extCy      int64
	groupDepth int
	// phType is the current shape's placeholder type (<p:ph type>), cleared at
	// each shape boundary. It is the only signal PresentationML gives for a
	// paragraph's role — there is no w:pStyle on a slide.
	phType string
}

// emuToPt converts English Metric Units (914400 EMU/inch, 12700 EMU/point) to
// points — the absolute unit the GeometryAnnotation BBox carries (Resolution 0).
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
// part, or 0 for any other PPTX part (notes, masters, layouts) — those have a
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
	// Root block names at this part before any shape opens — a later
	// re-rooting would drop the shape scope already pushed.
	p.path.ensurePart(partPath)
	d := xml.NewDecoder(bytes.NewReader(data))

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
				// DrawingML text body — contains paragraphs
				if err := p.parseTextBody(d, partPath, emitBlock); err != nil {
					return err
				}
			default:
				p.captureShapeGeometry(t)
				if p.cfg != nil && p.cfg.ExtractNonTranslatableContent() && isDrawingPropertyElement(t) {
					// Surface image/shape alt text (descr=) and object title
					// (title=) on <p:cNvPr>/<p:docPr> as Translatable:false
					// RoleCaption content (#928). PPTX does not extract the
					// graphic name= for translation, so name passes through.
					p.skelWriteDrawingPropElement(t, partPath, emitBlock)
				} else {
					p.skelWriteStartElement(t)
				}
			}

		case xml.EndElement:
			if t.Name.Local == "grpSp" {
				p.groupDepth--
			}
			p.skelWriteEndElement(t)

		case xml.CharData:
			p.skelText(xmlesc.Text(string(t)))

		case xml.ProcInst:
			p.skelText("<?" + t.Target + " " + string(t.Inst) + "?>")

		case xml.Comment:
			p.skelText("<!--" + string(t) + "-->")
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
		switch t.Name.Local {
		case "off": // <a:off x= y=> — shape origin (EMU)
			p.offX, _ = strconv.ParseInt(attrVal(t, "x"), 10, 64)
			p.offY, _ = strconv.ParseInt(attrVal(t, "y"), 10, 64)
			p.hasOff = true
		case "ext": // <a:ext cx= cy=> — shape size (EMU)
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
		p.hasOff, p.hasExt = false, false
	case "sp", "pic", "graphicFrame", "cxnSp":
		p.hasOff, p.hasExt = false, false
		p.phType = ""
		p.openShape()
	case "ph":
		// <p:ph type="title"|"ctrTitle"|"subTitle"|"body"|…> inside
		// <p:nvSpPr><p:nvPr> — the shape's placeholder role on the slide
		// layout. PresentationML has no w:pStyle: this element is the only
		// thing that distinguishes a slide title from body text, so without it
		// a deck's whole outline is invisible. ECMA-376-1 §19.3.1.36 makes
		// "body" the default when the attribute is absent.
		p.phType = attrVal(t, "type")
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
// sits with the section titles. Anything else — body text, a bare text box, a
// table cell — keeps the paragraph role it already had, so this narrows to the
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

// attachShapeGeometry sets the block's page geometry from the current shape's
// transform, when this is a slide part, we are not inside a group, and the
// shape carried a full <a:xfrm>. DrawingML's origin is top-left, so no flip.
func (p *dmlParser) attachShapeGeometry(b *model.Block) {
	if p.slideNum <= 0 || p.groupDepth != 0 || !p.hasOff || !p.hasExt {
		return
	}
	b.SetGeometry(&model.GeometryAnnotation{
		Page: p.slideNum,
		BBox: model.Rect{
			X: emuToPt(p.offX), Y: emuToPt(p.offY),
			W: emuToPt(p.extCx), H: emuToPt(p.extCy),
		},
		Origin: "top-left",
	})
}

// parseTextBody parses an <a:txBody> element.
func (p *dmlParser) parseTextBody(d *xml.Decoder, partPath string, emitBlock func(*model.Block)) error {
	p.skelWriteString("<a:txBody>")

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
				p.skelText(raw)
			default:
				p.skelWriteStartElement(t)
			}

		case xml.EndElement:
			if t.Name.Local == "txBody" {
				p.skelWriteString("</a:txBody>")
				return nil
			}
			p.skelWriteEndElement(t)
		}
	}
}

// parseParagraph parses an <a:p> element and emits a Block.
func (p *dmlParser) parseParagraph(d *xml.Decoder, partPath string, emitBlock func(*model.Block)) error {
	var runs []textRun
	var paraProps string

	for {
		tok, err := d.Token()
		if err != nil {
			return err
		}

		switch t := tok.(type) {
		case xml.StartElement:
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
				}

			case "r":
				run, err := p.parseRun(d)
				if err != nil {
					return err
				}
				runs = append(runs, run...)

			case "br":
				runs = append(runs, textRun{text: "\n", props: runProps{}})
				if err := skipElement(d); err != nil {
					return err
				}

			default:
				if err := skipElement(d); err != nil {
					return err
				}
			}

		case xml.EndElement:
			if t.Name.Local == "p" {
				merged := mergeRuns(runs)

				if isEmptyRuns(merged) {
					p.skelWriteString("<a:p>")
					if paraProps != "" {
						p.skelText(paraProps)
					}
					p.skelWriteString("</a:p>")
					return nil
				}

				*p.blockCounter++
				blockID := fmt.Sprintf("tu%d", *p.blockCounter)

				p.skelWriteString("<a:p>")
				if paraProps != "" {
					p.skelText(paraProps)
				}
				p.skelRef(blockID)
				p.skelWriteString("</a:p>")

				block := p.buildBlock(blockID, merged, partPath)
				if role, level := p.placeholderRole(); role != "" {
					block.SetSemanticRole(role, level)
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
func (p *dmlParser) parseRun(d *xml.Decoder) ([]textRun, error) {
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
				if err := skipElement(d); err != nil {
					return nil, err
				}
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

// parseDMLRunProps extracts run properties from DrawingML <a:rPr> attributes.
func parseDMLRunProps(el xml.StartElement) runProps {
	var props runProps
	for _, a := range el.Attr {
		switch a.Name.Local {
		case "b":
			props.bold = a.Value == "1" || a.Value == "true"
		case "i":
			props.italic = a.Value == "1" || a.Value == "true"
		case "u":
			if a.Value != "" && a.Value != "none" {
				props.underline = a.Value
			}
		case "strike":
			if a.Value != "" && a.Value != "noStrike" {
				props.strike = true
			}
		case "baseline":
			if strings.HasPrefix(a.Value, "-") {
				props.vertAlign = "subscript"
			} else if a.Value != "" && a.Value != "0" {
				props.vertAlign = "superscript"
			}
		}
	}
	return props
}

// buildBlock creates a model.Block from text runs.
func (p *dmlParser) buildBlock(id string, runs []textRun, partPath string) *model.Block {
	b := &runBuilder{}
	ids := &spanIDs{}
	var activeProps *runProps

	for _, run := range runs {
		if run.text == "\n" {
			b.AddPh(ids.placeholder(),
				TypeBreak, SubTypeBreak,
				"<a:br/>", "\n", "",
				false, false, false)
			continue
		}

		if activeProps == nil || !activeProps.equal(run.props) {
			if activeProps != nil && !activeProps.isEmpty() {
				activeProps.appendClosingRuns(b, ids)
			}
			if !run.props.isEmpty() {
				run.props.appendOpeningRuns(b, ids)
			}
			propsCopy := run.props
			activeProps = &propsCopy
		}

		b.AddText(run.text)
	}

	if activeProps != nil && !activeProps.isEmpty() {
		activeProps.appendClosingRuns(b, ids)
	}

	return &model.Block{
		ID:           id,
		Name:         p.paragraphName(partPath),
		Type:         "paragraph",
		Translatable: true,
		Source:       b.Runs(),
		Targets:      make(map[model.VariantKey]*model.Target),
		Properties:   map[string]string{"partPath": partPath},
	}
}

// skelWriteDrawingPropElement writes a <p:cNvPr>/<p:docPr> (or pic:/wps:
// variant) drawing-property start element to the skeleton, surfacing its descr=
// (accessibility alt text) and title= (object title) attribute values as
// Translatable:false RoleCaption "property" blocks (#928). Each surfaced value
// is replaced with a skeleton ref so the writer restores it via renderBlock
// ("property" → escaped text); an untranslated block restores the source value,
// keeping the round-trip byte-exact. All other attributes — including name=,
// which the PPTX path does not extract for translation — pass through verbatim.
//
// Mirrors the WML drawing-name path (writeDrawingPropertyElementTo) and the
// SpreadsheetML table-column path (skelWriteTableColumn). Only called when
// ExtractNonTranslatableContent is on; otherwise the caller writes the element
// verbatim so the part stream stays byte-identical to upstream Okapi. The
// skeleton helpers are no-ops when no skeleton store is wired (inspection-only
// reads), but the alt-text blocks are still emitted so an in-memory consumer
// sees them.
func (p *dmlParser) skelWriteDrawingPropElement(t xml.StartElement, partPath string, emitBlock func(*model.Block)) {
	registerNamespaces(t.Attr)
	var nameBuf strings.Builder
	nameBuf.WriteString("<")
	writeElementName(&nameBuf, t.Name)
	p.skelWriteString(nameBuf.String())
	for _, a := range t.Attr {
		var attrBuf strings.Builder
		attrBuf.WriteString(" ")
		writeAttrName(&attrBuf, a.Name)
		attrBuf.WriteString(`="`)
		p.skelWriteString(attrBuf.String())
		if a.Name.Space == "" && strings.TrimSpace(a.Value) != "" &&
			(a.Name.Local == "descr" || a.Name.Local == "title") {
			p.skelRef(p.emitDrawingProp(a, partPath, emitBlock))
		} else {
			p.skelWriteString(xmlesc.Attr(a.Value))
		}
		p.skelWriteString(`"`)
	}
	p.skelWriteString(">")
}

// emitDrawingProp allocates the next block id, emits a Translatable:false
// RoleCaption "property" block carrying the drawing-property attribute value as
// a single verbatim run, and returns the block id (for the skeleton ref).
func (p *dmlParser) emitDrawingProp(a xml.Attr, partPath string, emitBlock func(*model.Block)) string {
	*p.blockCounter++
	id := fmt.Sprintf("tu%d", *p.blockCounter)
	element := "drawing-descr"
	if a.Name.Local == "title" {
		element = "drawing-title"
	}
	p.path.ensurePart(partPath)
	block := &model.Block{
		ID:           id,
		Name:         p.path.name("@" + a.Name.Local),
		Type:         "property",
		Translatable: false,
		Source:       []model.Run{{Text: &model.TextRun{Text: a.Value}}},
		Targets:      make(map[model.VariantKey]*model.Target),
		Properties: map[string]string{
			"partPath": partPath,
			"element":  element,
		},
	}
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
	d := xml.NewDecoder(bytes.NewReader(data))
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

func (p *dmlParser) skelText(s string) {
	if p.skeletonStore != nil {
		p.skelBuf.WriteString(s)
	}
}

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

func (p *dmlParser) skelWriteStartElement(t xml.StartElement) {
	if p.skeletonStore == nil {
		return
	}
	registerNamespaces(t.Attr)
	var buf strings.Builder
	buf.WriteString("<")
	writeElementName(&buf, t.Name)
	for _, a := range t.Attr {
		buf.WriteString(" ")
		writeAttrName(&buf, a.Name)
		buf.WriteString(`="`)
		buf.WriteString(xmlesc.Attr(a.Value))
		buf.WriteString(`"`)
	}
	buf.WriteString(">")
	p.skelBuf.WriteString(buf.String())
}

func (p *dmlParser) skelWriteEndElement(t xml.EndElement) {
	if p.skeletonStore == nil {
		return
	}
	var buf strings.Builder
	buf.WriteString("</")
	writeElementName(&buf, t.Name)
	buf.WriteString(">")
	p.skelBuf.WriteString(buf.String())
}

func (p *dmlParser) skelWriteString(s string) {
	if p.skeletonStore != nil {
		p.skelBuf.WriteString(s)
	}
}
