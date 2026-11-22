//go:build !onnx

package main

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/plugins/vision/internal/ocr"
	"github.com/neokapi/neokapi/plugins/vision/visionproto"
)

// TestServeHandler_Stub covers the build without ONNX: the handler answers ping
// and info, and reports the missing runtime on each OCR request, so a host can
// probe what the plugin does. A build with the onnx tag has a real engine, which
// needs model files to start, so this file is left out of it.
func TestServeHandler_Stub(t *testing.T) {
	engine, err := ocr.NewEngine(nil)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	h := serveHandler(engine, err)

	if resp := h(visionproto.Request{Op: visionproto.OpPing}, nil); resp.Version == "" {
		t.Error("ping should return a version")
	}
	info := h(visionproto.Request{Op: visionproto.OpInfo}, nil)
	if len(info.Models) == 0 {
		t.Error("info should list model assets")
	}
	ocrResp := h(visionproto.Request{Op: visionproto.OpOCR, Path: "/tmp/x.png"}, nil)
	if ocrResp.Error == "" || !strings.Contains(ocrResp.Error, "ONNX") {
		t.Errorf("stub OCR should report the no-ONNX limitation, got %q", ocrResp.Error)
	}
	if bad := h(visionproto.Request{Op: "nope"}, nil); bad.Error == "" {
		t.Error("unknown op should error")
	}
}
