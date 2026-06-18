//go:build onnx

package ocr

import (
	"os"
	"strings"
	"testing"
)

// TestOCRSmoke runs the real ONNX OCR pipeline end to end against the committed
// hello.png fixture and asserts it reads the text. It needs the native
// onnxruntime library and the model assets, so it is skipped unless both
// KAPI_VISION_ORT_LIB and KAPI_VISION_MODELS_DIR are set:
//
//	KAPI_VISION_ORT_LIB=/path/to/libonnxruntime.dylib \
//	KAPI_VISION_MODELS_DIR=/path/to/models \
//	GOWORK=off CGO_ENABLED=1 go test -tags onnx ./internal/ocr/ -run Smoke -v
//
// The models dir must contain the PP-OCRv4 det/rec/cls .onnx files and
// ppocr_keys_v1.txt (see internal/models). In normal CI (no native lib) it skips.
func TestOCRSmoke(t *testing.T) {
	if os.Getenv("KAPI_VISION_ORT_LIB") == "" || os.Getenv("KAPI_VISION_MODELS_DIR") == "" {
		t.Skip("set KAPI_VISION_ORT_LIB and KAPI_VISION_MODELS_DIR to run the OCR smoke test")
	}
	eng, err := NewEngine(func(string, ...any) {})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer func() { _ = eng.Close() }()

	res, err := eng.OCR("testdata/hello.png", "", "")
	if err != nil {
		t.Fatalf("OCR: %v", err)
	}
	if res == nil || len(res.Lines) == 0 {
		t.Fatal("OCR returned no lines for hello.png")
	}
	var all strings.Builder
	for _, ln := range res.Lines {
		all.WriteString(ln.Text)
		all.WriteByte(' ')
	}
	got := all.String()
	// The recognizer occasionally drops inter-word spaces, so check the words
	// independently rather than the exact "Hello World".
	if !strings.Contains(got, "Hello") || !strings.Contains(strings.ReplaceAll(got, " ", ""), "World") {
		t.Errorf("OCR text = %q, want it to contain Hello and World", got)
	}
}
