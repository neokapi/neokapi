package observe

import (
	"context"
	"errors"

	sentry "github.com/getsentry/sentry-go"
)

// Transaction opens a root transaction for a unit of work that is not an HTTP
// request — a gRPC call, a queued job, an MCP method.
//
// The HTTP middleware cannot serve these: it needs an echo.Context, and more to
// the point a request is only one of the ways this system does slow things. The
// worker carries the same SENTRY_TRACES_SAMPLE_RATE as the server and, until
// this existed, produced nothing at all — a convergence run that took an hour
// was as invisible as the 22-second dashboard that prompted the HTTP half.
//
// name is what Sentry aggregates on, so it must be the SHAPE of the work — the
// queue, the RPC method — and never carry a job id, project id or workspace.
// The identifying handle goes on as a tag instead, where high cardinality is
// what tags are for.
//
// The returned function ends the transaction and records whether the work
// failed. Call it with the work's error, or nil:
//
//	ctx, end := observe.Transaction(ctx, "queue.task", "translation")
//	err := process(ctx)
//	end(err)
//
// It is a no-op when Sentry is unconfigured, and returns ctx unchanged, so a
// call site never needs to know whether tracing is on.
func Transaction(ctx context.Context, op, name string) (context.Context, func(error)) {
	if !sentryEnabled {
		return ctx, func(error) {}
	}
	transaction := sentry.StartTransaction(ctx, name,
		sentry.WithOpName(op),
		// SourceTask: the name is one this code chose, not a URL a caller
		// supplied. Sentry uses the distinction to decide what it may safely
		// group, and mislabelling a chosen name as a URL invites it to try to
		// parameterize something that is already parameterized.
		sentry.WithTransactionSource(sentry.SourceTask),
		sentry.WithSpanOrigin(spanOriginBowrain),
	)
	// The correlation id the surface already uses — the job id for a job, the
	// request id for an RPC — so a trace and its log lines join on the handle a
	// person actually has.
	if ref, ok := RequestIDFromContext(ctx); ok && ref != "" {
		transaction.SetTag("request_id", ref)
		transaction.SetTag("reference", ref)
	}
	return transaction.Context(), func(err error) {
		transaction.Status = workStatus(err)
		transaction.Finish()
	}
}

// TagTransaction adds a tag to whatever transaction ctx is carrying.
//
// For the identifying detail a name must not hold: which workspace, which
// project, which tool. Tags are indexed and may be high-cardinality; a
// transaction NAME may not.
func TagTransaction(ctx context.Context, key, value string) {
	if !sentryEnabled || value == "" {
		return
	}
	if t := sentry.TransactionFromContext(ctx); t != nil {
		t.SetTag(key, value)
	}
}

// workStatus maps a unit of work's error onto a span status.
//
// A cancelled context is separated from a failure on purpose: shutting the
// worker down mid-job is an ordinary event, and filing it as an internal error
// would put a spike in the failure rate every time the service is deployed.
func workStatus(err error) sentry.SpanStatus {
	switch {
	case err == nil:
		return sentry.SpanStatusOK
	case errors.Is(err, context.Canceled):
		return sentry.SpanStatusCanceled
	case errors.Is(err, context.DeadlineExceeded):
		return sentry.SpanStatusDeadlineExceeded
	default:
		return sentry.SpanStatusInternalError
	}
}
