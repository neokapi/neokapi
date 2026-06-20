// Package llmproto defines the line-delimited JSON protocol spoken between a
// host process and the kapi-llm local-LLM plugin over the plugin's stdin and
// stdout.
//
// The protocol is deliberately small and dependency-free: it contains only
// plain Go structs plus encode/decode helpers and a client driver. It imports
// nothing beyond the standard library — in particular it has no cgo, no ONNX
// runtime, and no tokenizer dependency. This lets the host-side Gemma provider
// import this package to talk to the plugin without inheriting the plugin's
// heavy native build requirements.
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
// echoes that id in the matching response, so the host correlates by id.
//
// # Multimodal inputs
//
// A [Message] carries text plus zero or more [Media] references. Media is
// passed BY PATH (a local filesystem path the plugin can open), never inline
// bytes — mirroring the kapi-asr and kapi-vision plugins. The host materializes
// image/audio/video to a temp file and sends the path; the plugin decodes and
// runs the appropriate encoder, splicing the resulting embeddings into the
// prompt before decoding.
//
// # Operations
//
//	{"id":1,"op":"generate","messages":[{"role":"user","text":"Bonjour"}],"max_tokens":256}
//	  -> {"id":1,"text":"Hello","input_tokens":12,"output_tokens":3}
//
//	{"op":"ping"}
//	  -> {"ok":true,"version":"0.1.0"}
//
//	{"op":"info"}
//	  -> {"models":[{"name":"gemma-4-e2b","loaded":true}],"modalities":["image","audio"],"version":"0.1.0"}
//
// Errors are reported in-band: a response carrying a non-empty error field
// describes a failure for the request with the matching id. The plugin stays
// alive after an error so the host can continue issuing requests.
package llmproto

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
	// OpGenerate requests an autoregressive completion of Request.Messages.
	OpGenerate Op = "generate"
	// OpPing is a liveness/version check. It carries no payload.
	OpPing Op = "ping"
	// OpInfo asks the plugin to enumerate the models it supports, which are
	// loaded, and the non-text input modalities it accepts.
	OpInfo Op = "info"
)

// MediaKind discriminates a non-text input attached to a message.
type MediaKind string

const (
	MediaImage MediaKind = "image"
	MediaAudio MediaKind = "audio"
	MediaVideo MediaKind = "video"
)

// Media references one non-text input by local filesystem path. The host writes
// the bytes to a temp file (or points at an existing local file) and sends the
// path; the plugin opens and decodes it. Passing a path rather than inline
// base64 keeps large media off the JSON line and out of memory until the plugin
// needs it.
type Media struct {
	// Kind is the modality: image, audio, or video.
	Kind MediaKind `json:"kind"`
	// Path is the local filesystem path the plugin opens.
	Path string `json:"path"`
	// MIME is the optional media type hint (e.g. "image/png").
	MIME string `json:"mime,omitempty"`
}

// Message is one turn in the conversation. Role is "system", "user", or
// "assistant". Text holds the turn's text; Media attaches any image/audio/video
// inputs for that turn (used only on user turns).
type Message struct {
	Role  string  `json:"role"`
	Text  string  `json:"text,omitempty"`
	Media []Media `json:"media,omitempty"`
}

// Request is one line-delimited JSON request from host to plugin.
type Request struct {
	// ID correlates this request with its [Response]. Ping/info may omit it.
	ID int64 `json:"id,omitempty"`

	// Op selects the operation. Required.
	Op Op `json:"op"`

	// Messages is the conversation to complete. Used only by OpGenerate.
	Messages []Message `json:"messages,omitempty"`

	// Model selects which model to use (e.g. "gemma-4-e2b"). Empty means the
	// plugin's default model.
	Model string `json:"model,omitempty"`

	// MaxTokens bounds the number of tokens generated. Zero means the model's
	// default budget.
	MaxTokens int `json:"max_tokens,omitempty"`

	// Temperature is the sampling temperature. Zero (or negative) requests
	// greedy decoding.
	Temperature float64 `json:"temperature,omitempty"`

	// TopP is the nucleus-sampling cutoff in (0,1]. Zero means "no nucleus
	// filtering" (consider the full distribution, subject to Temperature).
	TopP float64 `json:"top_p,omitempty"`

	// Schema, when non-empty, is a JSON Schema the output should conform to.
	// The plugin steers generation toward valid JSON (best-effort); it is not a
	// hard grammar constraint.
	Schema json.RawMessage `json:"schema,omitempty"`
}

