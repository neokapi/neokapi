package aiprovider

import (
	"fmt"
	"net/http"
	"strings"
)

// ---------------------------------------------------------------------------
// Ollama error mapping
// ---------------------------------------------------------------------------
//
// The two Ollama failures a user actually hits — the server isn't running, and
// the model isn't pulled — are both recoverable by one concrete command. Making
// them typed (rather than only well-worded) lets a GUI turn each into a button
// instead of re-deriving the situation by matching on prose, which breaks the
// moment the wording is edited or translated.
//
// Modelled on ClaudeCodeError: the kind carries the classification, Error()
// carries the same actionable sentence the CLI has always printed.

// OllamaErrorKind classifies Ollama failures that have a concrete remedy.
type OllamaErrorKind int

const (
	// OllamaErrUnreachable — nothing is listening at the configured base URL.
	// Remedy: start the server (`ollama serve`) or install Ollama.
	OllamaErrUnreachable OllamaErrorKind = iota
	// OllamaErrModelNotInstalled — the server is up but the model has not been
	// pulled. Remedy: `ollama pull <model>`.
	OllamaErrModelNotInstalled
	// OllamaErrOther — an API-level failure that is neither of the above.
	OllamaErrOther
)

// OllamaError is a typed failure from the Ollama provider or manager.
type OllamaError struct {
	Kind OllamaErrorKind
	// Model is the model tag involved, for ModelNotInstalled.
	Model string
	// BaseURL is the server endpoint, for Unreachable.
	BaseURL string
	// Status is the upstream HTTP status for Other (0 when unknown).
	Status int
	// Detail is the server's own message, when it sent one.
	Detail string
	// Err is the underlying transport error, for Unreachable.
	Err error
}

func (e *OllamaError) Error() string {
	switch e.Kind {
	case OllamaErrUnreachable:
		return fmt.Sprintf(
			"ollama: cannot reach Ollama at %s. Is it running? Start it with `ollama serve`, or install it from https://ollama.com (underlying error: %v)",
			e.BaseURL, e.Err)
	case OllamaErrModelNotInstalled:
		if e.Detail != "" {
			return fmt.Sprintf(
				"ollama: model %q is not installed. Pull it first with `ollama pull %s` (server said: %s)",
				e.Model, e.Model, e.Detail)
		}
		return fmt.Sprintf("ollama: model %q is not installed. Pull it first with `ollama pull %s`",
			e.Model, e.Model)
	default:
		if e.Status != 0 {
			return fmt.Sprintf("ollama: API error %d: %s", e.Status, e.Detail)
		}
		return "ollama: " + e.Detail
	}
}

// Unwrap exposes the transport cause so callers can still reach
// context.DeadlineExceeded / net errors through errors.Is.
func (e *OllamaError) Unwrap() error { return e.Err }

// Remediation is the single command that fixes this failure, or "" when there
// is none. A UI can offer it as a copyable command or a one-click action.
func (e *OllamaError) Remediation() string {
	switch e.Kind {
	case OllamaErrUnreachable:
		return "ollama serve"
	case OllamaErrModelNotInstalled:
		if e.Model == "" {
			return ""
		}
		return "ollama pull " + e.Model
	default:
		return ""
	}
}

// ollamaUnreachableError turns a connection-level failure into guidance: the
// usual cause is that no Ollama server is running at baseURL. Shared by the
// provider and the manager so the message is identical everywhere.
func ollamaUnreachableError(baseURL string, err error) error {
	return &OllamaError{Kind: OllamaErrUnreachable, BaseURL: baseURL, Err: err}
}

// ollamaModelNotInstalledError reports a model the server does not have. detail
// is the server's own message, when there was one.
func ollamaModelNotInstalledError(model, detail string) error {
	return &OllamaError{Kind: OllamaErrModelNotInstalled, Model: model, Detail: detail}
}

// classifyOllamaStatus maps a non-200 response onto a typed error. A 404 — or a
// body that says "not found" under any status — almost always means the
// requested model has not been pulled yet.
func classifyOllamaStatus(model string, status int, body []byte) error {
	msg := strings.TrimSpace(string(body))
	if status == http.StatusNotFound || strings.Contains(strings.ToLower(msg), "not found") {
		return ollamaModelNotInstalledError(model, msg)
	}
	return &OllamaError{Kind: OllamaErrOther, Model: model, Status: status, Detail: msg}
}
