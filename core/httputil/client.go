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
				time.Sleep(backoff(attempt))
				if req.GetBody != nil {
					req.Body, _ = req.GetBody()
				}
				continue
			}
			return nil, err
		}

		// Retry on 429 (rate limited) or 5xx (server error).
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			if attempt < t.maxRetries {
				wait := backoff(attempt)
				// Respect Retry-After header if present.
				if ra := resp.Header.Get("Retry-After"); ra != "" {
					if secs, err := strconv.Atoi(ra); err == nil {
						wait = time.Duration(secs) * time.Second
					}
				}
				resp.Body.Close()
				time.Sleep(wait)
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

// backoff returns an exponential backoff duration with jitter.
func backoff(attempt int) time.Duration {
	base := time.Duration(math.Pow(2, float64(attempt))) * 500 * time.Millisecond
	jitter := time.Duration(rand.Int64N(int64(base / 2)))
	return base + jitter
}
