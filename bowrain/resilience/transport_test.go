package resilience

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fastRegistry is a registry whose breakers trip after 4 failures and stay open
// long enough for a test to observe it.
func fastRegistry() *Registry {
	r := NewRegistry()
	r.Configure(Overrides{
		MinimumRequests: new(4),
		FailureRatio:    new(0.5),
		CooldownSeconds: new(60),
	})
	return r
}

// reply is what a test needs from a response once the body has been settled.
// Returning this rather than the *http.Response keeps body ownership entirely
// inside get, so no test can leak a connection.
type reply struct {
	status int
	header http.Header
}

// get issues a GET through c, drains and closes the body, and returns the
// status and headers. Keeping the plumbing here leaves each test about breaker
// behaviour.
func get(t *testing.T, c *http.Client, url string) (reply, error) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	require.NoError(t, err)
	resp, err := c.Do(req)
	if err != nil {
		return reply{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	return reply{status: resp.StatusCode, header: resp.Header}, nil
}

func TestTransportTripsOnServerErrors(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	r := fastRegistry()
	c := &http.Client{Timeout: 5 * time.Second, Transport: TransportFor(r, "connector:test", KindConnector, nil)}

	for range 4 {
		resp, err := get(t, c, srv.URL)
		require.NoError(t, err, "a 503 is a response, not a transport error")
		require.Equal(t, http.StatusServiceUnavailable, resp.status)
	}
	require.Equal(t, StateOpen, r.Get("connector:test", KindConnector).State())

	// The circuit now sheds load: the server must see no further requests.
	before := hits
	_, err := get(t, c, srv.URL)
	require.Error(t, err)
	assert.True(t, IsUnavailable(err), "url.Error must still unwrap to ErrUnavailable, got %v", err)
	assert.Equal(t, before, hits, "an open circuit must not reach the server")
}

func TestTransportTripsOnTooManyRequests(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	r := fastRegistry()
	c := &http.Client{Transport: TransportFor(r, "connector:throttled", KindConnector, nil)}
	for range 4 {
		_, err := get(t, c, srv.URL)
		require.NoError(t, err)
	}
	assert.Equal(t, StateOpen, r.Get("connector:throttled", KindConnector).State(),
		"sustained throttling is a dependency we should back off from")
}

func TestTransportIgnoresClientErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	r := fastRegistry()
	c := &http.Client{Transport: TransportFor(r, "connector:badcreds", KindConnector, nil)}

	// A misconfigured token yields an endless stream of 401s. The dependency is
	// perfectly healthy and must stay callable for everyone else.
	for range 20 {
		resp, err := get(t, c, srv.URL)
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, resp.status)
	}
	assert.Equal(t, StateClosed, r.Get("connector:badcreds", KindConnector).State())
}

func TestTransportTripsOnTransportFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // nothing is listening now

	r := fastRegistry()
	c := &http.Client{Transport: TransportFor(r, "connector:dead", KindConnector, nil)}
	for range 4 {
		_, err := get(t, c, url)
		require.Error(t, err)
	}
	assert.Equal(t, StateOpen, r.Get("connector:dead", KindConnector).State())
}

func TestTransportPassesSuccessThroughUntouched(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Marker", "yes")
		_, _ = w.Write([]byte("body"))
	}))
	defer srv.Close()

	c := &http.Client{Transport: TransportFor(fastRegistry(), "connector:ok", KindConnector, nil)}
	resp, err := get(t, c, srv.URL)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.status)
	assert.Equal(t, "yes", resp.header.Get("X-Marker"))
}

// countingTransport records how many times it was invoked.
type countingTransport struct {
	calls int
	base  http.RoundTripper
}

func (t *countingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.calls++
	return t.base.RoundTrip(req)
}

