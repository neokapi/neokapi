//go:build js

package pdf

// WasmReader reads PDFs in the browser by delegating to a PDFium WebAssembly
// module loaded as a SIBLING module in the browser's own WASM engine (e.g.
// @embedpdf/pdfium) and exposed on the JS global as __kapiPdfium. PDFium's cgo
// build can't run in the browser, but PDFium *compiled to wasm* runs directly
// there — so this bridges Go-wasm → JS → pdfium.wasm via syscall/js, giving the
// browser build the same positioned-text extraction (text + per-rect geometry)
// as the native kapi-pdfium plugin, and letting us retire the pure-Go
// hand-rolled reader in the browser too.
//
// JS contract (provided by the web app; see register_pdf_js.go and
// strategy/doclang/pdfium-browser-bridge.md):
//
//	globalThis.__kapiPdfium = {
//	  ready: Promise<void>,                   // resolves once pdfium.wasm is loaded
//	  extract(bytes: Uint8Array): Promise<{   // one call per document
//	    pages: {
//	      number: number, height: number,     // page height in points (top-left flip)
//	      rects: { text: string, l, t, r, b: number }[]  // bottom-left coords
//	    }[]
//	  }>,
//	}

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"syscall/js"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/structure"
)

// NewWasmReader constructs the browser PDFium bridge reader.
func NewWasmReader() *WasmReader {
	cfg := &Config{}
	return &WasmReader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "pdf",
			FormatDisplayName: "PDF (PDFium/WASM)",
			FormatMimeType:    "application/pdf",
			FormatExtensions:  []string{".pdf"},
			Cfg:               cfg,
		},
	}
}

// WasmReader implements format.DataFormatReader against a JS-hosted PDFium wasm.
type WasmReader struct {
	format.BaseFormatReader
}

// Signature mirrors the native PDF reader's detection metadata.
func (r *WasmReader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"application/pdf"},
		Extensions: []string{".pdf"},
		MagicBytes: [][]byte{[]byte("%PDF-")},
	}
}

// Open stores the document for reading.
func (r *WasmReader) Open(_ context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("pdf: nil document or reader")
	}
	r.Doc = doc
	return nil
}

// bridge returns the installed __kapiPdfium object, or an error if the web app
// has not loaded the PDFium-wasm bridge into this page.
func bridge() (js.Value, error) {
	g := js.Global().Get("__kapiPdfium")
	if g.IsUndefined() || g.IsNull() {
		return js.Value{}, errors.New("pdf: __kapiPdfium bridge not loaded in this page")
	}
	return g, nil
}

// awaitPromise blocks the calling goroutine until the JS promise settles. It is
// safe because Read does its work in a dedicated goroutine: the receive yields
// to the Go-wasm scheduler so the JS event loop keeps running the then/catch
// callbacks. Returns the resolved value, or an error carrying the rejection.
func awaitPromise(promise js.Value) (js.Value, error) {
	type settled struct {
		val js.Value
		err error
	}
	ch := make(chan settled, 1)

	var then, catch js.Func
	then = js.FuncOf(func(_ js.Value, args []js.Value) any {
		var v js.Value
		if len(args) > 0 {
			v = args[0]
		}
		ch <- settled{val: v}
		return nil
	})
	catch = js.FuncOf(func(_ js.Value, args []js.Value) any {
		msg := "pdf: pdfium.wasm bridge rejected"
		if len(args) > 0 && !args[0].IsUndefined() && !args[0].IsNull() {
			msg = "pdf: " + args[0].Call("toString").String()
		}
		ch <- settled{err: errors.New(msg)}
		return nil
	})
	defer then.Release()
	defer catch.Release()

	promise.Call("then", then).Call("catch", catch)
	s := <-ch
	return s.val, s.err
}

