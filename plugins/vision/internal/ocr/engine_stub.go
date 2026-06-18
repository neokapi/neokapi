//go:build !onnx

package ocr

import "github.com/neokapi/neokapi/plugins/vision/visionproto"

// stubEngine is the default (no-ONNX) build: it answers ping/info via the serve
// loop but cannot recognize text. OCR reports ErrNoONNX so a host can detect
// "installed but no ONNX backend" gracefully.
type stubEngine struct{}

// NewEngine returns the stub engine on the default build.
func NewEngine(_ Logf) (Engine, error) { return &stubEngine{}, nil }

func (*stubEngine) OCR([]byte, string, string) (*visionproto.OCRResult, error) {
	return nil, ErrNoONNX
}
func (*stubEngine) Loaded() bool { return false }
func (*stubEngine) Close() error { return nil }
