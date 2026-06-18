package visionproto

import (
	"bytes"
	"io"
	"testing"
)

// serveLoopback wires a Serve goroutine to a Client over two in-memory pipes,
// returning the client and a stop func.
func serveLoopback(t *testing.T, h Handler) (*Client, func()) {
	t.Helper()
	inR, inW := io.Pipe()   // host → plugin stdin
	outR, outW := io.Pipe() // plugin → host stdout
	done := make(chan struct{})
	go func() {
		_ = Serve(inR, outW, h)
		close(done)
	}()
	return NewClient(inW, outR), func() {
		_ = inW.Close()
		<-done
		_ = outW.Close()
	}
}

func TestRoundTrip(t *testing.T) {
	gotPath := ""
	h := func(req Request, _ []byte) Response {
		switch req.Op {
		case OpPing:
			return Response{Version: "test-1"}
		case OpInfo:
			return Response{Version: "test-1", Models: []ModelInfo{{Name: "ppocrv4", Default: true, Loaded: true}}}
		case OpOCR:
			gotPath = req.Path
			return Response{OCR: &OCRResult{
				Width: 100, Height: 40,
				Lines: []OCRLine{{Text: "hi " + req.Lang, X: 1, Y: 2, W: 30, H: 10, Confidence: 0.9}},
			}}
		default:
			return Response{Error: "unknown op"}
		}
	}
	c, stop := serveLoopback(t, h)
	defer stop()

	// ping
	resp, err := c.Do(Request{Op: OpPing}, nil)
	if err != nil || resp.Version != "test-1" {
		t.Fatalf("ping = %+v err=%v", resp, err)
	}
	// info
	resp, err = c.Do(Request{Op: OpInfo}, nil)
	if err != nil || len(resp.Models) != 1 || !resp.Models[0].Default {
		t.Fatalf("info = %+v err=%v", resp, err)
	}
	// ocr by path (no image payload — the plugin opens the file itself)
	resp, err = c.Do(Request{Op: OpOCR, Path: "/tmp/scan.png", Lang: "en"}, nil)
	if err != nil {
		t.Fatalf("ocr err: %v", err)
	}
	if gotPath != "/tmp/scan.png" {
		t.Errorf("path not transported: got %q", gotPath)
	}
	if resp.OCR == nil || len(resp.OCR.Lines) != 1 || resp.OCR.Lines[0].Text != "hi en" {
		t.Fatalf("ocr response = %+v", resp.OCR)
	}
}

func TestFrameRoundTrip_EmptyAndLarge(t *testing.T) {
	var buf bytes.Buffer
	// header + empty payload
	if err := WriteMessage(&buf, Request{Op: OpPing}, nil); err != nil {
		t.Fatal(err)
	}
	// header + large payload
	big := bytes.Repeat([]byte("x"), 1<<20) // 1 MB
	if err := WriteMessage(&buf, Request{Op: OpOCR}, big); err != nil {
		t.Fatal(err)
	}

	req, payload, err := ReadRequest(&buf)
	if err != nil || req.Op != OpPing || payload != nil {
		t.Fatalf("first msg: req=%+v payload=%d err=%v", req, len(payload), err)
	}
	req, payload, err = ReadRequest(&buf)
	if err != nil || req.Op != OpOCR || len(payload) != 1<<20 {
		t.Fatalf("second msg: req=%+v payload=%d err=%v", req, len(payload), err)
	}
	// clean EOF at boundary
	if _, _, err := ReadRequest(&buf); err != io.EOF {
		t.Errorf("expected EOF at boundary, got %v", err)
	}
}

func TestServe_ErrorResponse(t *testing.T) {
	c, stop := serveLoopback(t, func(req Request, _ []byte) Response {
		return Response{Error: "boom"}
	})
	defer stop()
	resp, err := c.Do(Request{Op: "weird"}, nil)
	if err != nil {
		t.Fatalf("transport err: %v", err)
	}
	if resp.Error != "boom" {
		t.Errorf("error response = %q, want boom", resp.Error)
	}
}

func TestRoundTrip_Layout(t *testing.T) {
	var gotPath string
	c, stop := serveLoopback(t, func(req Request, _ []byte) Response {
		if req.Op != OpLayout {
			return Response{Error: "unexpected op"}
		}
		gotPath = req.Path
		return Response{Layout: &LayoutResult{Width: 200, Height: 100, Regions: []Region{
			{Role: "heading", X: 0, Y: 0, W: 200, H: 20, ReadingOrder: 0, Confidence: 0.98},
			{Role: "table", X: 0, Y: 30, W: 200, H: 60, ReadingOrder: 1, Confidence: 0.91},
		}}}
	})
	defer stop()
	resp, err := c.Do(Request{Op: OpLayout, Path: "/tmp/page.png"}, nil)
	if err != nil {
		t.Fatalf("layout err: %v", err)
	}
	if gotPath != "/tmp/page.png" {
		t.Errorf("path not transported: %q", gotPath)
	}
	if resp.Layout == nil || len(resp.Layout.Regions) != 2 || resp.Layout.Regions[1].Role != "table" {
		t.Fatalf("layout response = %+v", resp.Layout)
	}
}
