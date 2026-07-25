package resilience

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// Transport wraps base so every request through it is guarded by the named
// breaker. This is how the platform gets *one* breaker abstraction across a
// dozen unrelated HTTP integrations: the guard attaches to the client, not to
// each call site, so a connector's own code is untouched and cannot forget it.
//
// base may be nil, meaning http.DefaultTransport. Layering matters: put the
// breaker outermost, above any retrying transport, so one logical call is one
// observation rather than one per retry.
func Transport(name string, kind Kind, base http.RoundTripper) http.RoundTripper {
	// Idempotent: guarding an already-guarded transport for the same dependency
	// is a no-op. Call sites layer (a rewriting transport wrapped by a guard,
	// wrapped again at a higher boundary) and a stack of guards would count one
	// call several times.
	if bt, ok := base.(*breakerTransport); ok && bt.breaker.Name() == name {
		return bt
	}
	return &breakerTransport{breaker: Get(name, kind), base: base}
}

// TransportFor is [Transport] against an explicit registry, for tests.
func TransportFor(r *Registry, name string, kind Kind, base http.RoundTripper) http.RoundTripper {
	return &breakerTransport{breaker: r.Get(name, kind), base: base}
}

// Client returns an http.Client guarded by the named breaker, with an explicit
// timeout. Both halves matter: the timeout bounds a single hung call, the
// breaker bounds a *sustained* outage. Neither substitutes for the other, and
// a client with only one of them is the gap this replaces.
func Client(name string, kind Kind, timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: Transport(name, kind, nil),
	}
}

// Guard adds breaker protection to an existing client, preserving its timeout
// and transport. It returns a copy; the original is untouched. Use it when a
// dependency's client is already configured (a rewriting transport, a unix
// socket dialer) and only the guard is missing. A nil client is treated as a
// fresh client with the given fallback timeout.
func Guard(c *http.Client, name string, kind Kind, fallbackTimeout time.Duration) *http.Client {
	if c == nil {
		return Client(name, kind, fallbackTimeout)
	}
	guarded := *c
	if guarded.Timeout == 0 {
		guarded.Timeout = fallbackTimeout
	}
	guarded.Transport = Transport(name, kind, c.Transport)
	return &guarded
}

type breakerTransport struct {
	breaker *Breaker
	base    http.RoundTripper
}

func (t *breakerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}

	var (
		resp  *http.Response
		rtErr error
	)
	err := t.breaker.Do(req.Context(), func(context.Context) error {
		resp, rtErr = base.RoundTrip(req)
		if rtErr != nil {
			return rtErr
		}
		return statusFailure(resp.StatusCode)
	})

	// Only an open circuit changes what the caller sees; a status-derived
	// failure was purely a signal to the breaker and the response still stands.
	if IsUnavailable(err) {
		return nil, err
	}
	if rtErr != nil {
		return nil, rtErr
	}
	return resp, nil
}

// statusFailure turns a response code into a breaker signal. 5xx and 429 mean
// the dependency is in trouble; every other code — including 4xx — means it
// answered correctly and simply refused, which is not a health signal.
func statusFailure(code int) error {
	if code >= 500 || code == http.StatusTooManyRequests {
		return fmt.Errorf("upstream API error %d", code)
	}
	return nil
}
