package service

import (
	"context"

	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// meteredProvider wraps the provider a run's step was granted so every call it
// serves lands in the run's accumulator.
//
// A decorator rather than a hook in each tool: the platform grants one provider
// per step and there are eight model-backed tools, so measuring at the provider
// counts every call each of them makes, including the ones a tool makes outside
// its own usage accumulator.
func meteredProvider(inner aiprovider.LLMProvider, run *AIRun, operation string) aiprovider.LLMProvider {
	m := &meteringProvider{inner: inner, run: run, operation: operation}
	// Preserve streaming when the granted provider offers it: the translate
	// tool type-asserts for StreamingLLMProvider to surface live thinking
	// progress, and a wrapper that dropped it would turn that off with no
	// symptom but a progress view that stopped moving.
	if s, ok := inner.(aiprovider.StreamingLLMProvider); ok {
		return &meteringStreamingProvider{meteringProvider: m, stream: s}
	}
	return m
}

type meteringProvider struct {
	inner     aiprovider.LLMProvider
	run       *AIRun
	operation string
}

// Unwrap exposes the wrapped provider, so a caller looking for something
// further down the stack can see through this wrapper.
func (m *meteringProvider) Unwrap() aiprovider.LLMProvider { return m.inner }

func (m *meteringProvider) Name() aiprovider.ProviderID            { return m.inner.Name() }
func (m *meteringProvider) InputModalities() []aiprovider.Modality { return m.inner.InputModalities() }
func (m *meteringProvider) Close() error                           { return m.inner.Close() }

// record folds one call's usage into the run. A response that names no model
// is recorded under the empty model rather than dropped, because the tokens
// were spent whatever the provider chose to report.
func (m *meteringProvider) record(model string, usage aiprovider.TokenUsage) {
	m.run.add(m.operation, model, usage)
}

func (m *meteringProvider) Translate(ctx context.Context, req aiprovider.TranslateRequest) (*aiprovider.TranslateResponse, error) {
	resp, err := m.inner.Translate(ctx, req)
	if resp != nil {
		m.record(resp.Model, resp.Usage)
	}
	return resp, err
}

func (m *meteringProvider) Chat(ctx context.Context, messages []aiprovider.Message) (*aiprovider.ChatResponse, error) {
	resp, err := m.inner.Chat(ctx, messages)
	if resp != nil {
		m.record(resp.Model, resp.Usage)
	}
	return resp, err
}

func (m *meteringProvider) ChatStructured(ctx context.Context, messages []aiprovider.Message, schema aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
	resp, err := m.inner.ChatStructured(ctx, messages, schema)
	if resp != nil {
		m.record(resp.Model, resp.Usage)
	}
	return resp, err
}

// meteringStreamingProvider is meteringProvider that is still a
// StreamingLLMProvider, so a type assertion for streaming keeps succeeding
// through the wrapper.
type meteringStreamingProvider struct {
	*meteringProvider
	stream aiprovider.StreamingLLMProvider
}

func (m *meteringStreamingProvider) ChatStream(ctx context.Context, messages []aiprovider.Message, onEvent func(aiprovider.ChatStreamEvent)) (*aiprovider.ChatResponse, error) {
	resp, err := m.stream.ChatStream(ctx, messages, onEvent)
	if resp != nil {
		m.record(resp.Model, resp.Usage)
	}
	return resp, err
}

func (m *meteringStreamingProvider) ChatStructuredStream(ctx context.Context, messages []aiprovider.Message, schema aiprovider.JSONSchema, onEvent func(aiprovider.ChatStreamEvent)) (*aiprovider.ChatResponse, error) {
	resp, err := m.stream.ChatStructuredStream(ctx, messages, schema, onEvent)
	if resp != nil {
		m.record(resp.Model, resp.Usage)
	}
	return resp, err
}
