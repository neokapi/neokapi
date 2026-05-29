// Package checkproto defines the line-delimited JSON protocol spoken between a
// host process and the kapi-check ML checker plugin over the plugin's stdin and
// stdout.
//
// Like the segmenter's satproto, it is deliberately tiny and dependency-free:
// plain Go structs plus encode/decode helpers and a small client driver, with
// nothing beyond the standard library — no cgo, no ONNX runtime, no tokenizer.
// The host imports it to drive the plugin without inheriting the plugin's heavy
// native build.
//
// # Operations
//
//	{"id":1,"op":"similarity","text":"We are delighted to help.","refs":["Hi there!","Welcome."]}
//	  -> {"id":1,"scores":[0.71,0.83]}        // cosine of text to each ref
//
//	{"id":2,"op":"embed","text":"Willkommen"}
//	  -> {"id":2,"embedding":[0.01,-0.02, ...]}
//
//	{"op":"ping"}  -> {"ok":true,"version":"0.1.0"}
//	{"op":"info"}  -> {"models":[{"name":"e5-small-int8","loaded":true,"default":true}],"version":"0.1.0"}
//
// Errors are reported in-band: a response with a non-empty error field describes
// a failure for the request with the matching id; the plugin stays alive.
package checkproto

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
	// OpSimilarity returns the cosine similarity of Text to each of Refs.
	OpSimilarity Op = "similarity"
	// OpEmbed returns the embedding vector for Text.
	OpEmbed Op = "embed"
	// OpPing is a liveness/version check.
	OpPing Op = "ping"
	// OpInfo enumerates the models the plugin supports.
	OpInfo Op = "info"
)

// Request is one line-delimited JSON request from host to plugin.
type Request struct {
	ID    int64    `json:"id,omitempty"`
	Op    Op       `json:"op"`
	Text  string   `json:"text,omitempty"`
	Refs  []string `json:"refs,omitempty"`
	Model string   `json:"model,omitempty"`
}

// Response is one line-delimited JSON response from plugin to host.
type Response struct {
	ID        int64       `json:"id,omitempty"`
	Scores    []float64   `json:"scores,omitempty"`
	Embedding []float32   `json:"embedding,omitempty"`
	OK        bool        `json:"ok,omitempty"`
	Version   string      `json:"version,omitempty"`
	Models    []ModelInfo `json:"models,omitempty"`
	Error     string      `json:"error,omitempty"`
}

// ModelInfo describes one model the plugin can serve.
type ModelInfo struct {
	Name    string `json:"name"`
	Loaded  bool   `json:"loaded"`
	Default bool   `json:"default,omitempty"`
}

// MaxLineBytes bounds a single protocol line (16 MiB — checks operate on short
// strings, but embeddings and ref lists can add up).
const MaxLineBytes = 16 << 20

func newScanner(r io.Reader) *bufio.Scanner {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), MaxLineBytes)
	return sc
}

// WriteMessage encodes v as a single JSON line followed by '\n'.
func WriteMessage(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("checkproto: marshal: %w", err)
	}
	b = append(b, '\n')
	if _, err := w.Write(b); err != nil {
		return fmt.Errorf("checkproto: write: %w", err)
	}
	return nil
}

// Handler processes a single request and returns the response to send.
type Handler func(Request) Response

// Serve runs the plugin read/dispatch/write loop. It returns nil at EOF (clean
// shutdown) and a non-nil error only on an unrecoverable I/O failure. A
// malformed line yields an error response and the loop continues.
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
		return fmt.Errorf("checkproto: scan: %w", err)
	}
	return nil
}

// Client drives a spawned plugin over its stdin/stdout pipes. Safe for
// concurrent use: each call sends one request and reads one response.
type Client struct {
	mu  sync.Mutex
	w   io.Writer
	sc  *bufio.Scanner
	id  int64
	err error
}

// NewClient wraps the plugin's stdin (w) and stdout (r). The caller owns the
// process lifecycle; closing w signals the plugin to exit.
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
			c.err = fmt.Errorf("checkproto: read response: %w", err)
		} else {
			c.err = fmt.Errorf("checkproto: plugin closed stdout: %w", io.EOF)
		}
		return Response{}, c.err
	}
	var resp Response
	if err := json.Unmarshal(c.sc.Bytes(), &resp); err != nil {
		c.err = fmt.Errorf("checkproto: decode response: %w", err)
		return Response{}, c.err
	}
	return resp, nil
}

// Similarity returns the cosine similarity of text to each ref, in order.
func (c *Client) Similarity(text string, refs []string, model string) ([]float64, error) {
	resp, err := c.roundTrip(Request{Op: OpSimilarity, Text: text, Refs: refs, Model: model})
	if err != nil {
		return nil, err
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("checkproto: plugin error: %s", resp.Error)
	}
	if resp.Scores == nil {
		return []float64{}, nil
	}
	return resp.Scores, nil
}

// Embed returns the embedding vector for text.
func (c *Client) Embed(text, model string) ([]float32, error) {
	resp, err := c.roundTrip(Request{Op: OpEmbed, Text: text, Model: model})
	if err != nil {
		return nil, err
	}
	if resp.Error != "" {
		return nil, fmt.Errorf("checkproto: plugin error: %s", resp.Error)
	}
	return resp.Embedding, nil
}

// Ping issues a liveness check and returns the plugin version.
func (c *Client) Ping() (string, error) {
	resp, err := c.roundTrip(Request{Op: OpPing})
	if err != nil {
		return "", err
	}
	if resp.Error != "" {
		return "", fmt.Errorf("checkproto: plugin error: %s", resp.Error)
	}
	if !resp.OK {
		return "", errors.New("checkproto: ping not acknowledged")
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
		return nil, "", fmt.Errorf("checkproto: plugin error: %s", resp.Error)
	}
	return resp.Models, resp.Version, nil
}
