//go:build pdfium && pdfium_cgo

// Cgo backend: PDFium linked natively (statically, alongside ICU). Faster than
// wazero; requires a libpdfium archive on the pkg-config path at build time.
// Selected with -tags "pdfium pdfium_cgo".
package pdf

import (
	"sync"

	pdfium "github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/single_threaded"
)

const pdfiumBackend = "cgo"

var (
	poolOnce sync.Once
	pdfPool  pdfium.Pool
)

func pdfiumPool() (pdfium.Pool, error) {
	poolOnce.Do(func() {
		pdfPool = single_threaded.Init(single_threaded.Config{})
	})
	return pdfPool, nil
}
