// Package ocr is the OCR engine for the kapi-vision plugin. It has two builds,
// exactly like the SaT plugin: the real engine (build tag `onnx`, engine_onnx.go)
// links onnxruntime and runs the RapidOCR / PP-OCRv5 detection and
// recognition models; the stub engine (default build, engine_stub.go) compiles
// with no native dependency and reports ErrNoONNX, so the module builds and the
// protocol/plumbing tests stay green on machines without onnxruntime.
package ocr

import (
	"errors"

	"github.com/neokapi/neokapi/plugins/vision/visionproto"
)

// Engine recognizes text in a page image. Implementations load models lazily and
// are used sequentially by the serve loop.
type Engine interface {
	// OCR recognizes text lines in the PNG/JPEG image at imagePath. The plugin
	// opens the file itself, so the host never holds the image bytes. lang and
	// model are advisory (empty = defaults).
	OCR(imagePath, lang, model string) (*visionproto.OCRResult, error)
	// Layout detects layout regions (roles + boxes + reading order) in the image
	// at imagePath. Path-based like OCR. Returns ErrNoONNX on the stub build.
	Layout(imagePath, lang, model string) (*visionproto.LayoutResult, error)
	// Loaded reports whether the recognition models are resident.
	Loaded() bool
	// Close releases models and the runtime environment.
	Close() error
}

// ErrNoONNX is returned by the stub engine: the binary was built without the
// `onnx` build tag, so no real OCR backend is linked in.
var ErrNoONNX = errors.New("vision: built without ONNX support (rebuild with -tags onnx and the onnxruntime native library)")

// Logf is the plugin's stderr logger, threaded into the engine for progress.
type Logf func(format string, args ...any)
