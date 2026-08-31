package observe

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	sentry "github.com/getsentry/sentry-go"
	"github.com/labstack/echo/v4"
)

// Tracing produces the Sentry transactions the traces sample rate exists to
// sample.
//
// sentry-go does not auto-instrument. Setting SENTRY_TRACES_SAMPLE_RATE turns on
// a pipeline and nothing feeds it, so a slow endpoint that returns 200 stays
// invisible in Sentry however high the rate is set — which is exactly what
// happened to a dashboard route running at 22 seconds at the MEDIAN across 38
// requests, every one of them a success (#2105). Errors were reported the whole
// time; nothing was wrong, it was just slow, and error reporting has nothing to
// say about that.

// spanOriginBowrain marks these spans as produced by this repository's own
// instrumentation rather than by an SDK integration, so a future move to an
// off-the-shelf one is visible in the data.
const spanOriginBowrain = "manual.http.bowrain"

// A 404 transaction is dropped by the SDK, not by anything here:
// sentry.ClientOptions.TraceIgnoreStatusCodes defaults to [][]int{{404}}, so a
// request that matched no route never reaches Sentry as a transaction. That is
// the behaviour worth having — the internet scans for /wp-admin and /.env all
// day and none of it is a performance signal — but it is worth writing down,
// because the alternative reading of an empty 404 list in Sentry is that this
// middleware is broken.

// TracingMiddleware opens one Sentry transaction per request.
//
// Place it INSIDE RequestIDMiddleware, so the request id exists to tag the
// transaction with, and OUTSIDE middleware.Recover, so a panic the recover
// middleware turns into a 500 is still observed here and the transaction
// records the failure rather than ending in whatever state the unwind left.
//
// It is a no-op when Sentry is not configured, which is the default for
// local and self-hosted deployments.
func TracingMiddleware() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if !sentryEnabled || isHealthProbe(c.Path()) {
				return next(c)
			}

			req := c.Request()
			ctx := req.Context()

			// ContinueFromRequest adopts an inbound sentry-trace/baggage header
			// when there is one, so a browser-initiated trace and this server's
			// transaction are one trace rather than two unrelated ones.
			transaction := sentry.StartTransaction(ctx, transactionName(c),
				sentry.ContinueFromRequest(req),
				sentry.WithOpName("http.server"),
				// SourceRoute tells Sentry the name is a parameterized route, so
				// it aggregates. Naming a transaction by the concrete path would
				// mint a separate one per workspace and project id and aggregate
				// nothing — the reason this reuses the matched route rather than
				// the URL.
				sentry.WithTransactionSource(sentry.SourceRoute),
				sentry.WithSpanOrigin(spanOriginBowrain),
			)
			defer transaction.Finish()

			transaction.SetTag("http.method", req.Method)
			transaction.SetTag("http.route", c.Path())
			// The handle a person actually has: it is in the response header, in
			// every log line for this request, and in any Sentry error raised
			// under it. Tagging the transaction with it is what lets a trace and
			// a log line be joined.
			if reqID, ok := c.Get("request_id").(string); ok && reqID != "" {
				transaction.SetTag("request_id", reqID)
				transaction.SetTag("reference", reqID)
			}

			// Hand the transaction's context down so spans opened by anything
			// below attach to this transaction rather than starting their own.
			c.SetRequest(req.WithContext(transaction.Context()))

			err := next(c)

			// Read AFTER the handler, not before: the workspace, plan and user
			// are put on the context by the auth middleware, which runs inside
			// this one. Tagging on the way in would find them all empty and the
			// traces would be unfilterable — which is the failure this is here
			// to prevent, arrived at by instrumenting in the wrong order.
			// transaction.Context(), not the ctx captured above: that one
			// predates the transaction and carries no span, so tagging through
			// it silently writes nothing.
			TagScope(transaction.Context(), echoScope(c))

			status := responseStatus(c, err)
			transaction.SetTag("http.status_code", strconv.Itoa(status))
			transaction.SetData("http.response.status_code", status)
			transaction.Status = spanStatus(status)
			return err
		}
	}
}

