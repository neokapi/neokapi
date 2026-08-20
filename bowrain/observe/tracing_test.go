package observe

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sentry "github.com/getsentry/sentry-go"
	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// capture installs a Sentry client that keeps events instead of sending them,
// and restores whatever was there before. Transactions arrive through the same
// transport as errors, so this sees them.
func capture(t *testing.T) *captureTransport {
	t.Helper()
	tr := &captureTransport{}
	prevHub := sentry.CurrentHub()
	prevEnabled := sentryEnabled

	client, err := sentry.NewClient(sentry.ClientOptions{
		Dsn:              "https://key@example.invalid/1",
		Transport:        tr,
		EnableTracing:    true,
		TracesSampleRate: 1.0,
	})
	require.NoError(t, err)
	sentry.CurrentHub().BindClient(client)
	sentryEnabled = true

	t.Cleanup(func() {
		sentryEnabled = prevEnabled
		sentry.SetHubOnContext(context.Background(), prevHub)
	})
	return tr
}

type captureTransport struct{ events []*sentry.Event }

func (t *captureTransport) Configure(sentry.ClientOptions) {}
func (t *captureTransport) SendEvent(e *sentry.Event)      { t.events = append(t.events, e) }
func (t *captureTransport) Flush(time.Duration) bool       { return true }
func (t *captureTransport) FlushWithContext(context.Context) bool {
	return true
}
func (t *captureTransport) Close() {}

func (t *captureTransport) transactions() []*sentry.Event {
	var out []*sentry.Event
	for _, e := range t.events {
		if e.Type == "transaction" {
			out = append(out, e)
		}
	}
	return out
}

// serve runs one request through an Echo instance carrying the request-id and
// tracing middleware, with the handler registered at the given route pattern.
func serve(t *testing.T, route, target string, h echo.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	e := echo.New()
	e.Use(RequestIDMiddleware())
	e.Use(TracingMiddleware())
	e.GET(route, h)

	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	return rec
}

// A transaction is named by the route it matched, never by the URL it was
// asked for.
//
// This is the property the whole thing turns on. Naming a transaction
// `/api/v1/acme/p_9f3/dashboard/main` mints a separate transaction for every
// workspace and project id, so nothing aggregates and the slow route is
// invisible in a list of thousands of one-off names — which is the same failure
// as having no transactions at all, with more data.
func TestTracing_TransactionIsNamedByRouteNotURL(t *testing.T) {
	tr := capture(t)

	serve(t, "/api/v1/:ws/:id/dashboard/:ref", "/api/v1/acme/p_9f3/dashboard/main",
		func(c echo.Context) error { return c.NoContent(http.StatusOK) })

	txns := tr.transactions()
	require.Len(t, txns, 1)
	assert.Equal(t, "GET /api/v1/:ws/:id/dashboard/:ref", txns[0].Transaction)
	assert.NotContains(t, txns[0].Transaction, "acme")
	assert.NotContains(t, txns[0].Transaction, "p_9f3")
}

// The request id rides on the transaction, which is what lets a trace and a log
// line be joined: the same id is in the response header, in every log record
// for the request, and on any error captured under it.
func TestTracing_CarriesTheRequestID(t *testing.T) {
	tr := capture(t)

	rec := serve(t, "/x", "/x", func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	header := rec.Header().Get(RequestIDHeader)
	require.NotEmpty(t, header)

	txns := tr.transactions()
	require.Len(t, txns, 1)
	assert.Equal(t, header, txns[0].Tags["request_id"])
	assert.Equal(t, header, txns[0].Tags["reference"])
}

// A slow success is the case this exists for: no error, status 200, and the
// only signal that anything is wrong is the duration.
func TestTracing_RecordsASlowSuccess(t *testing.T) {
	tr := capture(t)

	serve(t, "/slow", "/slow", func(c echo.Context) error {
		time.Sleep(15 * time.Millisecond)
		return c.NoContent(http.StatusOK)
	})

	txns := tr.transactions()
	require.Len(t, txns, 1)
	assert.Equal(t, sentry.SpanStatusOK, txns[0].Contexts["trace"]["status"])
	elapsed := txns[0].Timestamp.Sub(txns[0].StartTime)
	assert.GreaterOrEqual(t, elapsed, 10*time.Millisecond,
		"the transaction must span the handler, or a slow route reads as fast")
}

// A 4xx is the caller being refused, not this service failing. Reporting both
// as internal errors makes the failure rate mean nothing.
func TestTracing_SeparatesRefusalFromFailure(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		want   sentry.SpanStatus
	}{
		{"ok", http.StatusOK, sentry.SpanStatusOK},
		{"unauthorized", http.StatusUnauthorized, sentry.SpanStatusUnauthenticated},
		{"forbidden", http.StatusForbidden, sentry.SpanStatusPermissionDenied},
		{"rate limited", http.StatusTooManyRequests, sentry.SpanStatusResourceExhausted},
		{"server error", http.StatusInternalServerError, sentry.SpanStatusInternalError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tr := capture(t)
			serve(t, "/s", "/s", func(c echo.Context) error { return c.NoContent(tc.status) })

			txns := tr.transactions()
			require.Len(t, txns, 1)
			assert.Equal(t, tc.want, txns[0].Contexts["trace"]["status"])
		})
	}
}

