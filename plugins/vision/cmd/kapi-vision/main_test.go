package main

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/plugins/vision/internal/ocr"
	"github.com/neokapi/neokapi/plugins/vision/visionproto"
)

// The serve handler answers ping/info regardless of OCR backend, and on the
// default (stub) build reports the ErrNoONNX limitation per OCR request — so a
// host can probe capability and degrade gracefully.
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
	ocrResp := h(visionproto.Request{Op: visionproto.OpOCR}, []byte("img"))
	if ocrResp.Error == "" || !strings.Contains(ocrResp.Error, "ONNX") {
		t.Errorf("stub OCR should report the no-ONNX limitation, got %q", ocrResp.Error)
	}
	if bad := h(visionproto.Request{Op: "nope"}, nil); bad.Error == "" {
		t.Error("unknown op should error")
	}
}
