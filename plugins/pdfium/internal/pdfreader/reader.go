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
	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/responses"
	"github.com/klippa-app/go-pdfium/single_threaded"

	"github.com/neokapi/neokapi/core/model"
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
}

// ReadParts parses a PDF and returns the ordered Part stream (root Layer,
// per-page Layers, Blocks). The locale is stamped on the layers.
func ReadParts(data []byte, locale model.LocaleID, uri string, opts Options) ([]*model.Part, error) {
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
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

	blockCounter := 0
	for i := 0; i < pc.PageCount; i++ {
		pageNum := i + 1
		pg := requests.Page{ByIndex: &requests.PageByIndex{Document: doc.Document, Index: i}}

		pageBlocks, err := readPage(inst, doc, pg, pageNum, opts, &blockCounter)
		if err != nil {
			return nil, err
		}
		if len(pageBlocks) == 0 {
			continue
		}
		pageLayer := &model.Layer{
			ID: fmt.Sprintf("page%d", pageNum), Name: fmt.Sprintf("Page %d", pageNum),
			Format: "pdf", Locale: locale,
			Properties: map[string]string{"page-number": strconv.Itoa(pageNum)},
		}
		parts = append(parts, &model.Part{Type: model.PartLayerStart, Resource: pageLayer})
		for _, b := range pageBlocks {
			parts = append(parts, &model.Part{Type: model.PartBlock, Resource: b})
		}
		parts = append(parts, &model.Part{Type: model.PartLayerEnd, Resource: pageLayer})
	}

	parts = append(parts, &model.Part{Type: model.PartLayerEnd, Resource: root})
	return parts, nil
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

	// Geometry path: one block per positioned text rect.
	var pageHeight float64
	if sz, err := inst.FPDF_GetPageSizeByIndex(&requests.FPDF_GetPageSizeByIndex{Document: doc.Document, Index: pageNum - 1}); err == nil {
		pageHeight = sz.Height
	}
	st, err := inst.GetPageTextStructured(&requests.GetPageTextStructured{Page: pg, Mode: requests.GetPageTextStructuredModeRects})
	if err != nil || st == nil {
		return nil, nil
	}
	var out []*model.Block
	for _, rect := range st.Rects {
		text := strings.TrimSpace(rect.Text)
		if text == "" {
			continue
		}
		*counter++
		b := model.NewBlock(fmt.Sprintf("tu%d", *counter), text)
		b.Properties["page-number"] = strconv.Itoa(pageNum)
		if g := geometryFromRect(rect.PointPosition, pageNum, pageHeight); g != nil {
			b.SetGeometry(g)
		}
		out = append(out, b)
	}
	return out, nil
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