// Read streams Parts from the JS-hosted PDFium: a document Layer, then per page
// a page Layer with one Block per positioned text rect carrying a
// GeometryAnnotation (top-left origin), then the page/document LayerEnds.
func (r *WasmReader) Read(_ context.Context) <-chan model.PartResult {
	ch := make(chan model.PartResult, 64)
	go func() {
		defer close(ch)

		b, err := bridge()
		if err != nil {
			ch <- model.PartResult{Error: err}
			return
		}
		if ready := b.Get("ready"); !ready.IsUndefined() && !ready.IsNull() {
			if _, err := awaitPromise(ready); err != nil {
				ch <- model.PartResult{Error: err}
				return
			}
		}

		data, err := io.ReadAll(r.Doc.Reader)
		if err != nil {
			ch <- model.PartResult{Error: fmt.Errorf("pdf: read document: %w", err)}
			return
		}
		u8 := js.Global().Get("Uint8Array").New(len(data))
		js.CopyBytesToJS(u8, data)

		result, err := awaitPromise(b.Call("extract", u8))
		if err != nil {
			ch <- model.PartResult{Error: err}
			return
		}

		locale := model.LocaleEnglish
		uri := "document.pdf"
		if r.Doc.URI != "" {
			uri = r.Doc.URI
		}
		root := &model.Layer{
			ID: "doc1", Name: uri, Format: "pdf", Locale: locale,
			Encoding: "UTF-8", MimeType: "application/pdf",
		}
		ch <- model.PartResult{Part: &model.Part{Type: model.PartLayerStart, Resource: root}}

		blockCounter := 0
		groupCounter := 0
		pages := result.Get("pages")
		for i := 0; i < pages.Length(); i++ {
			page := pages.Index(i)
			pageNum := page.Get("number").Int()
			pageHeight := page.Get("height").Float()
			rects := page.Get("rects")

			// Collect rects, then merge them into line-level runs (shared with the
			// native plugin via GroupRuns) so a font-change/lone glyph isn't a
			// single-character block.
			n := rects.Length()
			rectsJS := make([]js.Value, n)
			ins := make([]RectIn, n)
			for j := 0; j < n; j++ {
				rc := rects.Index(j)
				rectsJS[j] = rc
				ins[j] = RectIn{Text: rc.Get("text").String(), L: rc.Get("l").Float(), T: rc.Get("t").Float(), R: rc.Get("r").Float(), B: rc.Get("b").Float()}
			}
			blocks := make([]*model.Block, 0, n)
			for _, run := range GroupRuns(ins) {
				if run.Text == "" {
					continue
				}
				blockCounter++
				blk := model.NewBlock(fmt.Sprintf("tu%d", blockCounter), run.Text)
				blk.Properties["page-number"] = strconv.Itoa(pageNum)
				if g := runGeometry(run, rectsJS, pageNum, pageHeight); g != nil {
					blk.SetGeometry(g)
				}
				blocks = append(blocks, blk)
			}
			if len(blocks) == 0 {
				continue
			}

			pageLayer := &model.Layer{
				ID: fmt.Sprintf("page%d", pageNum), Name: fmt.Sprintf("Page %d", pageNum),
				Format: "pdf", Locale: locale,
				Properties: map[string]string{"page-number": strconv.Itoa(pageNum)},
			}
			ch <- model.PartResult{Part: &model.Part{Type: model.PartLayerStart, Resource: pageLayer}}
			// Infer structure (tables, heading/paragraph roles) from the
			// positioned blocks — same analysis as the native plugin.
			for _, p := range structure.ToParts(structure.Analyze(blocks), &groupCounter) {
				ch <- model.PartResult{Part: p}
			}
			ch <- model.PartResult{Part: &model.Part{Type: model.PartLayerEnd, Resource: pageLayer}}
		}

		ch <- model.PartResult{Part: &model.Part{Type: model.PartLayerEnd, Resource: root}}
	}()
	return ch
}

// flipBox converts a PDFium box (bottom-left coords, where "t" is the larger Y)
// to a top-left Rect when the page height is known — the same flip the native
// kapi-pdfium plugin applies. ok is false for a degenerate (zero-area) box.
func flipBox(l, t, r, b, pageHeight float64) (rect model.Rect, topLeft, ok bool) {
	x := math.Min(l, r)
	w := math.Abs(r - l)
	upper := math.Max(t, b)
	lower := math.Min(t, b)
	h := upper - lower
	if w == 0 && h == 0 {
		return model.Rect{}, false, false
	}
	if pageHeight > 0 {
		return model.Rect{X: x, Y: pageHeight - upper, W: w, H: h}, true, true
	}
	return model.Rect{X: x, Y: lower, W: w, H: h}, false, true
}

// runGeometry builds a GeometryAnnotation for a grouped run: the flipped union
// box plus the per-character glyph boxes gathered from the run's member rects.
func runGeometry(run RunOut, rectsJS []js.Value, page int, pageHeight float64) *model.GeometryAnnotation {
	box, topLeft, ok := flipBox(run.L, run.T, run.R, run.B, pageHeight)
	if !ok {
		return nil
	}
	origin := "bottom-left"
	if topLeft {
		origin = "top-left"
	}
	g := &model.GeometryAnnotation{Page: page, BBox: box, Origin: origin}
	for _, mi := range run.Members {
		if mi < 0 || mi >= len(rectsJS) {
			continue
		}
		glyphs := rectsJS[mi].Get("glyphs")
		if glyphs.IsUndefined() || glyphs.IsNull() {
			continue
		}
		for i := 0; i < glyphs.Length(); i++ {
			gl := glyphs.Index(i)
			gbox, _, gok := flipBox(gl.Get("l").Float(), gl.Get("t").Float(), gl.Get("r").Float(), gl.Get("b").Float(), pageHeight)
			if !gok {
				continue
			}
			g.Glyphs = append(g.Glyphs, model.GlyphBox{Text: gl.Get("text").String(), BBox: gbox})
		}
	}
	return g
}

// Close is a no-op; the JS-side PDFium module is owned by the web app.
func (r *WasmReader) Close() error { return nil }
