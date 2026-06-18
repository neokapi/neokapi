// Package visionproto is the line-independent, binary-framed stdin/stdout
// protocol the kapi-vision plugin speaks. Unlike the SaT segmenter's
// line-delimited JSON, vision requests carry raw image bytes (MB-scale), which
// would blow a bufio.Scanner line buffer and waste 33% on base64 — so each
// message is length-prefixed binary: a JSON header frame followed by a binary
// payload frame.
//
// Wire format (both directions), big-endian:
//
//	[uint32 headerLen][header JSON bytes][uint32 payloadLen][payload bytes]
//
// The header is a Request (host→plugin) or Response (plugin→host). The payload
// is the image bytes for an "ocr" request, and empty (payloadLen 0) otherwise.
// The package imports only the standard library so a host can speak the protocol
// without inheriting the plugin's native (onnxruntime) build requirements.
package visionproto

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// Op is a request operation.
const (
	OpPing = "ping" // liveness + version
	OpInfo = "info" // capabilities + loaded models
	OpOCR  = "ocr"  // recognize text in the payload image
)

// Request is the header for one operation. For OpOCR the image bytes travel in
// the message payload frame, not here.
type Request struct {
	Op    string `json:"op"`
	Lang  string `json:"lang,omitempty"`  // advisory language hint
	Model string `json:"model,omitempty"` // empty = default
}

// Response is the header for one result. Exactly one of OCR/Models/error is set
// per the request op; OK/Version answer ping.
type Response struct {
	OK      string      `json:"ok,omitempty"` // unused except symmetry; ping uses Version
	Version string      `json:"version,omitempty"`
	Error   string      `json:"error,omitempty"`
	OCR     *OCRResult  `json:"ocr,omitempty"`
	Models  []ModelInfo `json:"models,omitempty"`
}

// OCRResult is the recognized text of one image plus its pixel size.
type OCRResult struct {
	Width  int       `json:"width"`
	Height int       `json:"height"`
	Lines  []OCRLine `json:"lines"`
}

// OCRLine is one recognized text line: top-left pixel box + confidence [0,1].
type OCRLine struct {
	Text       string  `json:"text"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	W          float64 `json:"w"`
	H          float64 `json:"h"`
	Confidence float64 `json:"confidence"`
}

// ModelInfo describes a model the plugin supports.
type ModelInfo struct {
	Name    string `json:"name"`
	Default bool   `json:"default,omitempty"`
	Loaded  bool   `json:"loaded,omitempty"`
}

// maxFrame caps a single frame at 256 MB — generous for any page image, a guard
// against a corrupt length header allocating unbounded memory.
const maxFrame = 256 << 20

// WriteMessage writes one framed message: the header (marshaled to JSON) then
// the payload. payload may be nil.
func WriteMessage(w io.Writer, header any, payload []byte) error {
	hb, err := json.Marshal(header)
	if err != nil {
		return fmt.Errorf("visionproto: marshal header: %w", err)
	}
	if err := writeFrame(w, hb); err != nil {
		return err
	}
	return writeFrame(w, payload)
}

func writeFrame(w io.Writer, b []byte) error {
	var lenbuf [4]byte
	binary.BigEndian.PutUint32(lenbuf[:], uint32(len(b)))
	if _, err := w.Write(lenbuf[:]); err != nil {
		return err
	}
	if len(b) > 0 {
		if _, err := w.Write(b); err != nil {
			return err
		}
	}
	return nil
}

// readFrame reads one length-prefixed frame. It returns io.EOF cleanly when the
// stream ends at a frame boundary (no partial header read).
func readFrame(r io.Reader) ([]byte, error) {
	var lenbuf [4]byte
	if _, err := io.ReadFull(r, lenbuf[:]); err != nil {
		return nil, err // io.EOF at a clean boundary
	}
	n := binary.BigEndian.Uint32(lenbuf[:])
	if n > maxFrame {
		return nil, fmt.Errorf("visionproto: frame too large (%d bytes)", n)
	}
	if n == 0 {
		return nil, nil
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf("visionproto: short frame: %w", err)
	}
	return buf, nil
}

// ReadRequest reads one request message (header + payload). The payload is the
// image bytes for OpOCR, nil otherwise.
func ReadRequest(r io.Reader) (Request, []byte, error) {
	hb, err := readFrame(r)
	if err != nil {
		return Request{}, nil, err
	}
	var req Request
	if err := json.Unmarshal(hb, &req); err != nil {
		return Request{}, nil, fmt.Errorf("visionproto: decode request: %w", err)
	}
	payload, err := readFrame(r)
	if err != nil {
		return Request{}, nil, err
	}
	return req, payload, nil
}

// ReadResponse reads one response message (a header frame + an empty payload
// frame, which it discards).
func ReadResponse(r io.Reader) (Response, error) {
	hb, err := readFrame(r)
	if err != nil {
		return Response{}, err
	}
	var resp Response
	if err := json.Unmarshal(hb, &resp); err != nil {
		return Response{}, fmt.Errorf("visionproto: decode response: %w", err)
	}
	if _, err := readFrame(r); err != nil { // trailing (empty) payload frame
		return Response{}, err
	}
	return resp, nil
}

// Handler answers one request; payload is the image bytes for OpOCR.
type Handler func(req Request, payload []byte) Response

// Serve runs the plugin-side read/dispatch/write loop until r reaches EOF. Each
// request is answered with a response message (empty payload). A malformed frame
// ends the loop with an error.
func Serve(r io.Reader, w io.Writer, h Handler) error {
	br := bufio.NewReader(r)
	bw := bufio.NewWriter(w)
	for {
		req, payload, err := ReadRequest(br)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		resp := h(req, payload)
		if err := WriteMessage(bw, resp, nil); err != nil {
			return err
		}
		if err := bw.Flush(); err != nil {
			return err
		}
	}
}

// Client drives a plugin process over its stdin/stdout pipes. It serializes
// round-trips, so it is safe for concurrent callers.
type Client struct {
	mu sync.Mutex
	w  io.Writer
	r  *bufio.Reader
}

// NewClient wraps a plugin's stdin (w) and stdout (r).
func NewClient(w io.Writer, r io.Reader) *Client {
	return &Client{w: w, r: bufio.NewReader(r)}
}

// Do sends one request (with optional image payload) and returns the response.
func (c *Client) Do(req Request, payload []byte) (Response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := WriteMessage(c.w, req, payload); err != nil {
		return Response{}, err
	}
	if f, ok := c.w.(interface{ Flush() error }); ok {
		if err := f.Flush(); err != nil {
			return Response{}, err
		}
	}
	return ReadResponse(c.r)
}
