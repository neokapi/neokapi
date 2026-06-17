//go:build pdfium

package pdf

import "github.com/neokapi/neokapi/core/format"

// RegisteredReader returns the PDFium-backed reader under -tags pdfium: correct
// font-encoding/CID/CJK extraction plus per-segment positioned geometry.
func RegisteredReader() format.DataFormatReader { return NewPdfiumReader() }
