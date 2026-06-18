//go:build !js

package formats

import "github.com/neokapi/neokapi/core/registry"

// registerPDF is a no-op on native builds: PDF is read by the kapi-pdfium plugin
// (cgo + PDFium), which registers its "pdf" format reader at runtime via the
// plugin format factory. Building it into core would pull cgo + libpdfium into
// every kapi binary and let a malformed PDF crash the process; the plugin keeps
// it isolated. With no plugin installed, PDF is simply an unsupported format.
func registerPDF(*registry.FormatRegistry) {}
