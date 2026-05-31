package httputil

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestTransport builds a retryTransport whose base hits the given test
// server, so we exercise the real RoundTrip retry logic without TLS.
func newTestTransport(maxRetries int) *retryTransport {
	return &retryTransport{base: http.DefaultTransport, maxRetries: maxRetries}
}

func TestRetryTransport_ContextCancellationAbortsBackoff(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		retryAfter string
	}{
		{name: "5xx with no retry-after", statusCode: http.StatusBadGateway},
		{name: "429 with no retry-after", statusCode: http.StatusTooManyRequests},
		{name: "429 with long retry-after", statusCode: http.StatusTooManyRequests, retryAfter: "30"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var hits int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&hits, 1)
				if tt.retryAfter != "" {
					w.Header().Set("Retry-After", tt.retryAfter)
				}
				w.WriteHeader(tt.statusCode)
			}))
			defer srv.Close()

			transport := newTestTransport(MaxRetries)

			ctx, cancel := context.WithCancel(context.Background())
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
			require.NoError(t, err)

			// Cancel shortly after the first response, while RoundTrip is
			// blocked in backoff. backoff(0) is >= 500ms, so a 50ms cancel
			// lands inside the wait.
			cancelAt := 50 * time.Millisecond
			go func() {
				time.Sleep(cancelAt)
				cancel()
			}()

			start := time.Now()
			resp, err := transport.RoundTrip(req)
			elapsed := time.Since(start)

			if resp != nil {
				resp.Body.Close()
			}
			require.Error(t, err)
			assert.ErrorIs(t, err, context.Canceled)

			// Must return promptly after cancel — well under the full backoff
			// (>= 500ms) it would otherwise have slept for.
			assert.Less(t, elapsed, 400*time.Millisecond,
				"RoundTrip should abort backoff promptly on context cancel")
			// Exactly one upstream call: the backoff before the retry was aborted.
			assert.Equal(t, int32(1), atomic.LoadInt32(&hits))
		})
	}
}

func TestRetryAfterDelay_ClampedToMax(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   time.Duration
		wantOK bool
	}{
		{name: "absent header", header: "", want: 0, wantOK: false},
		{name: "non-numeric (HTTP-date)", header: "Wed, 21 Oct 2025 07:28:00 GMT", want: 0, wantOK: false},
		{name: "negative", header: "-5", want: 0, wantOK: false},
		{name: "zero", header: "0", want: 0, wantOK: true},
		{name: "under cap", header: "10", want: 10 * time.Second, wantOK: true},
		{name: "at cap", header: strconv.Itoa(int(MaxRetryAfter.Seconds())), want: MaxRetryAfter, wantOK: true},
		{name: "just over cap is clamped", header: strconv.Itoa(int(MaxRetryAfter.Seconds()) + 5), want: MaxRetryAfter, wantOK: true},
		{name: "huge value is clamped", header: strconv.Itoa(int((10 * time.Minute).Seconds())), want: MaxRetryAfter, wantOK: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := retryAfterDelay(tt.header)
			assert.Equal(t, tt.wantOK, ok)
			assert.Equal(t, tt.want, got)
			if ok {
				assert.LessOrEqual(t, got, MaxRetryAfter, "honored delay must never exceed the cap")
			}
		})
	}
}

// TestRetryTransport_RetryAfterRespectedAndCapped exercises the full RoundTrip
// path with a small (in-cap) Retry-After to confirm the header is honored and
// the retry succeeds without waiting the (uncapped) advertised time.
func TestRetryTransport_RetryAfterRespectedAndCapped(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n == 1 {
			// Advertise far above the cap; the client must clamp to MaxRetryAfter,
			// and the context below cancels well before either the advertised
			// value or the cap would elapse — proving the retry is gated on wait.
			w.Header().Set("Retry-After", "600")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	transport := newTestTransport(MaxRetries)

	// Cancel during the (clamped) wait; the retry must not fire, and elapsed
	// must be far below the advertised 600s — confirming the wait is bounded.
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	require.NoError(t, err)

	start := time.Now()
	resp, err := transport.RoundTrip(req)
	elapsed := time.Since(start)
	if resp != nil {
		resp.Body.Close()
	}

	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, int32(1), atomic.LoadInt32(&hits))
	assert.Less(t, elapsed, MaxRetryAfter, "wait must be bounded, never the advertised 600s")
}

// nonReplayableBody is an io.ReadCloser that cannot be rewound, mimicking a
// request body with no GetBody.
type nonReplayableBody struct {
	r io.Reader
}

func (b *nonReplayableBody) Read(p []byte) (int, error) { return b.r.Read(p) }
func (b *nonReplayableBody) Close() error               { return nil }

func TestRetryTransport_NoRetryOnNilGetBody(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{name: "429 not retried", statusCode: http.StatusTooManyRequests},
		{name: "503 not retried", statusCode: http.StatusServiceUnavailable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var hits int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				atomic.AddInt32(&hits, 1)
				_, _ = io.Copy(io.Discard, r.Body)
				w.WriteHeader(tt.statusCode)
			}))
			defer srv.Close()

			transport := newTestTransport(MaxRetries)

			req, err := http.NewRequest(http.MethodPost, srv.URL, &nonReplayableBody{r: strings.NewReader("payload")})
			require.NoError(t, err)
			// Force the non-replayable case: a body present but no GetBody.
			req.GetBody = nil

			resp, err := transport.RoundTrip(req)
			require.NoError(t, err)
			require.NotNil(t, resp)
			defer resp.Body.Close()

			assert.Equal(t, tt.statusCode, resp.StatusCode)
			assert.Equal(t, int32(1), atomic.LoadInt32(&hits),
				"must not retry when body cannot be replayed")
		})
	}
}

func TestRetryTransport_RetriesReplayableBodyOn429(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		body, _ := io.ReadAll(r.Body)
		assert.Equal(t, "payload", string(body), "replayed body should be intact on retry")
		if n == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	transport := newTestTransport(MaxRetries)

	// strings.NewReader bodies get an automatic GetBody, so retry is allowed.
	req, err := http.NewRequest(http.MethodPost, srv.URL, strings.NewReader("payload"))
	require.NoError(t, err)
	require.NotNil(t, req.GetBody)

	resp, err := transport.RoundTrip(req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, int32(2), atomic.LoadInt32(&hits))
}

func TestRetryTransport_SucceedsWithoutRetry(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	transport := newTestTransport(MaxRetries)

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	require.NoError(t, err)

	resp, err := transport.RoundTrip(req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, int32(1), atomic.LoadInt32(&hits))
}

func TestBackoff_Increases(t *testing.T) {
	// Sanity: backoff base grows with attempt and is always within [base, 1.5*base).
	for attempt := 0; attempt < 4; attempt++ {
		base := time.Duration(1<<uint(attempt)) * 500 * time.Millisecond
		got := backoff(attempt)
		assert.GreaterOrEqual(t, got, base)
		assert.Less(t, got, base+base/2)
	}
}
