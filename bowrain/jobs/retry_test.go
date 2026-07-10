package jobs

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsTransientError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil is not transient", err: nil, want: false},

		// Provider 5xx / overload / rate-limit → transient.
		{name: "openai 503", err: errors.New("openai: API error 503: service unavailable"), want: true},
		{name: "azureopenai 500", err: errors.New("azureopenai: API error 500: internal"), want: true},
		{name: "gemini 502", err: errors.New("gemini: API error 502: bad gateway"), want: true},
		{name: "504 gateway timeout", err: errors.New("openai: API error 504: gateway timeout"), want: true},
		{name: "anthropic 529 overloaded", err: errors.New("anthropic: API error 529: overloaded"), want: true},
		{name: "429 rate limit", err: errors.New("anthropic: API error 429: rate limit exceeded"), want: true},

		// Permanent 4xx → not transient.
		{name: "400 bad request", err: errors.New("openai: API error 400: bad request"), want: false},
		{name: "401 unauthorized", err: errors.New("openai: API error 401: invalid api key"), want: false},
		{name: "403 forbidden", err: errors.New("openai: API error 403: forbidden"), want: false},
		{name: "404 not found", err: errors.New("openai: API error 404: model not found"), want: false},
		{name: "422 unprocessable", err: errors.New("openai: API error 422: unprocessable"), want: false},

		// Non-provider permanent failures.
		{name: "missing project", err: errors.New("get project: not found"), want: false},
		{name: "store blocks", err: errors.New("store blocks: constraint violation"), want: false},

		// Network / timeout → transient, including wrapped forms.
		{name: "context deadline", err: context.DeadlineExceeded, want: true},
		{name: "wrapped deadline", err: fmt.Errorf("translate chunk 0-50: %w", context.DeadlineExceeded), want: true},
		{name: "connection refused", err: errors.New("dial tcp 127.0.0.1:443: connection refused"), want: true},
		{name: "connection reset", err: errors.New("read: connection reset by peer"), want: true},
		{name: "no such host", err: errors.New("dial tcp: lookup api.openai.com: no such host"), want: true},
		{name: "net timeout error", err: &timeoutError{}, want: true},

		// Wrapped provider error keeps its classification through %w.
		{
			name: "wrapped provider 503",
			err:  fmt.Errorf("resolve provider: %w", fmt.Errorf("translate chunk 0-50: %w", errors.New("openai: API error 503: down"))),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isTransientError(tt.err))
		})
	}
}

func TestTransientErrorUnwraps(t *testing.T) {
	base := errors.New("openai: API error 503: down")
	te := &transientError{err: base}

	assert.Equal(t, base.Error(), te.Error())
	require.ErrorIs(t, te, base)

	var got *transientError
	assert.ErrorAs(t, fmt.Errorf("wrapped: %w", te), &got)
}

// timeoutError is a net.Error whose Timeout() reports true.
type timeoutError struct{}

func (*timeoutError) Error() string   { return "i/o operation stalled" }
func (*timeoutError) Timeout() bool   { return true }
func (*timeoutError) Temporary() bool { return true }

var _ net.Error = (*timeoutError)(nil)

func TestDefaultMaxJobAttemptsAlignsWithBroker(t *testing.T) {
	// The DB attempt budget and the NATS redelivery cap must agree so an
	// exhausted retry ACK+fails rather than looping.
	assert.Equal(t, natsMaxDeliver, defaultMaxJobAttempts)
	// Sanity: the stale threshold sits above the broker ack-wait so the sweeper
	// does not race in-flight redelivery.
	assert.Greater(t, defaultStaleJobThreshold, natsAckWait)
}
