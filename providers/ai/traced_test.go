package aiprovider

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/observe"
)

type fakeTracer struct{ spans []*fakeSpan }

func (f *fakeTracer) Start(ctx context.Context, op, name string) (context.Context, observe.Span) {
	s := &fakeSpan{op: op, name: name, attrs: map[string]string{}}
	f.spans = append(f.spans, s)
	return ctx, s
}

type fakeSpan struct {
	op, name string
	attrs    map[string]string
	ended    bool
	err      error
}

func (s *fakeSpan) SetAttr(k, v string) { s.attrs[k] = v }
func (s *fakeSpan) End(err error)       { s.ended, s.err = true, err }

// traceCtx scopes a tracer to the call rather than registering one, so these
// tests leave no process state behind.
func traceCtx(t *testing.T) (context.Context, *fakeTracer) {
	t.Helper()
	tr := &fakeTracer{}
	return observe.WithTracer(context.Background(), tr), tr
}

// stubProvider answers with whatever the test set.
type stubProvider struct {
	chat *ChatResponse
	tr   *TranslateResponse
	err  error
}

func (s stubProvider) Name() ProviderID            { return ProviderID("anthropic") }
func (s stubProvider) InputModalities() []Modality { return nil }
func (s stubProvider) Close() error                { return nil }
func (s stubProvider) Translate(context.Context, TranslateRequest) (*TranslateResponse, error) {
	return s.tr, s.err
}
func (s stubProvider) Chat(context.Context, []Message) (*ChatResponse, error) {
	return s.chat, s.err
}
func (s stubProvider) ChatStructured(context.Context, []Message, JSONSchema) (*ChatResponse, error) {
	return s.chat, s.err
}

// A model call reports what it cost, not just how long it took.
//
// Latency says the call was slow; tokens say what it cost, and only one of
// those turns up on an invoice.
func TestTraced_ChatRecordsCostAndModel(t *testing.T) {
	ctx, tr := traceCtx(t)

	p := Traced(stubProvider{chat: &ChatResponse{
		Model: "claude-opus-5",
		Usage: TokenUsage{InputTokens: 1200, OutputTokens: 340, CacheReadTokens: 900},
	}})
	_, err := p.Chat(ctx, []Message{TextMessage(RoleUser, "hi")})
	require.NoError(t, err)

	require.Len(t, tr.spans, 1)
	s := tr.spans[0]
	assert.Equal(t, "gen_ai.call", s.op)
	assert.Equal(t, "anthropic chat", s.name, "provider and operation, no ids — the name has to aggregate")
	assert.Equal(t, "anthropic", s.attrs["gen_ai.system"])
	assert.Equal(t, "claude-opus-5", s.attrs["gen_ai.response.model"])
	assert.Equal(t, "1200", s.attrs["gen_ai.usage.input_tokens"])
	assert.Equal(t, "340", s.attrs["gen_ai.usage.output_tokens"])
	assert.Equal(t, "900", s.attrs["gen_ai.usage.cache_read_tokens"])
	assert.True(t, s.ended)
}

// A capped response is a distinct failure from a slow one, and otherwise
// invisible: the call succeeded, the answer is just a fragment.
func TestTraced_RecordsTruncation(t *testing.T) {
	ctx, tr := traceCtx(t)

	p := Traced(stubProvider{chat: &ChatResponse{Model: "m", Truncated: true}})
	_, err := p.Chat(ctx, nil)
	require.NoError(t, err)

	require.Len(t, tr.spans, 1)
	assert.Equal(t, "true", tr.spans[0].attrs["gen_ai.response.truncated"])
}

// A failed call still ends its span, carrying the error.
func TestTraced_FailureEndsTheSpanWithTheError(t *testing.T) {
	ctx, tr := traceCtx(t)

	boom := errors.New("upstream refused")
	p := Traced(stubProvider{err: boom})
	_, err := p.Chat(ctx, nil)

	require.ErrorIs(t, err, boom)
	require.Len(t, tr.spans, 1)
	assert.True(t, tr.spans[0].ended)
	assert.Equal(t, boom, tr.spans[0].err)
}

// A provider that returns neither response nor error must not take the process
// down on the way to being measured.
func TestTraced_NilResponseIsSurvivable(t *testing.T) {
	ctx, tr := traceCtx(t)

	p := Traced(stubProvider{})
	assert.NotPanics(t, func() { _, _ = p.Chat(ctx, nil) })
	require.Len(t, tr.spans, 1)
	assert.True(t, tr.spans[0].ended)
}

// Each entry point is named for the work it does, so a slow structured call is
// distinguishable from a slow chat.
func TestTraced_NamesEachOperation(t *testing.T) {
	ctx, tr := traceCtx(t)
	p := Traced(stubProvider{chat: &ChatResponse{}, tr: &TranslateResponse{}})

	_, _ = p.Chat(ctx, nil)
	_, _ = p.ChatStructured(ctx, nil, JSONSchema{Name: "s"})
	_, _ = p.Translate(ctx, TranslateRequest{})

	require.Len(t, tr.spans, 3)
	assert.Equal(t, "anthropic chat", tr.spans[0].name)
	assert.Equal(t, "anthropic chat_structured", tr.spans[1].name)
	assert.Equal(t, "anthropic translate", tr.spans[2].name)
}

// Wrapping is transparent: the decorator is still the provider it wrapped.
func TestTraced_PassesThroughIdentity(t *testing.T) {
	p := Traced(stubProvider{})
	assert.Equal(t, ProviderID("anthropic"), p.Name())
	require.NoError(t, p.Close())
	assert.Nil(t, Traced(nil), "nothing to wrap is nothing, not a wrapper around nil")
}
