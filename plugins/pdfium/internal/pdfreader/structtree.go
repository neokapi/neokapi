package pdfreader

import (
	"fmt"
	"math"
	"strings"

	pdfium "github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/enums"
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/structure"
)

// structtree.go implements the tier-1 structure path: when a PDF is *tagged*
// (carries a logical structure tree — Document › H1 › P › Table › TR › TH/TD …),
// the structure is authoritative and we read it directly instead of inferring it
// geometrically. We bridge the tagged elements to text through marked-content IDs
// (MCIDs): each struct element references the MCIDs of the page's text objects
// that render its content, so we first build an MCID→{text,bbox} map from the page
// objects, then walk the struct tree and gather each element's text from its (and
// its NonStruct descendants') MCIDs.
//
// The page-object and struct-element MCID accessors are PDFium *experimental* APIs
// (go-pdfium only wires them under the `pdfium_experimental` build tag; the bundled
// bblanchon libpdfium exports them). Without that tag the accessors return a
// runtime error — which we detect and treat as "no tier-1 available", falling back
// to the geometric analyzer (structure.Analyze). So this file needs no build tag:
// it compiles either way and degrades gracefully at runtime.

// mcEntry accumulates the text and bounding box of all page text objects that
// share one marked-content ID. Bounds are raw PDFium (bottom-left) coords.
type mcEntry struct {
	text       strings.Builder
	l, t, r, b float64
	has        bool
}

func (e *mcEntry) add(text string, l, b, r, t float64) {
	e.text.WriteString(text)
	if !e.has {
		e.l, e.b, e.r, e.t = l, b, r, t
		e.has = true
		return
	}
	e.l, e.b = math.Min(e.l, l), math.Min(e.b, b)
	e.r, e.t = math.Max(e.r, r), math.Max(e.t, t)
}

// structRegions reads a tagged PDF page's logical structure into structure.Regions
// (the same neutral shape the geometric analyzer produces, so the caller maps both
// onto the content model identically). It returns ok=false — meaning "no usable
// tagged structure, fall back to geometry" — when the page has no struct tree, when
// the experimental MCID APIs are unavailable, or when the tree yields no text.
func structRegions(inst pdfium.Pdfium, docRef references.FPDF_DOCUMENT, pageIndex int, pageHeight float64, counter *int) ([]structure.Region, bool) {
	pageRes, err := inst.FPDF_LoadPage(&requests.FPDF_LoadPage{Document: docRef, Index: pageIndex})
	if err != nil {
		return nil, false
	}
	pageRef := pageRes.Page
	defer inst.FPDF_ClosePage(&requests.FPDF_ClosePage{Page: pageRef})
	page := requests.Page{ByReference: &pageRef}

	tpRes, err := inst.FPDFText_LoadPage(&requests.FPDFText_LoadPage{Page: page})
	if err != nil {
		return nil, false
	}
	textPage := tpRes.TextPage
	defer inst.FPDFText_ClosePage(&requests.FPDFText_ClosePage{TextPage: textPage})

	mc, ok := buildMCIDMap(inst, page, textPage)
	if !ok || len(mc) == 0 {
		return nil, false // experimental MCID API unavailable, or no tagged text
	}

	treeRes, err := inst.FPDF_StructTree_GetForPage(&requests.FPDF_StructTree_GetForPage{Page: page})
	if err != nil || treeRes.StructTree == "" {
		return nil, false
	}
	tree := treeRes.StructTree
	defer inst.FPDF_StructTree_Close(&requests.FPDF_StructTree_Close{StructTree: tree})

	cnt, err := inst.FPDF_StructTree_CountChildren(&requests.FPDF_StructTree_CountChildren{StructTree: tree})
	if err != nil || cnt.Count == 0 {
		return nil, false
	}

	w := &treeWalker{inst: inst, mc: mc, pageHeight: pageHeight, pageNum: pageIndex + 1, counter: counter}
	var regions []structure.Region
	for i := 0; i < cnt.Count; i++ {
		ch, err := inst.FPDF_StructTree_GetChildAtIndex(&requests.FPDF_StructTree_GetChildAtIndex{StructTree: tree, Index: i})
		if err != nil || ch.StructElement == "" {
			continue
		}
		regions = append(regions, w.element(ch.StructElement)...)
	}
	if len(regions) == 0 {
		return nil, false
	}
	return regions, true
}

