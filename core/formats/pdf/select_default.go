//go:build !pdfium

package pdf

import "github.com/neokapi/neokapi/core/format"

// RegisteredReader returns the PDF reader registered for the "pdf" format. The
// default build uses the pure-Go hand-rolled text extractor (WASM-safe, no
// dependencies). Build with -tags pdfium to use the PDFium-backed reader.
func RegisteredReader() format.DataFormatReader { return NewReader() }
