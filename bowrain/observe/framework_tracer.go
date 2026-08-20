package observe

import (
	"context"

	sentry "github.com/getsentry/sentry-go"

	coreobserve "github.com/neokapi/neokapi/core/observe"
)

// The framework measures its own work; this is what makes the measurements
// arrive somewhere.
//
// core/observe is the seam: the framework is Apache-2.0 and cannot import this
// package, so it opens spans through an interface it owns and something below
// supplies the implementation. This is that implementation for the platform.
// kapi and kapi-desktop install nothing and the same framework code runs
// untraced. Nothing about the arrangement is a licence exception — the shared
// type lives on the framework side, which is where a type both sides need
// belongs.

// InstallFrameworkTracer routes the framework's spans into Sentry.
//
// Call once at startup, after Sentry is initialised. Calling it when Sentry is
// not configured is harmless: every span it produces then short-circuits.
func InstallFrameworkTracer() {
	coreobserve.Register(frameworkTracer{})
}

type frameworkTracer struct{}

// Start opens a child span on whatever transaction the context is carrying.
//
// A framework span is always a child, never a root. The unit of work a person
// asks about is the request or the job — "the translate call took 8 seconds" is
// an answer to "why was that job slow", and standing alone it is a trace of one
// span describing a request that apparently did nothing else.
func (frameworkTracer) Start(ctx context.Context, op, name string) (context.Context, coreobserve.Span) {
	if !sentryEnabled || sentry.SpanFromContext(ctx) == nil {
		return ctx, nopSpan{}
	}
	span := sentry.StartSpan(ctx, op,
		sentry.WithDescription(name),
		sentry.WithSpanOrigin(spanOriginBowrain),
	)
	return span.Context(), &sentrySpan{span: span}
}

type sentrySpan struct{ span *sentry.Span }

// SetAttr writes a tag, which is the indexed and filterable half of a span —
// the same reason Scope puts its dimensions on tags rather than in the name.
func (s *sentrySpan) SetAttr(key, value string) { s.span.SetTag(key, value) }

// End records the outcome and closes the span. Cancellation is not a failure,
// for the reason workStatus gives.
func (s *sentrySpan) End(err error) {
	s.span.Status = workStatus(err)
	s.span.Finish()
}

// nopSpan is returned when there is no transaction to attach to.
type nopSpan struct{}

func (nopSpan) SetAttr(string, string) {}
func (nopSpan) End(error)              {}
