// Package pdfreader extracts text (and, optionally, per-segment geometry) from
// PDFs using Google's PDFium via go-pdfium's cgo backend. It runs inside the
// kapi-pdfium plugin process, so a malformed-PDF crash is contained to the
// subprocess. The PDFium pool is process-global and kept warm across documents
// so batch scans (kgrep/kcat/kconv over hundreds of PDFs) amortize init.
package pdfreader

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	pdfium "github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/references"
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/responses"
	"github.com/klippa-app/go-pdfium/single_threaded"

	"github.com/neokapi/neokapi/core/docmeta"
	pdffmt "github.com/neokapi/neokapi/core/formats/pdf"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/structure"
	"github.com/neokapi/neokapi/core/vision"
)

var (
	poolOnce sync.Once
	pool     pdfium.Pool
)

func pdfiumPool() pdfium.Pool {
	poolOnce.Do(func() { pool = single_threaded.Init(single_threaded.Config{}) })
	return pool
}

// Options controls extraction granularity.
type Options struct {
	// Geometry, when true, emits one Block per positioned text rect with a
	// GeometryAnnotation (for the visual editor). When false (the default,
	// fast path for kgrep/kcat/kconv) it emits one Block per page of plain
	// text — fewer allocations, no per-rect position work.
	Geometry bool
	// Glyphs, when true, additionally attaches per-character boxes
	// (GeometryAnnotation.Glyphs) to each text-rect block, for character-precise
	// rendering/highlighting. Implies Geometry. Slightly more work (PDFium "both"
	// mode + char→rect bucketing), so it is opt-in.
	Glyphs bool
	// Tier3, when true, renders each page to a PNG raster (72 DPI, so PDF points
	// map 1:1 to pixels) and emits it as a Media part alongside the page's raw
	// positioned blocks — NOT the plugin's own tier-1/tier-2 structure. The host
	// runs the vision layout model over the raster to produce authoritative tier-3
	// structure (falling back to tier-2 when vision is unavailable). Implies
	// Geometry. The host owns the rendered file and deletes it after use.
	Tier3 bool
}

// renderInstance is the subset of the PDFium instance used to rasterize a page.
type renderInstance interface {
	RenderToFile(*requests.RenderToFile) (*responses.RenderToFile, error)
}

// VisionRasterProperty marks a Media part as a page raster the host's vision
// tier-3 pass should consume (and then delete). It re-exports the shared
// core/vision constant so the plugin producer and the host consumer never drift.
const VisionRasterProperty = vision.PageRasterProperty

// renderPageRaster renders page i to a temp PNG at 72 DPI — at which 1 PDF point
// equals 1 pixel, so the page's text geometry (top-left points) aligns with the
// raster pixels the layout model sees. Returns the file path and pixel size; the
// host reads and removes the file.
func renderPageRaster(inst renderInstance, doc *responses.OpenDocument, i int) (string, int, int, error) {
	res, err := inst.RenderToFile(&requests.RenderToFile{
		RenderPageInDPI: &requests.RenderPageInDPI{
			Page: requests.Page{ByIndex: &requests.PageByIndex{Document: doc.Document, Index: i}},
			DPI:  72,
		},
		OutputFormat: requests.RenderToFileOutputFormatPNG,
		OutputTarget: requests.RenderToFileOutputTargetFile,
	})
	if err != nil {
		return "", 0, 0, fmt.Errorf("pdfium render page %d: %w", i+1, err)
	}
	return res.ImagePath, res.Width, res.Height, nil
}

