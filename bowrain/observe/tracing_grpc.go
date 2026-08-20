package observe

import (
	"context"

	sentry "github.com/getsentry/sentry-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GRPCTracingUnaryInterceptor opens a transaction per unary RPC.
//
// The gRPC surface carries the CLI's pushes and flow executions — the calls
// most likely to be slow, and the ones least likely to be watched, since no
// browser is waiting on them. It shares the server process and therefore the
// traces sample rate, and produced nothing until this existed.
//
// Chain it OUTSIDE the auth interceptors, so a call rejected for its
// credentials is still a transaction: "authentication is slow" and
// "authentication refuses everything" are both things worth being able to see.
func GRPCTracingUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if !sentryEnabled {
			return handler(ctx, req)
		}
		// info.FullMethod is already the shape — /neokapi.v1.NeokapiService/PushContent
		// — with no ids in it, so it aggregates as it stands.
		ctx, end := grpcTransaction(ctx, info.FullMethod)
		resp, err := handler(ctx, req)
		end(err)
		return resp, err
	}
}

// GRPCTracingStreamInterceptor opens a transaction per streaming RPC.
//
// A stream's transaction spans the whole stream, not one message: that is the
// unit whose duration means something, and the one that hangs.
func GRPCTracingStreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if !sentryEnabled {
			return handler(srv, ss)
		}
		ctx, end := grpcTransaction(ss.Context(), info.FullMethod)
		err := handler(srv, &tracedServerStream{ServerStream: ss, ctx: ctx})
		end(err)
		return err
	}
}

// grpcTransaction starts the transaction and returns the finisher, which maps a
// gRPC error onto a span status.
func grpcTransaction(ctx context.Context, fullMethod string) (context.Context, func(error)) {
	transaction := sentry.StartTransaction(ctx, fullMethod,
		sentry.WithOpName("rpc.server"),
		sentry.WithTransactionSource(sentry.SourceRoute),
		sentry.WithSpanOrigin(spanOriginBowrain),
	)
	transaction.SetTag("rpc.method", fullMethod)
	if ref, ok := RequestIDFromContext(ctx); ok && ref != "" {
		transaction.SetTag("request_id", ref)
		transaction.SetTag("reference", ref)
	}
	return transaction.Context(), func(err error) {
		code := status.Code(err)
		transaction.SetTag("rpc.grpc.status_code", code.String())
		transaction.Status = grpcSpanStatus(code)
		transaction.Finish()
	}
}

// tracedServerStream carries the transaction's context to the handler.
//
// grpc.ServerStream exposes its context through a method rather than a field,
// so the only way to hand a stream handler the transaction is to wrap the
// stream. Without this a stream's inner spans have no transaction to attach to
// and are dropped.
type tracedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *tracedServerStream) Context() context.Context { return s.ctx }

// grpcSpanStatus maps a gRPC code onto a span status.
//
// The codes line up almost exactly — Sentry's span statuses were taken from
// gRPC's — so this is a rename rather than a judgement, with one exception
// worth stating: Canceled stays Canceled rather than becoming an error,
// because a client that hung up is not this service failing.
func grpcSpanStatus(code codes.Code) sentry.SpanStatus {
	switch code {
	case codes.OK:
		return sentry.SpanStatusOK
	case codes.Canceled:
		return sentry.SpanStatusCanceled
	case codes.InvalidArgument:
		return sentry.SpanStatusInvalidArgument
	case codes.DeadlineExceeded:
		return sentry.SpanStatusDeadlineExceeded
	case codes.NotFound:
		return sentry.SpanStatusNotFound
	case codes.AlreadyExists:
		return sentry.SpanStatusAlreadyExists
	case codes.PermissionDenied:
		return sentry.SpanStatusPermissionDenied
	case codes.ResourceExhausted:
		return sentry.SpanStatusResourceExhausted
	case codes.FailedPrecondition:
		return sentry.SpanStatusFailedPrecondition
	case codes.Aborted:
		return sentry.SpanStatusAborted
	case codes.OutOfRange:
		return sentry.SpanStatusOutOfRange
	case codes.Unimplemented:
		return sentry.SpanStatusUnimplemented
	case codes.Unavailable:
		return sentry.SpanStatusUnavailable
	case codes.DataLoss:
		return sentry.SpanStatusDataLoss
	case codes.Unauthenticated:
		return sentry.SpanStatusUnauthenticated
	default:
		return sentry.SpanStatusInternalError
	}
}