// Spans opened by the work below the middleware attach to the request's
// transaction. Without this a span is an orphan and says nothing about which
// request paid for it.
func TestTracing_SpansAttachToTheRequestsTransaction(t *testing.T) {
	tr := capture(t)

	serve(t, "/q", "/q", func(c echo.Context) error {
		end := StartSpan(c.Request().Context(), "db.query", "store.GetBlocks whole-stream")
		time.Sleep(5 * time.Millisecond)
		end()
		return c.NoContent(http.StatusOK)
	})

	txns := tr.transactions()
	require.Len(t, txns, 1)
	require.Len(t, txns[0].Spans, 1, "the span must be carried by the transaction, not orphaned")
	assert.Equal(t, "db.query", txns[0].Spans[0].Op)
	assert.Equal(t, "store.GetBlocks whole-stream", txns[0].Spans[0].Description)
}

// A span opened outside any request is dropped rather than sent on its own.
//
// An orphan trace holding one span reads in Sentry as a request that did
// nothing else, which is worse than absent data — it invents a request that
// never happened. Background jobs and tests take this path.
func TestStartSpan_WithoutATransactionIsANoOp(t *testing.T) {
	tr := capture(t)

	end := StartSpan(context.Background(), "db.query", "store.GetBlocks whole-stream")
	end()

	assert.Empty(t, tr.transactions())
}

// With Sentry unconfigured — the default for local and self-hosted runs —
// nothing is started and nothing is sent.
func TestTracing_NoOpWhenSentryDisabled(t *testing.T) {
	tr := capture(t)
	sentryEnabled = false

	rec := serve(t, "/x", "/x", func(c echo.Context) error { return c.NoContent(http.StatusOK) })

	assert.Equal(t, http.StatusOK, rec.Code, "the request is served either way")
	assert.Empty(t, tr.transactions())
}

// A handler that FAILS by returning an error is recorded as that failure.
//
// This is the case the response object cannot answer on its own: Echo's error
// handler writes the status after the middleware chain unwinds, so at the point
// the transaction is finished the response still reads as the zero-value 200. A
// middleware that trusted it would file every 500 as a success — backwards for
// the one signal this exists to produce.
func TestTracing_AnErrorReturnedIsNotRecordedAsSuccess(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want sentry.SpanStatus
	}{
		{"http error", echo.NewHTTPError(http.StatusInternalServerError, "boom"), sentry.SpanStatusInternalError},
		{"http error 503", echo.NewHTTPError(http.StatusServiceUnavailable), sentry.SpanStatusUnavailable},
		{"plain error", errors.New("boom"), sentry.SpanStatusInternalError},
		{"refusal", echo.NewHTTPError(http.StatusForbidden), sentry.SpanStatusPermissionDenied},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tr := capture(t)
			serve(t, "/e", "/e", func(echo.Context) error { return tc.err })

			txns := tr.transactions()
			require.Len(t, txns, 1)
			assert.Equal(t, tc.want, txns[0].Contexts["trace"]["status"])
		})
	}
}

// A committed response is authoritative even when the handler then fails: it
// answered what it wrote.
func TestTracing_ACommittedResponseWins(t *testing.T) {
	tr := capture(t)

	serve(t, "/c", "/c", func(c echo.Context) error {
		_ = c.NoContent(http.StatusOK)
		return errors.New("failed after answering")
	})

	txns := tr.transactions()
	require.Len(t, txns, 1)
	assert.Equal(t, sentry.SpanStatusOK, txns[0].Contexts["trace"]["status"])
}