// ReadParts parses a PDF and returns the ordered Part stream (root Layer,
// per-page Layers, Blocks). The locale is stamped on the layers.
func ReadParts(data []byte, locale model.LocaleID, uri string, opts Options) ([]*model.Part, error) {
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}
	if opts.Glyphs {
		opts.Geometry = true // glyph boxes ride on the per-rect geometry blocks
	}
	if opts.Tier3 {
		opts.Geometry = true // tier-3 structures the per-rect geometry blocks
	}
	inst, err := pdfiumPool().GetInstance(30 * time.Second)
	if err != nil {
		return nil, fmt.Errorf("pdfium instance: %w", err)
	}
	defer inst.Close()

	doc, err := inst.OpenDocument(&requests.OpenDocument{File: &data})
	if err != nil {
		return nil, fmt.Errorf("pdfium open: %w", err)
	}
	defer inst.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document})

	pc, err := inst.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: doc.Document})
	if err != nil {
		return nil, fmt.Errorf("pdfium page count: %w", err)
	}

	var parts []*model.Part
	root := &model.Layer{
		ID: "doc1", Name: uri, Format: "pdf", Locale: locale,
		Encoding: "UTF-8", MimeType: "application/pdf",
	}
	parts = append(parts, &model.Part{Type: model.PartLayerStart, Resource: root})

	// Document metadata (Info dictionary): translatable fields (Title/Subject/
	// Keywords) become metadata-plane Blocks; the rest (author, producer, dates)
	// are recorded as namespaced Properties on the document layer.
	for _, b := range metadataBlocks(inst, doc.Document, root) {
		parts = append(parts, &model.Part{Type: model.PartBlock, Resource: b})
	}

	blockCounter := 0
	groupCounter := 0
	for i := 0; i < pc.PageCount; i++ {
		pageNum := i + 1
		pg := requests.Page{ByIndex: &requests.PageByIndex{Document: doc.Document, Index: i}}

		// In geometry mode, prefer the tagged-PDF structure tree (tier 1): if the
		// page carries a logical structure tree and the experimental MCID API is
		// available, its tables/headings are authoritative. Otherwise we infer
		// structure geometrically from the positioned blocks (tier 2). The fast
		// (non-geometry) path emits whole-page plain text with no structure.
		var pageParts []*model.Part
		if opts.Tier3 {
			// Render the page raster and emit it with the raw positioned blocks;
			// the host's vision pass produces tier-3 structure (or falls back to
			// tier-2). We deliberately do NOT apply the plugin's own structure here.
			pageBlocks, err := readPage(inst, doc, pg, pageNum, opts, &blockCounter)
			if err != nil {
				return nil, err
			}
			if rasterPath, rw, rh, rerr := renderPageRaster(inst, doc, i); rerr == nil && rasterPath != "" {
				pageParts = append(pageParts, &model.Part{Type: model.PartMedia, Resource: &model.Media{
					ID:       fmt.Sprintf("raster%d", pageNum),
					MimeType: "image/png",
					URI:      rasterPath,
					Properties: map[string]string{
						"width":              strconv.Itoa(rw),
						"height":             strconv.Itoa(rh),
						"page-number":        strconv.Itoa(pageNum),
						VisionRasterProperty: "page",
					},
				}})
			}
			for _, b := range pageBlocks {
				pageParts = append(pageParts, &model.Part{Type: model.PartBlock, Resource: b})
			}
		} else if opts.Geometry {
			var pageHeight float64
			if sz, err := inst.FPDF_GetPageSizeByIndex(&requests.FPDF_GetPageSizeByIndex{Document: doc.Document, Index: i}); err == nil {
				pageHeight = sz.Height
			}
			if regions, ok := structRegions(inst, doc.Document, i, pageHeight, &blockCounter); ok {
				pageParts = structure.ToParts(regions, &groupCounter)
			} else {
				pageBlocks, err := readPage(inst, doc, pg, pageNum, opts, &blockCounter)
				if err != nil {
					return nil, err
				}
				pageParts = structure.ToParts(structure.Analyze(pageBlocks), &groupCounter)
			}
		} else {
			pageBlocks, err := readPage(inst, doc, pg, pageNum, opts, &blockCounter)
			if err != nil {
				return nil, err
			}
			for _, b := range pageBlocks {
				pageParts = append(pageParts, &model.Part{Type: model.PartBlock, Resource: b})
			}
		}
		if len(pageParts) == 0 {
			continue
		}
		pageLayer := &model.Layer{
			ID: fmt.Sprintf("page%d", pageNum), Name: fmt.Sprintf("Page %d", pageNum),
			Format: "pdf", Locale: locale,
			Properties: map[string]string{"page-number": strconv.Itoa(pageNum)},
		}
		parts = append(parts, &model.Part{Type: model.PartLayerStart, Resource: pageLayer})
		parts = append(parts, pageParts...)
		parts = append(parts, &model.Part{Type: model.PartLayerEnd, Resource: pageLayer})
	}

	parts = append(parts, &model.Part{Type: model.PartLayerEnd, Resource: root})
	return parts, nil
}

