package observe

import (
	"context"
	"errors"
	"testing"

	sentry "github.com/getsentry/sentry-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// A unit of work that is not a request still produces a transaction.
//
// The worker carries the same traces sample rate as the server and, before
// this, started nothing: a convergence run that took an hour was as invisible
// as the 22-second dashboard that prompted instrumenting HTTP.
func TestTransaction_NamesTheShapeAndTagsTheInstance(t *testing.T) {
	tr := capture(t)

	ctx := WithRequestID(context.Background(), "job_7f31c9")
	ctx, end := Transaction(ctx, "queue.task", "translation")
	assert.NotNil(t, sentry.TransactionFromContext(ctx), "the work runs inside its transaction")
	end(nil)

	txns := tr.transactions()
	require.Len(t, txns, 1)
	assert.Equal(t, "translation", txns[0].Transaction, "named by queue, so it aggregates")
	assert.Equal(t, "job_7f31c9", txns[0].Tags["request_id"], "the job id is a tag, not the name")
	assert.Equal(t, "job_7f31c9", txns[0].Tags["reference"])
	assert.Equal(t, sentry.SpanStatusOK, txns[0].Contexts["trace"]["status"])
}

// Shutting the worker down mid-job is an ordinary event, not a failure.
//
// Filing a cancelled job as an internal error puts a spike in the failure rate
// on every deploy, which is how a useful signal becomes one people mute.
func TestTransaction_SeparatesCancellationFromFailure(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want sentry.SpanStatus
	}{
		{"done", nil, sentry.SpanStatusOK},
		{"shutdown", context.Canceled, sentry.SpanStatusCanceled},
		{"timed out", context.DeadlineExceeded, sentry.SpanStatusDeadlineExceeded},
		{"failed", errors.New("boom"), sentry.SpanStatusInternalError},
		{"wrapped shutdown", errWrap{context.Canceled}, sentry.SpanStatusCanceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tr := capture(t)
			_, end := Transaction(context.Background(), "queue.task", "translation")
			end(tc.err)

			txns := tr.transactions()
			require.Len(t, txns, 1)
			assert.Equal(t, tc.want, txns[0].Contexts["trace"]["status"])
		})
	}
}

type errWrap struct{ inner error }

func (e errWrap) Error() string { return "wrapped: " + e.inner.Error() }
func (e errWrap) Unwrap() error { return e.inner }

// Spans opened by the work attach to its transaction, the same as under HTTP —
// which is what makes a slow job attributable to a query rather than to the job.
func TestTransaction_CarriesItsSpans(t *testing.T) {
	tr := capture(t)

	ctx, end := Transaction(context.Background(), "queue.task", "translation")
	spanEnd := StartSpan(ctx, "db.query", "store.GetBlocks whole-stream")
	spanEnd()
	end(nil)

	txns := tr.transactions()
	require.Len(t, txns, 1)
	require.Len(t, txns[0].Spans, 1)
	assert.Equal(t, "store.GetBlocks whole-stream", txns[0].Spans[0].Description)
}

// A gRPC call is named by its method, which is already the shape.
func TestGRPCTracing_UnaryNamesTheMethod(t *testing.T) {
	tr := capture(t)

	interceptor := GRPCTracingUnaryInterceptor()
	_, err := interceptor(context.Background(), nil,
		&grpc.UnaryServerInfo{FullMethod: "/neokapi.v1.NeokapiService/PushContent"},
		func(ctx context.Context, _ any) (any, error) {
			assert.NotNil(t, sentry.TransactionFromContext(ctx), "the handler runs inside the transaction")
			return nil, nil
		})
	require.NoError(t, err)

	txns := tr.transactions()
	require.Len(t, txns, 1)
	assert.Equal(t, "/neokapi.v1.NeokapiService/PushContent", txns[0].Transaction)
	assert.Equal(t, "OK", txns[0].Tags["rpc.grpc.status_code"])
}

