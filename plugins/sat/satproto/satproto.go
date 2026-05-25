// Package satproto defines the line-delimited JSON protocol spoken between a
// host process and the kapi-sat segmenter plugin over the plugin's stdin and
// stdout.
//
// The protocol is deliberately tiny and dependency-free: it contains only
// plain Go structs plus encode/decode helpers and a small client driver. It
// imports nothing beyond the standard library — in particular it has no cgo,
// no ONNX runtime, and no tokenizer dependency. This lets the host-side `sat`
// segment engine import this package to talk to the plugin without inheriting
// the plugin's heavy native build requirements.
//
// # Wire format
//
// Each message is a single JSON object on its own line ('\n'-terminated), in
// both directions. The host writes a [Request] per line on the plugin's stdin
// and reads a [Response] per line from the plugin's stdout. The plugin keeps
// running across many requests (a read loop on stdin), so a single spawned
// process serves an unbounded sequence of requests until stdin closes.
//
// Requests carry a monotonically increasing id chosen by the host; the plugin
// echoes that id in the matching response. Responses may arrive in any order
// relative to requests if the plugin chooses to process concurrently, so the
// host must correlate by id. (The reference plugin processes serially, but the
// protocol does not require it.)
//
// # Boundaries
//
// A segment response reports interior sentence boundaries as RUNE offsets into
// the exact request text. A boundary at rune offset i means "a new sentence
// begins at text[i]" (counting in runes, not bytes). Offsets 0 and len(runes)
// are never reported — only the interior cut points. The host reconstructs
// segments by slicing the original text at these rune offsets.
//
// # Operations
//
//	{"id":1,"op":"segment","text":"Hello world. How are you?","lang":"en","model":"sat-3l-sm","threshold":0.25}
//	  -> {"id":1,"boundaries":[13]}
//
//	{"op":"ping"}
//	  -> {"ok":true,"version":"0.1.0"}
//
//	{"op":"info"}
//	  -> {"models":[{"name":"sat-3l-sm","loaded":true},{"name":"sat-12l-sm","loaded":false}],"version":"0.1.0"}
//
// Errors are reported in-band: a response carrying a non-empty error field
// describes a failure for the request with the matching id. The plugin should
// stay alive after an error so the host can continue issuing requests.
package satproto

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

// Op identifies a request operation.
type Op string

const (
	// OpSegment requests sentence segmentation of Request.Text.
	OpSegment Op = "segment"
	// OpPing is a liveness/version check. It carries no payload.
	OpPing Op = "ping"
	// OpInfo asks the plugin to enumerate the models it supports and which
	// are currently loaded into memory.
	OpInfo Op = "info"
)

// Request is one line-delimited JSON request from host to plugin.
type Request struct {
	// ID correlates this request with its [Response]. The host chooses a
	// value (typically monotonically increasing); the plugin echoes it back.
	// Ping/info requests may omit it (zero), in which case the matching
	// response also carries id 0.
	ID int64 `json:"id,omitempty"`

	// Op selects the operation. Required.
	Op Op `json:"op"`

	// Text is the input to segment. Used only by OpSegment.
	Text string `json:"text,omitempty"`

	// Lang is an optional BCP-47 / ISO-639 hint (e.g. "en"). SaT is a
	// multilingual model and does not require a language; the hint is
	// currently advisory and reserved for future per-language tuning.
	Lang string `json:"lang,omitempty"`

	// Model selects which SaT model to use (e.g. "sat-3l-sm",
	// "sat-12l-sm"). Empty means the plugin's default model.
	Model string `json:"model,omitempty"`

	// Threshold is the sentence-boundary probability cutoff in (0,1).
	// Zero means "use the model's default" (0.25 for the *-sm models).
	Threshold float64 `json:"threshold,omitempty"`
}

// Response is one line-delimited JSON response from plugin to host.
//
// Exactly one of three shapes is populated depending on the request:
//   - OpSegment success: Boundaries set (possibly empty), Error empty.
//   - OpPing success:    OK true, Version set.
//   - OpInfo success:    Models set, Version set.
//   - Any failure:       Error non-empty (ID echoes the request).
type Response struct {
	// ID echoes Request.ID so the host can correlate.
	ID int64 `json:"id,omitempty"`

	// Boundaries holds interior sentence-boundary RUNE offsets into the
	// request text, strictly ascending, excluding 0 and the text length.
	// Non-nil (may be empty) on a successful OpSegment response.
	Boundaries []int `json:"boundaries,omitempty"`

	// OK is true on a successful OpPing response.
	OK bool `json:"ok,omitempty"`

	// Version is the plugin version, set on ping/info responses.
	Version string `json:"version,omitempty"`

	// Models lists the models the plugin supports, set on info responses.
	Models []ModelInfo `json:"models,omitempty"`

	// Error, when non-empty, describes a failure for the request with the
	// matching ID. When set, the other result fields are not meaningful.
	Error string `json:"error,omitempty"`
}

// ModelInfo describes one model the plugin can serve.
type ModelInfo struct {
	// Name is the model identifier (e.g. "sat-3l-sm").
	Name string `json:"name"`
	// Loaded reports whether the model is currently resident in memory.
	Loaded bool `json:"loaded"`
	// Default reports whether this is the plugin's default model.
	Default bool `json:"default,omitempty"`
}

