//go:build pdfium

// PDFium-backed PDF reader (built only under -tags pdfium). Unlike the pure-Go
// hand-rolled Reader, it drives Google's PDFium engine via go-pdfium, so it
// handles font encodings/CID/CJK correctly and returns per-segment positioned
// text — each text rect becomes a Block carrying a GeometryAnnotation. The
// backend (wazero by default, cgo single-threaded under -tags pdfium_cgo) is
// chosen by pdfiumPool() in a sub-tagged file, so the cgo-static backend is a
// drop-in swap with no change here.
package pdf

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/klippa-app/go-pdfium/requests"
	"github.com/klippa-app/go-pdfium/responses"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// PdfiumReader implements format.DataFormatReader on top of PDFium.
type PdfiumReader struct {
	format.BaseFormatReader
	cfg *Config
}

// NewPdfiumReader creates a PDFium-backed PDF reader.
func NewPdfiumReader() *PdfiumReader {
	cfg := &Config{}
	return &PdfiumReader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "pdf",
			FormatDisplayName: "PDF Text Extraction (PDFium)",
			FormatMimeType:    "application/pdf",
			FormatExtensions:  []string{".pdf"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// Signature returns detection metadata (identical to the hand-rolled reader).
func (r *PdfiumReader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"application/pdf"},
		Extensions: []string{".pdf"},
		MagicBytes: [][]byte{[]byte("%PDF-")},
	}
}

// Open stores the document for reading.
func (r *PdfiumReader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("pdf: nil document or reader")
	}
	r.Doc = doc
	return nil
}

// Read returns a channel of PartResults.
func (r *PdfiumReader) Read(ctx context.Context) <-chan model.PartResult {
	ch := make(chan model.PartResult, 64)
	go func() {
		defer close(ch)
		r.readContent(ctx, ch)
	}()
	return ch
}

// Close is a no-op; the PDFium pool is package-global and long-lived.
func (r *PdfiumReader) Close() error { return nil }

func (r *PdfiumReader) emit(ctx context.Context, ch chan<- model.PartResult, part *model.Part) bool {
	select {
	case ch <- model.PartResult{Part: part}:
		return true
	case <-ctx.Done():
		return false
	}
}

func (r *PdfiumReader) fail(ch chan<- model.PartResult, err error) {
	ch <- model.PartResult{Error: err}
}

func (r *PdfiumReader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	data, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		r.fail(ch, fmt.Errorf("pdf: reading: %w", err))
		return
	}

	pool, err := pdfiumPool()
	if err != nil {
		r.fail(ch, fmt.Errorf("pdf: pdfium pool (%s): %w", pdfiumBackend, err))
		return
	}
	inst, err := pool.GetInstance(30 * time.Second)
	if err != nil {
		r.fail(ch, fmt.Errorf("pdf: pdfium instance: %w", err))
		return
	}
	defer inst.Close()

	doc, err := inst.OpenDocument(&requests.OpenDocument{File: &data})
	if err != nil {
		r.fail(ch, fmt.Errorf("pdf: pdfium open: %w", err))
		return
	}
	defer inst.FPDF_CloseDocument(&requests.FPDF_CloseDocument{Document: doc.Document})

	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	root := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "pdf",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "application/pdf",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: root}) {
		return
	}

	pc, err := inst.FPDF_GetPageCount(&requests.FPDF_GetPageCount{Document: doc.Document})
	if err != nil {
		r.fail(ch, fmt.Errorf("pdf: pdfium page count: %w", err))
		return
	}

	blockCounter := 0
	for i := 0; i < pc.PageCount; i++ {
		pageNum := i + 1
		pg := requests.Page{ByIndex: &requests.PageByIndex{Document: doc.Document, Index: i}}

		// Page height (points) for the bottom-left → top-left flip below.
		var pageHeight float64
		if sz, err := inst.FPDF_GetPageSizeByIndex(&requests.FPDF_GetPageSizeByIndex{Document: doc.Document, Index: i}); err == nil {
			pageHeight = sz.Height
		}

		st, err := inst.GetPageTextStructured(&requests.GetPageTextStructured{
			Page: pg,
			Mode: requests.GetPageTextStructuredModeRects,
		})
		if err != nil || st == nil || len(st.Rects) == 0 {
			continue
		}

		pageLayer := &model.Layer{
			ID:         fmt.Sprintf("page%d", pageNum),
			Name:       fmt.Sprintf("Page %d", pageNum),
			Format:     "pdf",
			Locale:     locale,
			Properties: map[string]string{"page-number": strconv.Itoa(pageNum)},
		}
		if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: pageLayer}) {
			return
		}

		for _, rect := range st.Rects {
			text := strings.TrimSpace(rect.Text)
			if text == "" {
				continue
			}
			blockCounter++
			block := model.NewBlock(fmt.Sprintf("tu%d", blockCounter), text)
			block.Properties["page-number"] = strconv.Itoa(pageNum)
			if g := geometryFromRect(rect.PointPosition, pageNum, pageHeight); g != nil {
				block.SetGeometry(g)
			}
			if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
				return
			}
		}

		if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: pageLayer}) {
			return
		}
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: root})
}

// geometryFromRect converts a PDFium text-rect position into a
// GeometryAnnotation. PDFium reports coordinates in PDF-native bottom-left
// space (Top is the upper edge, the larger y); we flip to the top-left
// convention the other readers use when the page height is known, falling back
// to bottom-left origin otherwise. Units are points (Resolution 0 = absolute).
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
		return &model.GeometryAnnotation{
			Page:   page,
			BBox:   model.Rect{X: x, Y: pageHeight - upper, W: w, H: h},
			Origin: "top-left",
		}
	}
	return &model.GeometryAnnotation{
		Page:   page,
		BBox:   model.Rect{X: x, Y: lower, W: w, H: h},
		Origin: "bottom-left",
	}
}