// Response is one line-delimited JSON response from plugin to host.
//
//   - OpGenerate success: Text set (possibly empty), token counts populated.
//   - OpPing success:     OK true, Version set.
//   - OpInfo success:     Models + Modalities set, Version set.
//   - Any failure:        Error non-empty (ID echoes the request).
type Response struct {
	// ID echoes Request.ID so the host can correlate.
	ID int64 `json:"id,omitempty"`

	// Text is the generated completion on a successful OpGenerate response.
	Text string `json:"text,omitempty"`

	// InputTokens / OutputTokens report token usage for an OpGenerate response.
	InputTokens  int `json:"input_tokens,omitempty"`
	OutputTokens int `json:"output_tokens,omitempty"`

	// OK is true on a successful OpPing response.
	OK bool `json:"ok,omitempty"`

	// Version is the plugin version, set on ping/info responses.
	Version string `json:"version,omitempty"`

	// Models lists the models the plugin supports, set on info responses.
	Models []ModelInfo `json:"models,omitempty"`

	// Modalities lists the non-text input modalities the plugin accepts
	// ("image", "audio", "video"), set on info responses.
	Modalities []string `json:"modalities,omitempty"`

	// Error, when non-empty, describes a failure for the request with the
	// matching ID. When set, the other result fields are not meaningful.
	Error string `json:"error,omitempty"`
}

// ModelInfo describes one model the plugin can serve.
type ModelInfo struct {
	// Name is the model identifier (e.g. "gemma-4-e2b").
	Name string `json:"name"`
	// Loaded reports whether the model is currently resident in memory.
	Loaded bool `json:"loaded"`
	// Default reports whether this is the plugin's default model.
	Default bool `json:"default,omitempty"`
}

// MaxLineBytes bounds a single protocol line. Generate requests can carry large
// prompts, so the limit is generous (64 MiB). Both the plugin's server loop and
// the client reader use it to size their buffers.
const MaxLineBytes = 64 << 20

// newScanner returns a bufio.Scanner configured for the protocol's line size.
func newScanner(r io.Reader) *bufio.Scanner {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), MaxLineBytes)
	return sc
}

// errLineTooLong reports that a single protocol line exceeded MaxLineBytes. The
// offending line has been drained so the reader is positioned at the start of
// the next line.
var errLineTooLong = errors.New("llmproto: request line exceeds maximum size")

// readLine reads one '\n'-terminated line from br, enforcing the MaxLineBytes
// cap. It returns the line bytes (without the trailing newline). An oversized
// line is drained (up to the next '\n') and reported as errLineTooLong, leaving
// br positioned at the following line — so an oversized line is rejected rather
// than aborting the loop.
func readLine(br *bufio.Reader) ([]byte, error) {
	var buf []byte
	tooLong := false
	for {
		b, err := br.ReadByte()
		if err != nil {
			if len(buf) == 0 || tooLong {
				if tooLong {
					return nil, errLineTooLong
				}
				return nil, err
			}
			return buf, nil
		}
		if b == '\n' {
			if tooLong {
				return nil, errLineTooLong
			}
			return buf, nil
		}
		if tooLong {
			continue
		}
		if len(buf) >= MaxLineBytes {
			tooLong = true
			continue
		}
		buf = append(buf, b)
	}
}

// WriteMessage encodes v as a single JSON line (no embedded newlines, since
// encoding/json escapes them) followed by '\n', and writes it to w.
func WriteMessage(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("llmproto: marshal: %w", err)
	}
	b = append(b, '\n')
	if _, err := w.Write(b); err != nil {
		return fmt.Errorf("llmproto: write: %w", err)
	}
	return nil
}

