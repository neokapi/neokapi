// Package observe carries spans out of the framework without knowing where they
// go.
//
// The framework is Apache-2.0 and must not import bowrain, which is AGPL, so it
// cannot call the platform's Sentry instrumentation directly — `make
// audit-modules` fails the build if it tries. The type both sides need
// therefore lives here, below the licence line: the framework opens spans
// through an interface it owns, and whoever is hosting it supplies the
// implementation. Bowrain registers a Sentry-backed tracer; kapi and
// kapi-desktop register nothing and pay for nothing.
//
// Nothing here depends on OpenTelemetry, Sentry, or any other backend. That is
// the point — this interface is public framework API, and a dependency added
// here is one every consumer of the framework inherits.
package observe

import (
	"context"
	"sync/atomic"
)

// Tracer opens spans. One implementation per backend; register it once at
// startup.
type Tracer interface {
	Start(ctx context.Context, op, name string) (context.Context, Span)
}

// Span is one measured unit of work.
//
// Deliberately small. Every method here is one every backend must be able to
// answer, and an interface that grows to fit one backend's features stops
// being implementable by the next.
type Span interface {
	// SetAttr records a dimension to filter by. String-valued because the
	// backends disagree about everything else, and a number rendered as a
	// string is still filterable.
	SetAttr(key, value string)
	// End closes the span. A nil error is success; anything else is recorded
	// as the failure it is.
	End(err error)
}

// registered holds the process tracer. atomic.Value rather than a mutex
// because this is read on every span and written once at startup.
var registered atomic.Value

// Register installs the process-wide tracer.
//
// Call it once, before serving. Registering nothing is the supported default:
// the framework runs untraced and Start costs an interface dispatch returning
// a zero-size value.
func Register(t Tracer) {
	if t == nil {
		registered = atomic.Value{}
		return
	}
	registered.Store(tracerHolder{t})
}

// tracerHolder makes the stored type consistent, since atomic.Value panics if
// successive Stores disagree about the concrete type — which they would, given
// callers store whatever implements Tracer.
type tracerHolder struct{ Tracer }

type tracerKey struct{}

// WithTracer scopes a tracer to one context, overriding the registered one.
//
// For a host serving several tenants with separate destinations, and for tests
// that must not disturb process state.
func WithTracer(ctx context.Context, t Tracer) context.Context {
	return context.WithValue(ctx, tracerKey{}, t)
}

// tracerFrom resolves the tracer for this call: context first, then the
// process-wide one, then nothing.
func tracerFrom(ctx context.Context) Tracer {
	if t, ok := ctx.Value(tracerKey{}).(Tracer); ok && t != nil {
		return t
	}
	if h, ok := registered.Load().(tracerHolder); ok {
		return h.Tracer
	}
	return nil
}

// Start opens a span for a unit of work.
//
// op is the kind of work, shared across every span of that kind so a backend
// can aggregate them — "gen_ai.call", "flow.tool", "format.read". name
// identifies the particular work within that kind and must not carry ids: a
// name built from a workspace or document id mints one entry per caller and
// aggregates nothing.
//
// The returned context carries the span, so work nested below it attaches
// rather than starting its own root.
//
// Span an operation, not the items inside it. A span per block in a format
// writer would cost more than it measures.
func Start(ctx context.Context, op, name string) (context.Context, Span) {
	t := tracerFrom(ctx)
	if t == nil {
		return ctx, nopSpan{}
	}
	return t.Start(ctx, op, name)
}

// nopSpan is what an unconfigured framework returns. Zero-size, so converting
// it to the Span interface allocates nothing.
type nopSpan struct{}

func (nopSpan) SetAttr(string, string) {}
func (nopSpan) End(error)              {}
