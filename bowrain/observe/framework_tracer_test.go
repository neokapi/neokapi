package observe

import (
	"context"
	"errors"
	"testing"

	sentry "github.com/getsentry/sentry-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	coreobserve "github.com/neokapi/neokapi/core/observe"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// install routes framework spans here for one test and unroutes them after, so
// process state does not leak between tests.
func install(t *testing.T) {
	t.Helper()
	InstallFrameworkTracer()
	t.Cleanup(func() { coreobserve.Register(nil) })
}

// A span opened by the framework arrives in Sentry, inside the transaction the
// work is running under.
//
// This is the whole arrangement in one assertion: the framework is Apache-2.0
// and cannot import this package, so it opened this span through an interface
// it owns and never learned what happened next.
func TestFrameworkTracer_SpanLandsInsideTheTransaction(t *testing.T) {
	tr := capture(t)
	install(t)

	ctx, end := Transaction(context.Background(), "queue.task", "translation")
	_, span := coreobserve.Start(ctx, "gen_ai.call", "anthropic chat")
	span.SetAttr("gen_ai.response.model", "claude-opus-5")
	span.End(nil)
	end(nil)

	txns := tr.transactions()
	require.Len(t, txns, 1)
	require.Len(t, txns[0].Spans, 1, "the span is a child of the job, not a trace of its own")

	s := txns[0].Spans[0]
	assert.Equal(t, "gen_ai.call", s.Op)
	assert.Equal(t, "anthropic chat", s.Description)
	assert.Equal(t, "claude-opus-5", s.Tags["gen_ai.response.model"])
	assert.Equal(t, sentry.SpanStatusOK, s.Status)
}

// A failed call is recorded as failed, and a cancelled one is not.
//
// Shutting the worker down mid-job cancels every call in flight. Filing those
// as errors would put a spike in the failure rate on every deploy, which is how
// a signal becomes one people mute.
func TestFrameworkTracer_StatusSeparatesFailureFromCancellation(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want sentry.SpanStatus
	}{
		{"failure", errors.New("upstream refused"), sentry.SpanStatusInternalError},
		{"cancelled", context.Canceled, sentry.SpanStatusCanceled},
		{"deadline", context.DeadlineExceeded, sentry.SpanStatusDeadlineExceeded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tr := capture(t)
			install(t)

			ctx, end := Transaction(context.Background(), "queue.task", "translation")
			_, span := coreobserve.Start(ctx, "gen_ai.call", "anthropic chat")
			span.End(tc.err)
			end(nil)

			txns := tr.transactions()
			require.Len(t, txns, 1)
			require.Len(t, txns[0].Spans, 1)
			assert.Equal(t, tc.want, txns[0].Spans[0].Status)
		})
	}
}

// Framework work outside any transaction produces nothing rather than an
// orphan, which in Sentry reads as a request that did nothing else.
func TestFrameworkTracer_WithoutATransactionProducesNothing(t *testing.T) {
	tr := capture(t)
	install(t)

	_, span := coreobserve.Start(context.Background(), "gen_ai.call", "anthropic chat")
	span.SetAttr("k", "v")
	span.End(nil)

	assert.Empty(t, tr.transactions())
}

// End to end: a wrapped provider, called under a job, is measured — including
// what it cost.
//
// The framework decorator and this adapter are written on opposite sides of a
// licence boundary and neither imports the other, so the connection between
// them is worth asserting rather than assuming.
func TestFrameworkTracer_MeasuresAWrappedProvider(t *testing.T) {
	tr := capture(t)
	install(t)

	p := aiprovider.Traced(stubProvider{resp: &aiprovider.ChatResponse{
		Model: "claude-opus-5",
		Usage: aiprovider.TokenUsage{InputTokens: 900, OutputTokens: 120},
	}})

	ctx, end := Transaction(context.Background(), "queue.task", "translation")
	_, err := p.Chat(ctx, nil)
	require.NoError(t, err)
	end(nil)

	txns := tr.transactions()
	require.Len(t, txns, 1)
	require.Len(t, txns[0].Spans, 1)

	s := txns[0].Spans[0]
	assert.Equal(t, "gen_ai.call", s.Op)
	assert.Equal(t, "stub chat", s.Description)
	assert.Equal(t, "900", s.Tags["gen_ai.usage.input_tokens"])
	assert.Equal(t, "120", s.Tags["gen_ai.usage.output_tokens"])
}

type stubProvider struct{ resp *aiprovider.ChatResponse }

func (s stubProvider) Name() aiprovider.ProviderID            { return aiprovider.ProviderID("stub") }
func (s stubProvider) InputModalities() []aiprovider.Modality { return nil }
func (s stubProvider) Close() error                           { return nil }
func (s stubProvider) Translate(context.Context, aiprovider.TranslateRequest) (*aiprovider.TranslateResponse, error) {
	return nil, nil
}
func (s stubProvider) Chat(context.Context, []aiprovider.Message) (*aiprovider.ChatResponse, error) {
	return s.resp, nil
}
func (s stubProvider) ChatStructured(context.Context, []aiprovider.Message, aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
	return s.resp, nil
}