// metaInstance is the subset of the PDFium instance used to read the Info dict.
type metaInstance interface {
	FPDF_GetMetaText(*requests.FPDF_GetMetaText) (*responses.FPDF_GetMetaText, error)
}

// metadataBlocks reads the PDF Info dictionary and maps it onto the content model
// via core/docmeta: Title/Subject/Keywords become translatable metadata-plane
// Blocks; Author/Creator/Producer/CreationDate/ModDate are recorded as
// "pdf:"-namespaced Properties on the document layer (never translated, kept for
// inspection). Empty fields are skipped.
func metadataBlocks(inst metaInstance, docRef references.FPDF_DOCUMENT, root *model.Layer) []*model.Block {
	get := func(tag string) string {
		r, err := inst.FPDF_GetMetaText(&requests.FPDF_GetMetaText{Document: docRef, Tag: tag})
		if err != nil || r == nil {
			return ""
		}
		return strings.TrimSpace(r.Value)
	}
	entries := []docmeta.Entry{
		{Key: "pdf:title", Value: get("Title"), Translatable: true, Role: model.RoleTitle},
		{Key: "pdf:subject", Value: get("Subject"), Translatable: true},
		{Key: "pdf:keywords", Value: get("Keywords"), Translatable: true},
		{Key: "pdf:author", Value: get("Author")},
		{Key: "pdf:creator", Value: get("Creator")},
		{Key: "pdf:producer", Value: get("Producer")},
		{Key: "pdf:creationdate", Value: get("CreationDate")},
		{Key: "pdf:moddate", Value: get("ModDate")},
	}
	return docmeta.Apply(root, entries, "meta")
}

type pdfiumInstance interface {
	GetPageText(*requests.GetPageText) (*responses.GetPageText, error)
	GetPageTextStructured(*requests.GetPageTextStructured) (*responses.GetPageTextStructured, error)
	FPDF_GetPageSizeByIndex(*requests.FPDF_GetPageSizeByIndex) (*responses.FPDF_GetPageSizeByIndex, error)
}