// A gRPC status is recorded as itself. Refusals, cancellations and failures are
// three different things and only one of them means the service is broken.
func TestGRPCTracing_RecordsTheStatus(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want sentry.SpanStatus
	}{
		{"ok", nil, sentry.SpanStatusOK},
		{"refused", status.Error(codes.PermissionDenied, "no"), sentry.SpanStatusPermissionDenied},
		{"client hung up", status.Error(codes.Canceled, "gone"), sentry.SpanStatusCanceled},
		{"exhausted", status.Error(codes.ResourceExhausted, "slow down"), sentry.SpanStatusResourceExhausted},
		{"failed", status.Error(codes.Internal, "boom"), sentry.SpanStatusInternalError},
		{"plain error", errors.New("boom"), sentry.SpanStatusInternalError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tr := capture(t)
			interceptor := GRPCTracingUnaryInterceptor()
			_, _ = interceptor(context.Background(), nil,
				&grpc.UnaryServerInfo{FullMethod: "/svc/M"},
				func(context.Context, any) (any, error) { return nil, tc.err })

			txns := tr.transactions()
			require.Len(t, txns, 1)
			assert.Equal(t, tc.want, txns[0].Contexts["trace"]["status"])
		})
	}
}

// A streaming RPC's handler receives the transaction through the stream.
//
// grpc.ServerStream exposes its context through a method, so the only way to
// reach a stream handler is to wrap the stream — without it a stream's spans
// have no transaction and are dropped.
func TestGRPCTracing_StreamHandlerSeesTheTransaction(t *testing.T) {
	tr := capture(t)

	interceptor := GRPCTracingStreamInterceptor()
	err := interceptor(nil, fakeStream{ctx: context.Background()},
		&grpc.StreamServerInfo{FullMethod: "/svc/Stream"},
		func(_ any, ss grpc.ServerStream) error {
			require.NotNil(t, sentry.TransactionFromContext(ss.Context()),
				"the stream handler must see the transaction")
			end := StartSpan(ss.Context(), "db.query", "store.GetBlocks whole-stream")
			end()
			return nil
		})
	require.NoError(t, err)

	txns := tr.transactions()
	require.Len(t, txns, 1)
	assert.Equal(t, "/svc/Stream", txns[0].Transaction)
	require.Len(t, txns[0].Spans, 1, "a span opened in the stream handler must attach")
}

type fakeStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (f fakeStream) Context() context.Context { return f.ctx }

// With Sentry unconfigured, the non-HTTP surfaces behave as they did before:
// the work runs, nothing is started, nothing is sent.
func TestTransaction_NoOpWhenSentryDisabled(t *testing.T) {
	tr := capture(t)
	sentryEnabled = false

	ctx, end := Transaction(context.Background(), "queue.task", "translation")
	assert.Nil(t, sentry.TransactionFromContext(ctx))
	end(errors.New("boom"))

	ran := false
	interceptor := GRPCTracingUnaryInterceptor()
	_, _ = interceptor(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/svc/M"},
		func(context.Context, any) (any, error) { ran = true; return nil, nil })

	assert.True(t, ran, "the RPC is served either way")
	assert.Empty(t, tr.transactions())
}

// TagTransaction puts high-cardinality detail where it belongs: on a tag, never
// in the name Sentry groups by.
func TestTagTransaction_AddsToTheCurrentTransaction(t *testing.T) {
	tr := capture(t)

	ctx, end := Transaction(context.Background(), "queue.task", "translation")
	TagTransaction(ctx, "mcp.tool", "list_blocks")
	TagTransaction(ctx, "mcp.tool", "")                // empty is ignored
	TagTransaction(context.Background(), "stray", "x") // no transaction: no-op
	end(nil)

	txns := tr.transactions()
	require.Len(t, txns, 1)
	assert.Equal(t, "list_blocks", txns[0].Tags["mcp.tool"])
	assert.Equal(t, "translation", txns[0].Transaction, "the name is untouched")
}