// A request that matched no route produces no transaction at all — the SDK
// drops 404s (ClientOptions.TraceIgnoreStatusCodes defaults to {{404}}).
//
// That is the behaviour worth having: the internet scans for /wp-admin and
// /.env continuously and none of it is a performance signal. Asserted so that
// an empty 404 list in Sentry reads as intended rather than as this middleware
// being broken, and so a future change to that default is noticed here.
func TestTracing_UnmatchedRoutesAreDroppedBySentry(t *testing.T) {
	tr := capture(t)

	e := echo.New()
	e.Use(RequestIDMiddleware())
	e.Use(TracingMiddleware())
	e.GET("/known", func(c echo.Context) error { return c.NoContent(http.StatusOK) })

	for _, p := range []string{"/wp-admin", "/.env", "/phpmyadmin"} {
		e.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, p, nil))
	}

	assert.Empty(t, tr.transactions(), "scanner traffic must not fill Sentry")
}

// A traces sample rate above zero must actually enable tracing.
//
// sentry-go drops every transaction when EnableTracing is false, whatever the
// sample rate says — so setting only the rate yields a deployment that reads as
// instrumented, reports errors normally, and discards every transaction. That
// is the same fault this whole file exists to fix, one layer down, and it is
// invisible from the configuration.
func TestInitSentry_ATracesRateEnablesTracing(t *testing.T) {
	prevHub, prevEnabled := sentry.CurrentHub(), sentryEnabled
	t.Cleanup(func() {
		sentry.SetHubOnContext(context.Background(), prevHub)
		sentryEnabled = prevEnabled
	})

	for _, tc := range []struct {
		name string
		rate float64
		want bool
	}{
		{"sampled", 1.0, true},
		{"partially sampled", 0.1, true},
		{"off", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.True(t, InitSentry(SentryConfig{
				DSN:              "https://key@example.invalid/1",
				TracesSampleRate: tc.rate,
			}))
			opts := sentry.CurrentHub().Client().Options()
			assert.Equal(t, tc.want, opts.EnableTracing,
				"a rate of %v without EnableTracing=%v drops every transaction", tc.rate, tc.want)
			assert.Equal(t, tc.rate, opts.TracesSampleRate)
		})
	}
}

// The whole chain, configured the way production configures it.
//
// Every other test here builds its Sentry client by hand and can therefore set
// EnableTracing without noticing it matters. This one goes through InitSentry
// with nothing but a DSN and a sample rate — the two things the deployment
// actually sets — and asserts a transaction comes out the other end. It is the
// test that would have failed on the version of this change that added the
// middleware and left EnableTracing alone.
func TestTracing_EndToEndThroughInitSentry(t *testing.T) {
	prevHub, prevEnabled := sentry.CurrentHub(), sentryEnabled
	t.Cleanup(func() {
		sentry.SetHubOnContext(context.Background(), prevHub)
		sentryEnabled = prevEnabled
	})

	tr := &captureTransport{}
	require.True(t, InitSentry(SentryConfig{
		DSN:              "https://key@example.invalid/1",
		Environment:      "prod",
		TracesSampleRate: 1.0,
	}))
	// Swap in the capturing transport, keeping everything else InitSentry chose.
	opts := sentry.CurrentHub().Client().Options()
	opts.Transport = tr
	client, err := sentry.NewClient(opts)
	require.NoError(t, err)
	sentry.CurrentHub().BindClient(client)

	serve(t, "/api/v1/:ws/:id/dashboard/:ref", "/api/v1/acme/p_9f3/dashboard/main",
		func(c echo.Context) error {
			end := StartSpan(c.Request().Context(), "db.query", "store.GetBlocks whole-stream")
			defer end()
			return c.NoContent(http.StatusOK)
		})

	txns := tr.transactions()
	require.Len(t, txns, 1, "a sample rate the deployment sets must produce a transaction")
	assert.Equal(t, "GET /api/v1/:ws/:id/dashboard/:ref", txns[0].Transaction)
	require.Len(t, txns[0].Spans, 1)
	assert.Equal(t, "store.GetBlocks whole-stream", txns[0].Spans[0].Description)
}