// buildMCIDMap walks the page's text objects (recursing into form XObjects) and
// maps each marked-content ID to its accumulated text and bounds. ok is false when
// the experimental FPDFPageObj_GetMarkedContentID API is unavailable (the first
// call errors), which is the signal to fall back to the geometric tier.
func buildMCIDMap(inst pdfium.Pdfium, page requests.Page, textPage references.FPDF_TEXTPAGE) (map[int]*mcEntry, bool) {
	cnt, err := inst.FPDFPage_CountObjects(&requests.FPDFPage_CountObjects{Page: page})
	if err != nil {
		return nil, false
	}
	mc := map[int]*mcEntry{}
	experimentalOK := false
	var visit func(obj references.FPDF_PAGEOBJECT)
	visit = func(obj references.FPDF_PAGEOBJECT) {
		typeRes, err := inst.FPDFPageObj_GetType(&requests.FPDFPageObj_GetType{PageObject: obj})
		if err != nil {
			return
		}
		switch typeRes.Type {
		case enums.FPDF_PAGEOBJ_FORM:
			fc, err := inst.FPDFFormObj_CountObjects(&requests.FPDFFormObj_CountObjects{PageObject: obj})
			if err != nil {
				return
			}
			for i := 0; i < fc.Count; i++ {
				sub, err := inst.FPDFFormObj_GetObject(&requests.FPDFFormObj_GetObject{PageObject: obj, Index: uint64(i)})
				if err != nil || sub.PageObject == "" {
					continue
				}
				visit(sub.PageObject)
			}
		case enums.FPDF_PAGEOBJ_TEXT:
			mcRes, err := inst.FPDFPageObj_GetMarkedContentID(&requests.FPDFPageObj_GetMarkedContentID{PageObject: obj})
			if err != nil {
				return // experimental API unavailable
			}
			experimentalOK = true
			id := mcRes.MarkedContentID
			if id < 0 {
				return
			}
			txt, err := inst.FPDFTextObj_GetText(&requests.FPDFTextObj_GetText{PageObject: obj, TextPage: textPage})
			if err != nil {
				return
			}
			bnd, err := inst.FPDFPageObj_GetBounds(&requests.FPDFPageObj_GetBounds{PageObject: obj})
			if err != nil {
				return
			}
			e := mc[id]
			if e == nil {
				e = &mcEntry{}
				mc[id] = e
			}
			e.add(txt.Text, float64(bnd.Left), float64(bnd.Bottom), float64(bnd.Right), float64(bnd.Top))
		}
	}
	for i := 0; i < cnt.Count; i++ {
		obj, err := inst.FPDFPage_GetObject(&requests.FPDFPage_GetObject{Page: page, Index: i})
		if err != nil || obj.PageObject == "" {
			continue
		}
		visit(obj.PageObject)
	}
	return mc, experimentalOK
}

type treeWalker struct {
	inst       pdfium.Pdfium
	mc         map[int]*mcEntry
	pageHeight float64
	pageNum    int
	counter    *int
}

// element maps one struct element to zero or more regions in reading order.
// Container types (Document, Sect, Part, Div, NonStruct, L, …) recurse; block
// types (headings, paragraphs) become a single block region; a Table becomes a
// table region.
func (w *treeWalker) element(el references.FPDF_STRUCTELEMENT) []structure.Region {
	typ := w.elemType(el)
	switch {
	case typ == "TABLE":
		if t := w.table(el); t != nil {
			return []structure.Region{{Kind: structure.RegionTable, Table: t}}
		}
		return nil
	case isHeading(typ):
		if b := w.block(el); b != nil {
			return []structure.Region{{Kind: structure.RegionBlock, Block: b, Role: model.RoleHeading, Level: headingLevelOf(typ)}}
		}
		return nil
	case isParagraph(typ):
		if b := w.block(el); b != nil {
			return []structure.Region{{Kind: structure.RegionBlock, Block: b, Role: model.RoleParagraph}}
		}
		return nil
	default:
		// Container: recurse into children, preserving order.
		var out []structure.Region
		for _, c := range w.children(el) {
			out = append(out, w.element(c)...)
		}
		return out
	}
}

// table builds a structure.Table from a Table element, recursing through optional
// THead/TBody/TFoot section groups to reach the TR rows.
func (w *treeWalker) table(el references.FPDF_STRUCTELEMENT) *structure.Table {
	t := &structure.Table{}
	var collectRows func(e references.FPDF_STRUCTELEMENT)
	collectRows = func(e references.FPDF_STRUCTELEMENT) {
		for _, c := range w.children(e) {
			switch w.elemType(c) {
			case "THEAD", "TBODY", "TFOOT":
				collectRows(c)
			case "TR":
				t.Rows = append(t.Rows, w.row(c))
			}
		}
	}
	collectRows(el)
	if len(t.Rows) == 0 {
		return nil
	}
	return t
}

