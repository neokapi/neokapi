//go:build onnx

package ocr

import (
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/model"
)

// TestLayoutSmoke runs the real PP-DocLayoutV3 layout pipeline end to end against
// the committed doc.png fixture (a rendered report: title, paragraph, heading,
// table, paragraph). It needs the native onnxruntime library and the layout
// model, so it is skipped unless both KAPI_VISION_ORT_LIB and
// KAPI_VISION_MODELS_DIR (containing ppdoclayoutv3.onnx) are set:
//
//	KAPI_VISION_ORT_LIB=/path/to/libonnxruntime.dylib \
//	KAPI_VISION_MODELS_DIR=/path/to/models \
//	GOWORK=off CGO_ENABLED=1 go test -tags onnx ./internal/ocr/ -run LayoutSmoke -v
func TestLayoutSmoke(t *testing.T) {
	if os.Getenv("KAPI_VISION_ORT_LIB") == "" || os.Getenv("KAPI_VISION_MODELS_DIR") == "" {
		t.Skip("set KAPI_VISION_ORT_LIB and KAPI_VISION_MODELS_DIR to run the layout smoke test")
	}
	eng, err := NewEngine(func(string, ...any) {})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	defer func() { _ = eng.Close() }()

	res, err := eng.Layout("testdata/doc.png", "", "")
	if err != nil {
		t.Fatalf("Layout: %v", err)
	}
	if res == nil || len(res.Regions) < 3 {
		t.Fatalf("layout returned %d regions, want >= 3", len(res.Regions))
	}
	roles := map[string]bool{}
	for _, r := range res.Regions {
		roles[r.Role] = true
		if r.W <= 0 || r.H <= 0 {
			t.Errorf("region has non-positive size: %+v", r)
		}
		if r.X < 0 || r.Y < 0 || r.X+r.W > float64(res.Width)+2 || r.Y+r.H > float64(res.Height)+2 {
			t.Errorf("region out of image bounds (%dx%d): %+v", res.Width, res.Height, r)
		}
	}
	// The fixture has a heading and a table; both must be detected.
	if !roles[model.RoleHeading] && !roles[model.RoleTitle] {
		t.Errorf("expected a heading/title region; roles=%v", roles)
	}
	if !roles[model.RoleTable] {
		t.Errorf("expected a table region; roles=%v", roles)
	}
}
