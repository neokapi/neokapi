//go:build js

package pdf

// WasmReader [SCAFFOLD] reads PDFs in the browser by delegating to a PDFium
// WebAssembly module loaded as a SIBLING module in the browser's own WASM
// engine (e.g. @embedpdf/pdfium / pdfium.js) and exposed on the JS global as
// __kapiPdfium. PDFium's cgo build can't run in the browser, but PDFium
// *compiled to wasm* runs directly there — so this bridges Go-wasm → JS →
// pdfium.wasm via syscall/js, giving the browser build the SAME correct text
// extraction (incl. CID/Type0/CJK) as the native kapi-pdfium plugin, and
// letting us retire the pure-Go hand-rolled reader in the browser too.
//
// JS contract (provided by the web app; see
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
//
// Status: scaffold. It IS the registered browser pdf reader (register_pdf_js.go
// wires it on js builds, replacing the retired hand-rolled in-core reader), but
// the async Promise→Go glue and the pages/rects→Part mapping are still TODO;
// until they land this reader reports that the bridge is unavailable/not-wired
// so callers fail clearly rather than silently extracting nothing.

import (
	"context"
	"errors"
	"syscall/js"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// errBridgeNotWired is returned until the Promise glue + Part mapping land.
var errBridgeNotWired = errors.New("pdf: browser pdfium.wasm bridge not yet wired (scaffold)")

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

// bridgeAvailable reports whether the web app has installed the PDFium bridge.
func bridgeAvailable() bool {
	g := js.Global().Get("__kapiPdfium")
	return !g.IsUndefined() && !g.IsNull()
}

// Read streams Parts from the JS-hosted PDFium. Scaffold: reports
// unavailability/not-wired so failures are explicit.
//
// TODO(pdfium-wasm-bridge):
//  1. await globalThis.__kapiPdfium.ready (Promise→chan via a js.Func callback).
//  2. copy r.Doc bytes into a Uint8Array, call extract(bytes), await the result.
//  3. map pages/rects → Layer/page-Layer/Block(+GeometryAnnotation), reusing the
//     same bottom-left→top-left flip as the native plugin reader.
func (r *WasmReader) Read(_ context.Context) <-chan model.PartResult {
	ch := make(chan model.PartResult, 1)
	go func() {
		defer close(ch)
		if !bridgeAvailable() {
			ch <- model.PartResult{Error: errors.New("pdf: __kapiPdfium bridge not loaded in this page")}
			return
		}
		ch <- model.PartResult{Error: errBridgeNotWired}
	}()
	return ch
}

// Close is a no-op; the JS-side PDFium module is owned by the web app.
func (r *WasmReader) Close() error { return nil }
