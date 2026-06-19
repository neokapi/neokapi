// Package asrproto defines the line-delimited JSON protocol spoken between a
// host process and the kapi-asr speech-recognition plugin over the plugin's
// stdin and stdout. It is the audio counterpart of satproto.
//
// The protocol is deliberately tiny and dependency-free: plain Go structs plus
// encode/decode helpers and a small client driver. It imports nothing beyond the
// standard library — no cgo, no ONNX runtime, no audio decoder. This lets the
// host-side asr engine import it to drive the plugin without inheriting the
// plugin's heavy native build requirements.
//
// # Wire format
//
// Each message is a single JSON object on its own line ('\n'-terminated), in
// both directions. The host writes a [Request] per line on the plugin's stdin
// and reads a [Response] per line from the plugin's stdout. The plugin keeps
// running across many requests until stdin closes. Requests carry a host-chosen
// id; the plugin echoes it so the host can correlate.
//
// Audio is passed by PATH, never inline: the request names a file the plugin
// opens and decodes itself, so the audio bytes never travel over the pipe.
//
// # Operations
//
//	{"id":1,"op":"transcribe","audioPath":"/tmp/track.wav","lang":"en"}
//	  -> {"id":1,"segments":[{"text":"hello there","startMs":0,"endMs":1200,"confidence":0.94}],"language":"en"}
//
//	{"op":"ping"}   -> {"ok":true,"version":"0.1.0"}
//	{"op":"info"}   -> {"models":[{"name":"whisper-base","loaded":true}],"version":"0.1.0"}
//
// Errors are reported in-band: a response with a non-empty error field describes
// a failure for the request with the matching id. The plugin stays alive after
// an error so the host can keep issuing requests.
package asrproto

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
)

// Op identifies a request operation.
type Op string

const (
	// OpTranscribe requests transcription of the audio file at Request.AudioPath.
	OpTranscribe Op = "transcribe"
	// OpPing is a liveness/version check. It carries no payload.
	OpPing Op = "ping"
	// OpInfo asks the plugin to enumerate the models it supports.
	OpInfo Op = "info"
)

// Segment is one recognized span on the wire: text plus millisecond time bounds
// and the model's confidence in [0,1]. Mirrors core/asr.Segment.
type Segment struct {
	Text       string  `json:"text"`
	StartMS    int64   `json:"startMs"`
	EndMS      int64   `json:"endMs,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
}

// Request is one line-delimited JSON request from host to plugin.
type Request struct {
	// ID correlates this request with its [Response]. Ping/info may omit it.
	ID int64 `json:"id,omitempty"`
	// Op selects the operation. Required.
	Op Op `json:"op"`
	// AudioPath is the local filesystem path of the audio to transcribe. Used
	// only by OpTranscribe. The plugin opens and decodes it.
	AudioPath string `json:"audioPath,omitempty"`
	// Lang is an optional BCP-47 language hint; empty lets the model auto-detect.
	Lang string `json:"lang,omitempty"`
	// Model selects the model (e.g. "whisper-base"); empty uses the default.
	Model string `json:"model,omitempty"`
}

// Response is one line-delimited JSON response from plugin to host.
//
//   - OpTranscribe success: Segments set (possibly empty), Error empty.
//   - OpPing success:       OK true, Version set.
//   - OpInfo success:       Models set, Version set.
//   - Any failure:          Error non-empty (ID echoes the request).
type Response struct {
	ID       int64       `json:"id,omitempty"`
	Segments []Segment   `json:"segments,omitempty"`
	Language string      `json:"language,omitempty"`
	OK       bool        `json:"ok,omitempty"`
	Version  string      `json:"version,omitempty"`
	Models   []ModelInfo `json:"models,omitempty"`
	Error    string      `json:"error,omitempty"`
}

// ModelInfo describes one model the plugin can serve.
type ModelInfo struct {
	Name    string `json:"name"`
	Loaded  bool   `json:"loaded"`
	Default bool   `json:"default,omitempty"`
}

// MaxLineBytes bounds a single protocol line (64 MiB) — a long transcript's
// segment list can be large, so the limit is generous.
const MaxLineBytes = 64 << 20

func newScanner(r io.Reader) *bufio.Scanner {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), MaxLineBytes)
	return sc
}

var errLineTooLong = errors.New("asrproto: line exceeds maximum size")

// readLine reads one '\n'-terminated line, enforcing MaxLineBytes; an oversized
// line is drained and rejected (errLineTooLong) rather than aborting the loop.
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

// WriteMessage encodes v as a single JSON line followed by '\n'.
func WriteMessage(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("asrproto: marshal: %w", err)
	}
	b = append(b, '\n')
	if _, err := w.Write(b); err != nil {
		return fmt.Errorf("asrproto: write: %w", err)
	}
	return nil
}

// Handler processes a single request and returns the response to send.
type Handler func(Request) Response

// Serve runs the plugin read/dispatch/write loop: one Request per line in, one
// Response per line out. Returns nil on EOF (clean shutdown). A malformed or
// oversized line yields an error Response and the loop continues.
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
			return fmt.Errorf("asrproto: read: %w", err)
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

// Client drives a spawned plugin over its stdin/stdout pipes. Safe for
// concurrent use: each call sends one request and reads one response, serialized
// by an internal mutex.
type Client struct {
	mu  sync.Mutex
	w   io.Writer
	sc  *bufio.Scanner
	id  int64
	err error
}

// NewClient wraps the plugin's stdin (w, host→plugin) and stdout (r,
// plugin→host). The caller owns the process lifecycle; closing w signals exit.
func NewClient(w io.Writer, r io.Reader) *Client {
	return &Client{w: w, sc: newScanner(r)}
}

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
			c.err = fmt.Errorf("asrproto: read response: %w", err)
		} else {
			c.err = fmt.Errorf("asrproto: plugin closed stdout: %w", io.EOF)
		}
		return Response{}, c.err
	}
	var resp Response
	if err := json.Unmarshal(c.sc.Bytes(), &resp); err != nil {
		c.err = fmt.Errorf("asrproto: decode response: %w", err)
		return Response{}, c.err
	}
	if resp.ID != req.ID {
		c.err = fmt.Errorf("asrproto: response id %d does not match request id %d", resp.ID, req.ID)
		return Response{}, c.err
	}
	return resp, nil
}

// Transcribe asks the plugin to transcribe the audio file at audioPath. model
// and lang may be empty for plugin defaults. A protocol-level plugin error is
// surfaced as a Go error. The context is honoured by aborting the wait if it is
// cancelled before the response arrives.
func (c *Client) Transcribe(ctx context.Context, audioPath, model, lang string) (*Response, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	resp, err := c.roundTrip(Request{Op: OpTranscribe, AudioPath: audioPath, Model: model, Lang: lang})
	if err != nil {
		return nil, err
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("asrproto: plugin error: %s", resp.Error)
	}
	return &resp, nil
}

// Ping issues a liveness check and returns the plugin version.
func (c *Client) Ping() (version string, err error) {
	resp, err := c.roundTrip(Request{Op: OpPing})
	if err != nil {
		return "", err
	}
	if resp.Error != "" {
		return "", fmt.Errorf("asrproto: plugin error: %s", resp.Error)
	}
	if !resp.OK {
		return "", errors.New("asrproto: ping not acknowledged")
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
		return nil, "", fmt.Errorf("asrproto: plugin error: %s", resp.Error)
	}
	return resp.Models, resp.Version, nil
}
