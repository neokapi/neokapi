package client

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// StatusError is a non-2xx HTTP response from the server. Callers that need to
// distinguish a permanent client error (4xx — the request itself was rejected
// and retrying the same request can never succeed) from a transient failure
// unwrap it with errors.As; the desktop offline-queue replay uses it to retire
// permanently failing queued operations instead of retrying them forever.
//
// It decodes the server's standard error envelope
// `{"error": <code>, "message": <sentence>, "reference": <request id>, …}`
// (bowrain/apierror) when the body carries one, so command output shows the
// human sentence and the correlation reference — `pull failed (HTTP 403):
// <sentence> (ref: <id>)` — instead of raw JSON; envelope-less bodies keep the
// historical `<op> failed (HTTP <status>): <body>` output.
type StatusError struct {
	StatusCode int
	// Path is the request path or operation label (e.g. "pull", "push init").
	Path string
	// Body is the raw response body.
	Body string

	// Envelope fields; empty when the body was not the standard envelope.
	Code      string // stable machine code — the envelope's "error" field
	Message   string // human sentence — the envelope's "message" field
	Reference string // per-request correlation ID the user can quote to support
}

// NewStatusError builds a StatusError for an operation from a non-2xx
// response body, decoding the standard error envelope when present.
func NewStatusError(op string, statusCode int, body []byte) *StatusError {
	e := &StatusError{StatusCode: statusCode, Path: op, Body: string(body)}
	var env struct {
		Error       string `json:"error"`
		Message     string `json:"message"`
		Reference   string `json:"reference"`
		ReferenceID string `json:"reference_id"`
	}
	if err := json.Unmarshal(body, &env); err == nil {
		e.Code = env.Error
		e.Message = env.Message
		e.Reference = env.Reference
		if e.Reference == "" {
			e.Reference = env.ReferenceID
		}
	}
	return e
}

func (e *StatusError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = e.Code
	}
	if msg == "" {
		// No envelope in the body — the historical output, raw body included.
		return fmt.Sprintf("%s failed (HTTP %d): %s", e.Path, e.StatusCode, e.Body)
	}
	if e.Reference != "" {
		return fmt.Sprintf("%s failed (HTTP %d): %s (ref: %s)", e.Path, e.StatusCode, msg, e.Reference)
	}
	return fmt.Sprintf("%s failed (HTTP %d): %s", e.Path, e.StatusCode, msg)
}

// Permanent reports whether retrying the identical request can never succeed:
// any 4xx except 408 (request timeout) and 429 (rate limited), which are
// transient by definition.
func (e *StatusError) Permanent() bool {
	if e.StatusCode == http.StatusRequestTimeout || e.StatusCode == http.StatusTooManyRequests {
		return false
	}
	return e.StatusCode >= 400 && e.StatusCode < 500
}