func (w *treeWalker) row(el references.FPDF_STRUCTELEMENT) []structure.Cell {
	var cells []structure.Cell
	for _, c := range w.children(el) {
		switch w.elemType(c) {
		case "TH", "TD":
			cell := structure.Cell{Header: w.elemType(c) == "TH"}
			if b := w.block(c); b != nil {
				cell.Blocks = []*model.Block{b}
			}
			cells = append(cells, cell)
		}
	}
	return cells
}

// block builds a content Block from a leaf-ish element by gathering the text and
// union bounds of its (and its descendants') marked-content IDs. Returns nil when
// the element carries no text.
func (w *treeWalker) block(el references.FPDF_STRUCTELEMENT) *model.Block {
	var text strings.Builder
	box := mcEntry{}
	w.gather(el, &text, &box)
	s := strings.TrimSpace(text.String())
	if s == "" {
		return nil
	}
	*w.counter++
	b := model.NewBlock(fmt.Sprintf("tu%d", *w.counter), s)
	b.Properties["page-number"] = fmt.Sprintf("%d", w.pageNum)
	if box.has {
		b.SetGeometry(w.flip(box))
	}
	return b
}

// gather recursively accumulates the text and bounds of an element's own MCIDs and
// all its descendants' MCIDs (Chrome and most taggers stash a block's text in
// NonStruct leaf children, so a plain element-level read misses it).
func (w *treeWalker) gather(el references.FPDF_STRUCTELEMENT, text *strings.Builder, box *mcEntry) {
	idCnt, err := w.inst.FPDF_StructElement_GetMarkedContentIdCount(&requests.FPDF_StructElement_GetMarkedContentIdCount{StructElement: el})
	if err == nil {
		for i := 0; i < idCnt.Count; i++ {
			idRes, err := w.inst.FPDF_StructElement_GetMarkedContentIdAtIndex(&requests.FPDF_StructElement_GetMarkedContentIdAtIndex{StructElement: el, Index: i})
			if err != nil {
				continue
			}
			if e := w.mc[idRes.MarkedContentID]; e != nil && e.has {
				text.WriteString(e.text.String())
				box.add(e.text.String(), e.l, e.b, e.r, e.t)
			}
		}
	}
	for _, c := range w.children(el) {
		w.gather(c, text, box)
	}
}

func (w *treeWalker) children(el references.FPDF_STRUCTELEMENT) []references.FPDF_STRUCTELEMENT {
	cnt, err := w.inst.FPDF_StructElement_CountChildren(&requests.FPDF_StructElement_CountChildren{StructElement: el})
	if err != nil {
		return nil
	}
	var out []references.FPDF_STRUCTELEMENT
	for i := 0; i < cnt.Count; i++ {
		ch, err := w.inst.FPDF_StructElement_GetChildAtIndex(&requests.FPDF_StructElement_GetChildAtIndex{StructElement: el, Index: i})
		if err != nil || ch.StructElement == "" {
			continue
		}
		out = append(out, ch.StructElement)
	}
	return out
}

func (w *treeWalker) elemType(el references.FPDF_STRUCTELEMENT) string {
	res, err := w.inst.FPDF_StructElement_GetType(&requests.FPDF_StructElement_GetType{StructElement: el})
	if err != nil {
		return ""
	}
	return strings.ToUpper(strings.TrimSpace(res.Type))
}

// flip converts an mcEntry's raw (bottom-left) union bounds to a top-left
// GeometryAnnotation, matching geometryFromRect.
func (w *treeWalker) flip(e mcEntry) *model.GeometryAnnotation {
	x := math.Min(e.l, e.r)
	width := math.Abs(e.r - e.l)
	upper := math.Max(e.t, e.b)
	lower := math.Min(e.t, e.b)
	height := upper - lower
	if w.pageHeight > 0 {
		return &model.GeometryAnnotation{Page: w.pageNum, BBox: model.Rect{X: x, Y: w.pageHeight - upper, W: width, H: height}, Origin: "top-left"}
	}
	return &model.GeometryAnnotation{Page: w.pageNum, BBox: model.Rect{X: x, Y: lower, W: width, H: height}, Origin: "bottom-left"}
}

func isHeading(typ string) bool {
	switch typ {
	case "H", "H1", "H2", "H3", "H4", "H5", "H6", "TITLE":
		return true
	}
	return false
}

func headingLevelOf(typ string) int {
	switch typ {
	case "H1", "H", "TITLE":
		return 1
	case "H2":
		return 2
	case "H3":
		return 3
	case "H4":
		return 4
	case "H5":
		return 5
	case "H6":
		return 6
	}
	return 1
}

func isParagraph(typ string) bool {
	switch typ {
	case "P", "LBODY", "LBL", "NOTE", "CAPTION", "QUOTE", "BLOCKQUOTE", "SPAN", "CODE":
		return true
	}
	return false
}