// ----------------------------------------------------------------------------
// Server-side helpers (used by the plugin; pure Go, safe for the host too).
// ----------------------------------------------------------------------------

// Handler processes a single request and returns the response to send.
type Handler func(Request) Response

// Serve runs the plugin read/dispatch/write loop: it reads one Request per line
// from r, calls h for each, and writes the resulting Response per line to w. It
// returns nil when r reaches EOF (clean host shutdown) and a non-nil error only
// on an I/O failure it cannot recover from.
//
// A malformed or oversized request line does not terminate the loop: Serve
// emits an error Response and continues, so a single bad line cannot take the
// long-lived process down.
func Serve(r io.Reader, w io.Writer, h Handler) error {
	br := bufio.NewReaderSize(r, 64*1024)
	for {
		line, err := readLine(br)
		if errors.Is(err, errLineTooLong) {
			if werr := WriteMessage(w, Response{Error: "malformed request: line exceeds maximum size"}); werr != nil {
				return werr
			}
			continue
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("llmproto: read: %w", err)
		}
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
}

// ----------------------------------------------------------------------------
// Client-side driver (used by the host; pure Go, no cgo).
// ----------------------------------------------------------------------------

// Client drives a spawned plugin over its stdin/stdout pipes. It is safe for
// concurrent use: each call sends one request and reads exactly one response,
// serialized by an internal mutex.
type Client struct {
	mu  sync.Mutex
	w   io.Writer
	sc  *bufio.Scanner
	id  int64
	err error
}

// NewClient wraps the plugin's stdin (w, host→plugin) and stdout (r,
// plugin→host) pipes. The caller owns the process lifecycle and the pipes;
// closing w signals the plugin to exit its read loop.
func NewClient(w io.Writer, r io.Reader) *Client {
	return &Client{w: w, sc: newScanner(r)}
}

// roundTrip sends req (assigning a fresh id) and returns the matching response.
// Once an I/O error occurs the client is poisoned and every subsequent call
// fails fast.
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
			c.err = fmt.Errorf("llmproto: read response: %w", err)
		} else {
			c.err = fmt.Errorf("llmproto: plugin closed stdout: %w", io.EOF)
		}
		return Response{}, c.err
	}
	var resp Response
	if err := json.Unmarshal(c.sc.Bytes(), &resp); err != nil {
		c.err = fmt.Errorf("llmproto: decode response: %w", err)
		return Response{}, c.err
	}
	if resp.ID != req.ID {
		c.err = fmt.Errorf("llmproto: response id %d does not match request id %d", resp.ID, req.ID)
		return Response{}, c.err
	}
	return resp, nil
}

// Generate requests a completion of messages and returns the response. model
// may be empty to use the plugin default. A protocol-level error reported by the
// plugin is surfaced as a Go error.
func (c *Client) Generate(req Request) (Response, error) {
	req.Op = OpGenerate
	resp, err := c.roundTrip(req)
	if err != nil {
		return Response{}, err
	}
	if resp.Error != "" {
		return Response{}, fmt.Errorf("llmproto: plugin error: %s", resp.Error)
	}
	return resp, nil
}

// Ping issues a liveness check and returns the plugin version.
func (c *Client) Ping() (version string, err error) {
	resp, err := c.roundTrip(Request{Op: OpPing})
	if err != nil {
		return "", err
	}
	if resp.Error != "" {
		return "", fmt.Errorf("llmproto: plugin error: %s", resp.Error)
	}
	if !resp.OK {
		return "", errors.New("llmproto: ping not acknowledged")
	}
	return resp.Version, nil
}

// Info enumerates the models the plugin supports and the modalities it accepts.
func (c *Client) Info() (models []ModelInfo, modalities []string, version string, err error) {
	resp, err := c.roundTrip(Request{Op: OpInfo})
	if err != nil {
		return nil, nil, "", err
	}
	if resp.Error != "" {
		return nil, nil, "", fmt.Errorf("llmproto: plugin error: %s", resp.Error)
	}
	return resp.Models, resp.Modalities, resp.Version, nil
}