// MaxLineBytes bounds a single protocol line. SaT requests can carry large
// documents, so the limit is generous (64 MiB). Both the plugin's server loop
// and the client reader use it to size their bufio.Scanner buffers.
const MaxLineBytes = 64 << 20

// newScanner returns a bufio.Scanner configured for the protocol's line size.
func newScanner(r io.Reader) *bufio.Scanner {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), MaxLineBytes)
	return sc
}

// WriteMessage encodes v as a single JSON line (no embedded newlines, since
// encoding/json escapes them) followed by '\n', and writes it to w.
func WriteMessage(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("satproto: marshal: %w", err)
	}
	b = append(b, '\n')
	if _, err := w.Write(b); err != nil {
		return fmt.Errorf("satproto: write: %w", err)
	}
	return nil
}

// ----------------------------------------------------------------------------
// Server-side helpers (used by the plugin; pure Go, safe for the host too).
// ----------------------------------------------------------------------------

// Handler processes a single request and returns the response to send. The
// plugin supplies one. Returning an error is equivalent to returning a
// Response with Error set; Serve converts it for you.
type Handler func(Request) Response

// Serve runs the plugin read/dispatch/write loop: it reads one Request per
// line from r, calls h for each, and writes the resulting Response per line to
// w. It returns nil when r reaches EOF (clean host shutdown) and a non-nil
// error only on an I/O failure it cannot recover from.
//
// A malformed request line does not terminate the loop: Serve emits an error
// Response (with id 0, since the id could not be parsed) and continues.
func Serve(r io.Reader, w io.Writer, h Handler) error {
	sc := newScanner(r)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var req Request
		if err := json.Unmarshal(line, &req); err != nil {
			if werr := WriteMessage(w, Response{Error: fmt.Sprintf("malformed request: %v", err)}); werr != nil {
				return werr
			}
			continue
		}
		resp := h(req)
		resp.ID = req.ID
		if err := WriteMessage(w, resp); err != nil {
			return err
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("satproto: scan: %w", err)
	}
	return nil
}

// ----------------------------------------------------------------------------
// Client-side driver (used by the host; pure Go, no cgo).
// ----------------------------------------------------------------------------

// Client drives a spawned plugin over its stdin/stdout pipes. It is safe for
// concurrent use: each call sends one request and reads exactly one response,
// serialized by an internal mutex. (The reference plugin replies in order, so
// strict request/response pairing is correct.)
type Client struct {
	mu  sync.Mutex
	w   io.Writer
	sc  *bufio.Scanner
	id  int64
	err error
}

// NewClient wraps the plugin's stdin (w, host→plugin) and stdout (r,
// plugin→host) pipes. The caller owns the process lifecycle (start, wait,
// kill) and the pipes; closing w signals the plugin to exit its read loop.
func NewClient(w io.Writer, r io.Reader) *Client {
	return &Client{w: w, sc: newScanner(r)}
}

// roundTrip sends req (assigning a fresh id) and returns the matching
// response. Once an I/O error occurs the client is poisoned and every
// subsequent call fails fast.
func (c *Client) roundTrip(req Request) (Response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return Response{}, c.err
	}
	c.id++
	req.ID = c.id
	if err := WriteMessage(c.w, req); err != nil {
		c.err = err
		return Response{}, err
	}
	if !c.sc.Scan() {
		if err := c.sc.Err(); err != nil {
			c.err = fmt.Errorf("satproto: read response: %w", err)
		} else {
			c.err = fmt.Errorf("satproto: plugin closed stdout: %w", io.EOF)
		}
		return Response{}, c.err
	}
	var resp Response
	if err := json.Unmarshal(c.sc.Bytes(), &resp); err != nil {
		c.err = fmt.Errorf("satproto: decode response: %w", err)
		return Response{}, c.err
	}
	return resp, nil
}

// Segment requests sentence segmentation of text and returns the interior
// boundary rune offsets. model and lang may be empty to use plugin defaults;
// threshold of 0 uses the model default. A protocol-level error reported by
// the plugin is surfaced as a Go error.
func (c *Client) Segment(text, model, lang string, threshold float64) ([]int, error) {
	resp, err := c.roundTrip(Request{
		Op:        OpSegment,
		Text:      text,
		Model:     model,
		Lang:      lang,
		Threshold: threshold,
	})
	if err != nil {
		return nil, err
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("satproto: plugin error: %s", resp.Error)
	}
	if resp.Boundaries == nil {
		return []int{}, nil
	}
	return resp.Boundaries, nil
}

// Ping issues a liveness check and returns the plugin version.
func (c *Client) Ping() (version string, err error) {
	resp, err := c.roundTrip(Request{Op: OpPing})
	if err != nil {
		return "", err
	}
	if resp.Error != "" {
		return "", fmt.Errorf("satproto: plugin error: %s", resp.Error)
	}
	if !resp.OK {
		return "", errors.New("satproto: ping not acknowledged")
	}
	return resp.Version, nil
}

// Info enumerates the models the plugin supports.
func (c *Client) Info() ([]ModelInfo, string, error) {
	resp, err := c.roundTrip(Request{Op: OpInfo})
	if err != nil {
		return nil, "", err
	}
	if resp.Error != "" {
		return nil, "", fmt.Errorf("satproto: plugin error: %s", resp.Error)
	}
	return resp.Models, resp.Version, nil
}
