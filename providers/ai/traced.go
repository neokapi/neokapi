package aiprovider

import (
	"context"
	"strconv"

	"github.com/neokapi/neokapi/core/observe"
)

// Traced wraps a provider so every call it serves opens a span.
//
// A decorator rather than instrumentation inside each provider: there are eight
// of them and the measurement is identical for all eight, so putting it in one
// place is both less code and the only version that cannot drift between
// backends. It is also what lets a caller choose — an embedded kapi run wraps
// nothing and pays nothing.
//
// The span is a no-op unless a tracer is registered (core/observe), so wrapping
// unconditionally is safe.
func Traced(inner LLMProvider) LLMProvider {
	if inner == nil {
		return nil
	}
	t := &tracedProvider{inner: inner}
	// Preserve streaming when the underlying provider offers it. A wrapper that
	// silently downgraded a streaming provider would turn live thinking
	// progress off, and the only symptom would be a UI that stopped updating.
	if s, ok := inner.(StreamingLLMProvider); ok {
		return &tracedStreamingProvider{tracedProvider: t, stream: s}
	}
	return t
}

// Span operation and attribute names follow OpenTelemetry's GenAI semantic
// conventions. Nothing here imports OpenTelemetry — the framework must stay
// dependency-free — but a backend that speaks those conventions gets usable
// data without a translation table, and one that does not loses nothing by the
// names being conventional.
const (
	opGenAICall = "gen_ai.call"

	attrSystem       = "gen_ai.system"
	attrOperation    = "gen_ai.operation.name"
	attrModel        = "gen_ai.response.model"
	attrInputTokens  = "gen_ai.usage.input_tokens"
	attrOutputTokens = "gen_ai.usage.output_tokens"
	attrCacheRead    = "gen_ai.usage.cache_read_tokens"
	attrTruncated    = "gen_ai.response.truncated"
)

type tracedProvider struct{ inner LLMProvider }

// Unwrap exposes the wrapped provider, so a caller that needs to recognise
// something further down the stack — a circuit breaker looking for its own
// guard — can look through this wrapper instead of being blinded by it.
func (t *tracedProvider) Unwrap() LLMProvider { return t.inner }

func (t *tracedProvider) Name() ProviderID            { return t.inner.Name() }
func (t *tracedProvider) InputModalities() []Modality { return t.inner.InputModalities() }
func (t *tracedProvider) Close() error                { return t.inner.Close() }

// start opens the span for one call.
//
// The name is the provider and the operation, never the prompt or a document
// id: those would mint a separate entry per call and aggregate nothing, which
// is the failure that makes a trace list unreadable.
func (t *tracedProvider) start(ctx context.Context, operation string) (context.Context, observe.Span) {
	provider := string(t.inner.Name())
	ctx, span := observe.Start(ctx, opGenAICall, provider+" "+operation)
	span.SetAttr(attrSystem, provider)
	span.SetAttr(attrOperation, operation)
	return ctx, span
}

// recordUsage puts the response's shape on the span.
//
// Distinct from this package's Exchange recorder, which answers "what did we
// send the model" for `--explain`: that fires after the call with no timing and
// carries the full prompt and reply, which is exactly what must not be shipped
// to a telemetry backend. This records shape and cost, never content.
//
// Tokens are the reason to measure a model call at all: latency says the call
// was slow and tokens say what it cost, and only one of those appears on an
// invoice. Truncation is recorded because a capped response is a distinct
// failure from a slow one and is otherwise invisible — the call succeeded.
func recordUsage(span observe.Span, model string, usage TokenUsage, truncated bool) {
	if model != "" {
		span.SetAttr(attrModel, model)
	}
	span.SetAttr(attrInputTokens, strconv.Itoa(usage.InputTokens))
	span.SetAttr(attrOutputTokens, strconv.Itoa(usage.OutputTokens))
	if usage.CacheReadTokens > 0 {
		span.SetAttr(attrCacheRead, strconv.Itoa(usage.CacheReadTokens))
	}
	if truncated {
		span.SetAttr(attrTruncated, "true")
	}
}

func (t *tracedProvider) Translate(ctx context.Context, req TranslateRequest) (resp *TranslateResponse, err error) {
	ctx, span := t.start(ctx, "translate")
	defer func() { span.End(err) }()

	resp, err = t.inner.Translate(ctx, req)
	if resp != nil {
		recordUsage(span, resp.Model, resp.Usage, resp.Truncated)
	}
	return resp, err
}

func (t *tracedProvider) Chat(ctx context.Context, messages []Message) (resp *ChatResponse, err error) {
	ctx, span := t.start(ctx, "chat")
	defer func() { span.End(err) }()

	resp, err = t.inner.Chat(ctx, messages)
	if resp != nil {
		recordUsage(span, resp.Model, resp.Usage, resp.Truncated)
	}
	return resp, err
}

func (t *tracedProvider) ChatStructured(ctx context.Context, messages []Message, schema JSONSchema) (resp *ChatResponse, err error) {
	ctx, span := t.start(ctx, "chat_structured")
	defer func() { span.End(err) }()

	resp, err = t.inner.ChatStructured(ctx, messages, schema)
	if resp != nil {
		recordUsage(span, resp.Model, resp.Usage, resp.Truncated)
	}
	return resp, err
}

// tracedStreamingProvider is tracedProvider that is still a
// StreamingLLMProvider, so a type assertion for streaming keeps succeeding
// through the wrapper.
type tracedStreamingProvider struct {
	*tracedProvider
	stream StreamingLLMProvider
}

func (t *tracedStreamingProvider) ChatStream(ctx context.Context, messages []Message, onEvent func(ChatStreamEvent)) (resp *ChatResponse, err error) {
	ctx, span := t.start(ctx, "chat_stream")
	defer func() { span.End(err) }()

	resp, err = t.stream.ChatStream(ctx, messages, onEvent)
	if resp != nil {
		recordUsage(span, resp.Model, resp.Usage, resp.Truncated)
	}
	return resp, err
}

func (t *tracedStreamingProvider) ChatStructuredStream(ctx context.Context, messages []Message, schema JSONSchema, onEvent func(ChatStreamEvent)) (resp *ChatResponse, err error) {
	ctx, span := t.start(ctx, "chat_structured_stream")
	defer func() { span.End(err) }()

	resp, err = t.stream.ChatStructuredStream(ctx, messages, schema, onEvent)
	if resp != nil {
		recordUsage(span, resp.Model, resp.Usage, resp.Truncated)
	}
	return resp, err
}