// echoScope reads the request's dimensions off the Echo context.
//
// Every one of these is already set by middleware that runs for authenticated
// routes — the auth middleware resolves the workspace and its plan, the router
// binds the project id. Nothing is looked up here: an extra query per request
// to enrich telemetry would be a fine way to make the thing measuring latency
// a cause of it.
func echoScope(c echo.Context) Scope {
	str := func(key string) string {
		v, _ := c.Get(key).(string)
		return v
	}
	return Scope{
		WorkspaceID: str("workspace_id"),
		// :id is the project on every workspace-scoped route (AD-011).
		ProjectID: c.Param("id"),
		Plan:      str("workspace_plan"),
		Feature:   FeatureFromRoute(c.Path()),
		UserID:    str("user_id"),
	}
}

// responseStatus is the status this request will answer with.
//
// Not simply c.Response().Status: a handler that FAILS by returning an error
// has written no header yet — Echo's error handler does that after the
// middleware chain unwinds — so the response still reads as the zero-value 200
// here. Taking it at face value records a failed request as a success, which is
// precisely backwards for the signal this middleware exists to produce.
//
// Once the response is committed it is authoritative, error or not: a handler
// that wrote a 200 and then failed did answer 200.
func responseStatus(c echo.Context, err error) int {
	if err == nil || c.Response().Committed {
		return c.Response().Status
	}
	if he, ok := errors.AsType[*echo.HTTPError](err); ok {
		return he.Code
	}
	return http.StatusInternalServerError
}

// transactionName is the matched route, which is what aggregates.
//
// c.Path() is the registered pattern (`/api/v1/:ws/:id/dashboard/:ref`) because
// Echo routes before it runs `Use` middleware. It is empty for a request that
// matched no route; naming those by method alone keeps a scan for a missing
// endpoint from being spread across every URL someone probed.
func transactionName(c echo.Context) string {
	route := c.Path()
	if route == "" {
		return c.Request().Method + " unmatched"
	}
	return c.Request().Method + " " + route
}

// spanStatus maps an HTTP status onto the span status Sentry filters on.
//
// The distinction that matters is 4xx from 5xx: a request refused because the
// caller asked for something they may not have is not this service failing, and
// lumping the two together makes the failure rate meaningless.
func spanStatus(status int) sentry.SpanStatus {
	switch {
	case status >= 200 && status < 400:
		return sentry.SpanStatusOK
	case status == http.StatusUnauthorized:
		return sentry.SpanStatusUnauthenticated
	case status == http.StatusForbidden:
		return sentry.SpanStatusPermissionDenied
	case status == http.StatusNotFound:
		return sentry.SpanStatusNotFound
	case status == http.StatusTooManyRequests:
		return sentry.SpanStatusResourceExhausted
	case status >= 400 && status < 500:
		return sentry.SpanStatusInvalidArgument
	case status == http.StatusNotImplemented:
		return sentry.SpanStatusUnimplemented
	case status == http.StatusServiceUnavailable:
		return sentry.SpanStatusUnavailable
	case status == http.StatusGatewayTimeout:
		return sentry.SpanStatusDeadlineExceeded
	case status >= 500:
		return sentry.SpanStatusInternalError
	default:
		return sentry.SpanStatusUnknown
	}
}

// StartSpan opens a child span on whatever transaction ctx is carrying, and
// returns the function that ends it.
//
// This is the half that makes a transaction worth having. A transaction saying
// "this took 22 seconds" only restates what the request log already said; the
// spans inside it are what say which query, how many times.
//
// Safe on a context with no transaction — the returned function is then a
// no-op, so a call site does not need to know whether tracing is on:
//
//	end := observe.StartSpan(ctx, "db.query", "GetBlocks")
//	defer end()
func StartSpan(ctx context.Context, operation, description string) func() {
	if !sentryEnabled {
		return func() {}
	}
	if sentry.SpanFromContext(ctx) == nil {
		// No transaction to hang this on. Starting one here would produce an
		// orphan trace of a single span, which reads in Sentry as a request
		// that did nothing else — worse than no data.
		return func() {}
	}
	span := sentry.StartSpan(ctx, operation,
		sentry.WithDescription(description),
		sentry.WithSpanOrigin(spanOriginBowrain),
	)
	return span.Finish
}

// SpanContext is StartSpan for a call that needs to pass the span's context on,
// so work it starts nests inside rather than beside it.
func SpanContext(ctx context.Context, operation, description string) (context.Context, func()) {
	if !sentryEnabled || sentry.SpanFromContext(ctx) == nil {
		return ctx, func() {}
	}
	span := sentry.StartSpan(ctx, operation,
		sentry.WithDescription(description),
		sentry.WithSpanOrigin(spanOriginBowrain),
	)
	return span.Context(), span.Finish
}