func readPage(inst pdfiumInstance, doc *responses.OpenDocument, pg requests.Page, pageNum int, opts Options, counter *int) ([]*model.Block, error) {
	if !opts.Geometry {
		// Fast path: whole-page plain text in one block.
		pt, err := inst.GetPageText(&requests.GetPageText{Page: pg})
		if err != nil {
			return nil, fmt.Errorf("pdfium page text: %w", err)
		}
		text := strings.TrimSpace(pt.Text)
		if text == "" {
			return nil, nil
		}
		*counter++
		b := model.NewBlock(fmt.Sprintf("tu%d", *counter), text)
		b.Name = fmt.Sprintf("page%d", pageNum)
		b.Properties["page-number"] = strconv.Itoa(pageNum)
		return []*model.Block{b}, nil
	}

	// Geometry path: one block per positioned text rect. In glyph mode we also
	// ask PDFium for chars ("both") and bucket them into their rect.
	var pageHeight float64
	if sz, err := inst.FPDF_GetPageSizeByIndex(&requests.FPDF_GetPageSizeByIndex{Document: doc.Document, Index: pageNum - 1}); err == nil {
		pageHeight = sz.Height
	}
	mode := requests.GetPageTextStructuredModeRects
	if opts.Glyphs {
		mode = requests.GetPageTextStructuredModeBoth
	}
	st, err := inst.GetPageTextStructured(&requests.GetPageTextStructured{Page: pg, Mode: mode})
	if err != nil || st == nil {
		return nil, nil
	}
	// Merge PDFium's per-rect fragments into line-level runs so a font-change or
	// lone glyph doesn't become a single-character block (shared with the wasm
	// bridge via core/formats/pdf.GroupRuns).
	rectIns := make([]pdffmt.RectIn, 0, len(st.Rects))
	for _, rect := range st.Rects {
		p := rect.PointPosition
		rectIns = append(rectIns, pdffmt.RectIn{Text: rect.Text, L: p.Left, T: p.Top, R: p.Right, B: p.Bottom})
	}
	var out []*model.Block
	for _, run := range pdffmt.GroupRuns(rectIns) {
		if run.Text == "" {
			continue
		}
		*counter++
		b := model.NewBlock(fmt.Sprintf("tu%d", *counter), run.Text)
		b.Properties["page-number"] = strconv.Itoa(pageNum)
		pos := responses.CharPosition{Left: run.L, Top: run.T, Right: run.R, Bottom: run.B}
		if g := geometryFromRect(pos, pageNum, pageHeight); g != nil {
			if opts.Glyphs {
				g.Glyphs = glyphsInRect(st.Chars, pos, pageHeight)
			}
			b.SetGeometry(g)
		}
		out = append(out, b)
	}
	return out, nil
}

// glyphsInRect returns the per-character boxes of the chars that fall inside the
// rect, flipped to the same origin as geometryFromRect (top-left when the page
// height is known). A char belongs to the rect when its center lies within the
// rect's bounds (PDF bottom-left coords).
func glyphsInRect(chars []*responses.GetPageTextStructuredChar, rect responses.CharPosition, pageHeight float64) []model.GlyphBox {
	rl := math.Min(rect.Left, rect.Right)
	rr := math.Max(rect.Left, rect.Right)
	rb := math.Min(rect.Top, rect.Bottom)
	rt := math.Max(rect.Top, rect.Bottom)
	var glyphs []model.GlyphBox
	for _, ch := range chars {
		if ch == nil || strings.TrimSpace(ch.Text) == "" {
			continue
		}
		p := ch.PointPosition
		cx := (p.Left + p.Right) / 2
		cy := (p.Top + p.Bottom) / 2
		if cx < rl || cx > rr || cy < rb || cy > rt {
			continue
		}
		x := math.Min(p.Left, p.Right)
		w := math.Abs(p.Right - p.Left)
		upper := math.Max(p.Top, p.Bottom)
		lower := math.Min(p.Top, p.Bottom)
		h := upper - lower
		y := lower
		if pageHeight > 0 {
			y = pageHeight - upper
		}
		glyphs = append(glyphs, model.GlyphBox{Text: ch.Text, BBox: model.Rect{X: x, Y: y, W: w, H: h}})
	}
	return glyphs
}

// geometryFromRect converts a PDFium text-rect (bottom-left coords) to a
// top-left GeometryAnnotation when the page height is known.
func geometryFromRect(p responses.CharPosition, page int, pageHeight float64) *model.GeometryAnnotation {
	x := math.Min(p.Left, p.Right)
	w := math.Abs(p.Right - p.Left)
	upper := math.Max(p.Top, p.Bottom)
	lower := math.Min(p.Top, p.Bottom)
	h := upper - lower
	if w == 0 && h == 0 {
		return nil
	}
	if pageHeight > 0 {
		return &model.GeometryAnnotation{Page: page, BBox: model.Rect{X: x, Y: pageHeight - upper, W: w, H: h}, Origin: "top-left"}
	}
	return &model.GeometryAnnotation{Page: page, BBox: model.Rect{X: x, Y: lower, W: w, H: h}, Origin: "bottom-left"}
}
