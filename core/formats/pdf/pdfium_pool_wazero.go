//go:build pdfium && !pdfium_cgo

// Wazero backend: PDFium runs inside a pure-Go WebAssembly runtime embedded in
// the binary (no cgo, no shared library to ship). This is the default under
// -tags pdfium. Swap to the cgo single-threaded backend with -tags pdfium_cgo.
package pdf

import (
	"sync"

	pdfium "github.com/klippa-app/go-pdfium"
	"github.com/klippa-app/go-pdfium/webassembly"
)

const pdfiumBackend = "wazero"

var (
	poolOnce sync.Once
	pdfPool  pdfium.Pool
	poolErr  error
)

func pdfiumPool() (pdfium.Pool, error) {
	poolOnce.Do(func() {
		pdfPool, poolErr = webassembly.Init(webassembly.Config{MinIdle: 1, MaxIdle: 1, MaxTotal: 4})
	})
	return pdfPool, poolErr
}
