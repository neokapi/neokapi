package jobs

import (
	"context"
	"errors"
	"net"
	"strings"
)

// defaultMaxJobAttempts bounds how many times a translation job is retried on
// transient failures before it is marked failed. It is aligned with the NATS
// JetStream redelivery cap (natsMaxDeliver) so the in-flight NAK retry and the
// DB attempt budget agree on the same ceiling.
const defaultMaxJobAttempts = natsMaxDeliver

// transientError marks a job failure the worker should NAK so the message
// broker redelivers it. The job row has been reset to 'queued' (see
// JobStore.RetryOrFail) so the redelivery can re-claim it. ACKing a transient
// failure would strand a job that would have succeeded on retry.
type transientError struct{ err error }

func (e *transientError) Error() string { return e.err.Error() }
func (e *transientError) Unwrap() error { return e.err }

// isTransientError reports whether err is a retryable upstream failure — a
// provider 5xx / 429 / 529, a request timeout, or a transport-level network
// error — as opposed to a permanent error (4xx auth/validation, missing
// project, malformed config) that would fail identically on redelivery. The AI
// providers surface HTTP failures as formatted strings ("openai: API error
// 503: ...") rather than typed errors, so classification is necessarily
// string-based, with typed net/context checks layered on top.
func isTransientError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}

	msg := strings.ToLower(err.Error())

	// HTTP status codes surfaced by the AI providers. 5xx and 429/529 are
	// retryable; 4xx (400/401/403/404/422) are permanent and deliberately absent.
	for _, code := range []string{
		"error 500", "error 502", "error 503", "error 504", "error 529", "error 429",
	} {
		if strings.Contains(msg, code) {
			return true
		}
	}

	for _, marker := range []string{
		"rate limit",
		"too many requests",
		"timeout",
		"timed out",
		"deadline exceeded",
		"connection refused",
		"connection reset",
		"broken pipe",
		"unexpected eof",
		"no such host",
		"temporarily unavailable",
		"service unavailable",
		"server error",
		"overloaded",
	} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}
