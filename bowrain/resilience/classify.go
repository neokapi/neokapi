package resilience

import (
	"context"
	"errors"
	"net"
	"strings"
)

// IsUpstreamFailure reports whether err is a retryable *upstream* failure — a
// provider 5xx / 429 / 529, a request timeout, or a transport-level network
// error — as opposed to a permanent fault (4xx auth/validation, missing
// resource, malformed config) that would fail identically on the next attempt.
//
// This is the single classifier for the platform. It decides three things at
// once, and they must agree or the system contradicts itself:
//
//  1. whether a breaker counts the call as a failure ([Breaker.Do]),
//  2. whether the worker retries a job or fails it (bowrain/jobs),
//  3. whether an open circuit is the honest explanation to show a user.
//
// Classification is necessarily part string-based: the AI providers surface
// HTTP failures as formatted strings ("openai: API error 503: …") rather than
// typed errors, so typed net/context checks are layered on top of a marker
// scan. When a call site does know better it should say so explicitly with
// [CallerFault] rather than hope the markers below cover it.
//
// An open-circuit rejection is deliberately *not* an upstream failure: nothing
// was attempted, so it must not be re-counted against the breaker that produced
// it, nor consume a job's retry budget.
func IsUpstreamFailure(err error) bool {
	if err == nil {
		return false
	}
	if IsUnavailable(err) || isCallerFault(err) {
		return false
	}
	// A caller that walked away is not evidence about the dependency.
	if errors.Is(err, context.Canceled) {
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
	for _, code := range transientStatusMarkers {
		if strings.Contains(msg, code) {
			return true
		}
	}
	for _, marker := range transientTextMarkers {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

var transientStatusMarkers = []string{
	"error 500", "error 502", "error 503", "error 504", "error 529", "error 429",
}

var transientTextMarkers = []string{
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
}
