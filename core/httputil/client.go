package httputil

import (
	"crypto/tls"
	"math"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"
)

// DefaultTimeout is the default HTTP client timeout for external API calls.
const DefaultTimeout = 30 * time.Second

// MaxRetries is the default number of retry attempts for transient errors.
const MaxRetries = 3

// MaxRetryAfter caps how long a server-controlled Retry-After header may delay
// a retry. A server (or a misbehaving proxy) could otherwise specify an
// arbitrarily large value and stall the client indefinitely.
const MaxRetryAfter = 60 * time.Second

// defaultTransport returns a base http.RoundTripper with a minimum TLS version.
func defaultTransport() http.RoundTripper {
	return &http.Transport{
		TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
	}
}

// NewClient returns an *http.Client with a sensible timeout for external APIs.
func NewClient() *http.Client {
	return &http.Client{
		Timeout:   DefaultTimeout,
		Transport: defaultTransport(),
	}
}

// NewResilientClient returns an *http.Client with retry and rate-limit awareness.
// It wraps the transport with automatic retry on 5xx and 429 responses.
func NewResilientClient() *http.Client {
	return &http.Client{
		Timeout:   DefaultTimeout,
		Transport: &retryTransport{base: defaultTransport(), maxRetries: MaxRetries},
	}
}

// retryTransport wraps an http.RoundTripper with retry logic for transient errors.
type retryTransport struct {
	base       http.RoundTripper
	maxRetries int
}

func (t *retryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	var resp *http.Response
	var err error

	for attempt := 0; attempt <= t.maxRetries; attempt++ {
		resp, err = t.base.RoundTrip(req)
		if err != nil {
			// Network-level error — only retry if the request body can be replayed.
			if req.GetBody == nil && req.Body != nil {
				return nil, err
			}
			if attempt < t.maxRetries {
				if werr := t.wait(req, backoff(attempt)); werr != nil {
					return nil, werr
				}
				if req.GetBody != nil {
					req.Body, _ = req.GetBody()
				}
				continue
			}
			return nil, err
		}

		// Retry on 429 (rate limited) or 5xx (server error).
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			// Only retry if the request body can be replayed; otherwise the
			// retry would send an exhausted (empty) body.
			if req.GetBody == nil && req.Body != nil {
				return resp, nil
			}
			if attempt < t.maxRetries {
				wait := backoff(attempt)
				// Respect Retry-After header if present, capped to a sane maximum
				// so a server-controlled value cannot stall the client.
				if ra, ok := retryAfterDelay(resp.Header.Get("Retry-After")); ok {
					wait = ra
				}
				resp.Body.Close()
				if werr := t.wait(req, wait); werr != nil {
					return nil, werr
				}
				if req.GetBody != nil {
					req.Body, _ = req.GetBody()
				}
				continue
			}
		}

		return resp, nil
	}

	return resp, err
}

// wait sleeps for d but aborts early if the request's context is cancelled,
// returning the context error in that case. This keeps backoff responsive to
// cancellation and to the surrounding Client.Timeout (which cancels the
// request context when it fires).
func (t *retryTransport) wait(req *http.Request, d time.Duration) error {
	ctx := req.Context()
	timer := time.NewTimer(d)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// retryAfterDelay parses a delta-seconds Retry-After header value and clamps it
// to MaxRetryAfter. It reports ok=false when the header is absent or not a
// non-negative integer (the HTTP-date form is not honored here, matching the
// previous behavior). Clamping prevents a server-controlled value from stalling
// the client indefinitely.
func retryAfterDelay(header string) (time.Duration, bool) {
	if header == "" {
		return 0, false
	}
	secs, err := strconv.Atoi(header)
	if err != nil || secs < 0 {
		return 0, false
	}
	wait := min(time.Duration(secs)*time.Second, MaxRetryAfter)
	return wait, true
}

// backoff returns an exponential backoff duration with jitter.
func backoff(attempt int) time.Duration {
	base := time.Duration(math.Pow(2, float64(attempt))) * 500 * time.Millisecond
	jitter := time.Duration(rand.Int64N(int64(base / 2)))
	return base + jitter
}
