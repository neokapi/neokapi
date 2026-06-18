package cli

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/neokapi/neokapi/core/vision"
)

// fakeVisionTransport returns canned OCR without a subprocess.
type fakeVisionTransport struct {
	gotPath string
	gotLang string
	closed  bool
}

func (f *fakeVisionTransport) ocr(imagePath, lang, _ string) (*vision.OCRResult, error) {
	f.gotPath = imagePath
	f.gotLang = lang
	return &vision.OCRResult{
		Width: 200, Height: 100,
		Lines: []vision.OCRLine{{Text: "hello", Confidence: 0.9}},
	}, nil
}
func (f *fakeVisionTransport) Close() error { f.closed = true; return nil }

func TestVisionEngine_OCR(t *testing.T) {
	ft := &fakeVisionTransport{}
	e := &visionEngine{transport: ft}

	res, err := e.OCR(context.Background(), "/tmp/scan.png", vision.OCROptions{Lang: "en"})
	if err != nil {
		t.Fatalf("OCR: %v", err)
	}
	if ft.gotPath != "/tmp/scan.png" || ft.gotLang != "en" {
		t.Errorf("transport got path=%q lang=%q", ft.gotPath, ft.gotLang)
	}
	if len(res.Lines) != 1 || res.Lines[0].Text != "hello" {
		t.Fatalf("OCR result = %+v", res)
	}
	if err := e.Close(); err != nil || !ft.closed {
		t.Errorf("Close err=%v closed=%v", err, ft.closed)
	}
}

// The duplicated framing must round-trip a request (header + binary payload) and
// a response, matching the documented visionproto-framed-v1 wire shape.
func TestVisionFraming_RoundTrip(t *testing.T) {
	var reqBuf bytes.Buffer
	img := bytes.Repeat([]byte{0x89, 'P', 'N', 'G'}, 500)
	if err := visionWriteMessage(&reqBuf, visionRequest{Op: "ocr", Lang: "en"}, img); err != nil {
		t.Fatal(err)
	}
	// Read the request as the plugin would: header frame then payload frame.
	hb, err := visionReadFrame(&reqBuf)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(hb, []byte(`"op":"ocr"`)) {
		t.Errorf("header frame = %s", hb)
	}
	pb, err := visionReadFrame(&reqBuf)
	if err != nil || !bytes.Equal(pb, img) {
		t.Fatalf("payload frame mismatch: %d vs %d (err=%v)", len(pb), len(img), err)
	}

	// Now a response round-trip.
	var respBuf bytes.Buffer
	if err := visionWriteMessage(&respBuf, visionResponse{
		OCR: &visionOCRResult{Width: 10, Height: 5, Lines: []visionOCRLine{{Text: "x", W: 3}}},
	}, nil); err != nil {
		t.Fatal(err)
	}
	resp, err := visionReadResponse(&respBuf)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if resp.OCR == nil || len(resp.OCR.Lines) != 1 || resp.OCR.Lines[0].Text != "x" {
		t.Fatalf("response = %+v", resp)
	}
	if _, err := visionReadFrame(&respBuf); err != io.EOF {
		t.Errorf("expected clean EOF after response, got %v", err)
	}
}
