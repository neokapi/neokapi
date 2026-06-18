//go:build js

package formats

import (
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/pdf"
	"github.com/neokapi/neokapi/core/registry"
)

// registerPDF registers the browser PDF reader on js builds. PDFium's cgo build
// can't run in the browser, but PDFium compiled to wasm runs there directly, so
// the reader bridges Go-wasm → JS → pdfium.wasm (see core/formats/pdf). It is
// read-only (no writer): editing tools fail cleanly rather than overwriting the
// document with extracted text.
func registerPDF(reg *registry.FormatRegistry) {
	reg.RegisterReader("pdf",
		func() format.DataFormatReader { return pdf.NewWasmReader() },
		format.FormatSignature{
			MIMETypes:  []string{"application/pdf"},
			Extensions: []string{".pdf"},
			MagicBytes: [][]byte{[]byte("%PDF-")},
		}, "PDF (PDFium/WASM)")
}