func TestGuardPreservesTimeoutAndBaseTransport(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()

	inner := &countingTransport{base: http.DefaultTransport}
	orig := &http.Client{Timeout: 7 * time.Second, Transport: inner}
	guarded := Guard(orig, "connector:layered", KindConnector, 30*time.Second)

	assert.NotSame(t, orig, guarded, "Guard must not mutate the caller's client")
	assert.Same(t, inner, orig.Transport, "the original client keeps its transport")
	assert.Equal(t, 7*time.Second, guarded.Timeout, "an explicit timeout is preserved")

	_, err := get(t, guarded, srv.URL)
	require.NoError(t, err)
	assert.Equal(t, 1, inner.calls, "the guard must delegate to the wrapped transport")
}

func TestGuardSuppliesFallbackTimeout(t *testing.T) {
	// The gap this closes: a client with no timeout at all can hang a goroutine
	// on an unresponsive peer forever.
	guarded := Guard(&http.Client{}, "identity:test", KindIdentity, 9*time.Second)
	assert.Equal(t, 9*time.Second, guarded.Timeout)

	assert.Equal(t, 9*time.Second, Guard(nil, "identity:test", KindIdentity, 9*time.Second).Timeout)
}

func TestTransportGuardIsIdempotent(t *testing.T) {
	once := Transport("connector:idem", KindConnector, nil)
	twice := Transport("connector:idem", KindConnector, once)
	assert.Same(t, once, twice, "re-guarding the same dependency must not stack guards")

	// A different dependency legitimately layers.
	other := Transport("connector:other", KindConnector, once)
	assert.NotSame(t, once, other)
}

func TestStatusFailureClassification(t *testing.T) {
	for _, code := range []int{500, 502, 503, 504, 429} {
		require.Error(t, statusFailure(code), "HTTP %d must count against the breaker", code)
		assert.True(t, IsUpstreamFailure(statusFailure(code)),
			"HTTP %d must classify as an upstream failure", code)
	}
	for _, code := range []int{200, 201, 204, 301, 400, 401, 403, 404, 409, 422} {
		assert.NoError(t, statusFailure(code), "HTTP %d must not count against the breaker", code)
	}
}

func TestUnavailableErrorShape(t *testing.T) {
	e := &UnavailableError{Dependency: "ai:bedrock", Kind: KindAI, RetryAfter: 12 * time.Second}
	assert.Equal(t, 12, e.RetryAfterSeconds())
	assert.Contains(t, e.Error(), "ai:bedrock")
	require.ErrorIs(t, e, ErrUnavailable)

	// Sub-second and negative estimates must still be a usable Retry-After.
	assert.Equal(t, 1, (&UnavailableError{RetryAfter: 10 * time.Millisecond}).RetryAfterSeconds())
	assert.Equal(t, 1, (&UnavailableError{RetryAfter: -5 * time.Second}).RetryAfterSeconds())

	// Rounds up so a client never retries a moment too early.
	assert.Equal(t, 3, (&UnavailableError{RetryAfter: 2100 * time.Millisecond}).RetryAfterSeconds())
}

func TestIsUpstreamFailureClassification(t *testing.T) {
	assert.False(t, IsUpstreamFailure(nil))
	assert.False(t, IsUpstreamFailure(errors.New("openai: API error 401: bad key")))
	assert.False(t, IsUpstreamFailure(errors.New("project not found")))
	assert.False(t, IsUpstreamFailure(CallerFault(errUpstream)))
	assert.False(t, IsUpstreamFailure(&UnavailableError{Dependency: "ai:x"}),
		"an open-circuit rejection must not be re-counted as an upstream failure")

	assert.True(t, IsUpstreamFailure(errors.New("anthropic: API error 529: overloaded")))
	assert.True(t, IsUpstreamFailure(errors.New("dial tcp: connection refused")))
	assert.True(t, IsUpstreamFailure(errors.New("Client.Timeout exceeded while awaiting headers")))
	assert.True(t, IsUpstreamFailure(errors.New("rate limit reached")))
}

func TestCallerFaultNilIsNil(t *testing.T) {
	assert.NoError(t, CallerFault(nil))
}
