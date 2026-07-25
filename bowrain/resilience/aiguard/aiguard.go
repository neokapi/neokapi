// Package aiguard puts every LLM call behind a per-provider circuit breaker.
//
// It guards at the [aiprovider.LLMProvider] interface rather than inside each
// provider, which buys two things. Coverage is total and automatic — Bedrock,
// Anthropic, OpenAI, Azure, Gemini, Ollama and any plugin-supplied provider are
// all covered by the same wrapper, including ones added later. And the guard
// sits *outside* the providers' HTTP retry transport, so one logical
// translation counts once against the breaker instead of once per retry.
//
// Breakers are keyed by provider id, so an outage of one model host never
// short-circuits a workspace that brought its own key for a different one.
package aiguard

import (
	"context"

	"github.com/neokapi/neokapi/bowrain/resilience"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// BreakerName is the registry name for an AI provider's breaker. Exported so
// health reporting and tests can refer to the same string the guard uses.
func BreakerName(id aiprovider.ProviderID) string { return "ai:" + string(id) }

// NewProvider is [aiprovider.NewProvider] with the guard already applied. It is
// the constructor every Bowrain call site should use: guarding at construction
// means no code path can acquire an unguarded provider by forgetting a wrapper.
func NewProvider(id aiprovider.ProviderID, cfg aiprovider.Config) (aiprovider.LLMProvider, error) {
	prov, err := aiprovider.NewProvider(id, cfg)
	if err != nil {
		return nil, err
	}
	return Wrap(prov), nil
}

// Wrap returns prov guarded by the process-wide breaker for its provider id.
// A nil provider passes through as nil so callers can wrap unconditionally.
//
// The wrapper is transparent: on a closed circuit it returns exactly what the
// provider returned. On an open circuit it returns a
// [resilience.UnavailableError] without calling the provider at all — no
// request is sent, no token is spent, and the caller can tell the difference
// between "the model refused" and "we never asked".
func Wrap(prov aiprovider.LLMProvider) aiprovider.LLMProvider {
	return WrapWith(prov, nil)
}

// WrapWith is [Wrap] against an explicit registry, for tests.
func WrapWith(prov aiprovider.LLMProvider, reg *resilience.Registry) aiprovider.LLMProvider {
	if prov == nil {
		return nil
	}
	// Never double-wrap: the provider factories are called from several layers
	// and a stack of guards would count one call many times.
	if g, ok := prov.(*guarded); ok {
		return g
	}
	if reg == nil {
		reg = resilience.Default()
	}
	b := reg.Get(BreakerName(prov.Name()), resilience.KindAI)
	g := &guarded{LLMProvider: prov, breaker: b}

	// Preserve streaming when the underlying provider offers it; a guard that
	// silently downgraded a streaming provider would disable live progress.
	if s, ok := prov.(aiprovider.StreamingLLMProvider); ok {
		return &guardedStreaming{guarded: g, stream: s}
	}
	return g
}

// Breaker returns the breaker guarding a provider, or nil when prov is not
// guarded. Call sites use it to ask "is this dependency currently down?"
// before committing to expensive setup.
func Breaker(prov aiprovider.LLMProvider) *resilience.Breaker {
	switch g := prov.(type) {
	case *guardedStreaming:
		return g.breaker
	case *guarded:
		return g.breaker
	}
	return nil
}

// Available reports whether prov's circuit is closed enough to attempt a call.
// An unguarded provider is always available.
func Available(prov aiprovider.LLMProvider) bool {
	b := Breaker(prov)
	return b == nil || b.State() != resilience.StateOpen
}

type guarded struct {
	aiprovider.LLMProvider
	breaker *resilience.Breaker
}

func (g *guarded) Translate(ctx context.Context, req aiprovider.TranslateRequest) (*aiprovider.TranslateResponse, error) {
	return resilience.Do(ctx, g.breaker, func(ctx context.Context) (*aiprovider.TranslateResponse, error) {
		return g.LLMProvider.Translate(ctx, req)
	})
}

func (g *guarded) Chat(ctx context.Context, messages []aiprovider.Message) (*aiprovider.ChatResponse, error) {
	return resilience.Do(ctx, g.breaker, func(ctx context.Context) (*aiprovider.ChatResponse, error) {
		return g.LLMProvider.Chat(ctx, messages)
	})
}

func (g *guarded) ChatStructured(ctx context.Context, messages []aiprovider.Message, schema aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
	return resilience.Do(ctx, g.breaker, func(ctx context.Context) (*aiprovider.ChatResponse, error) {
		return g.LLMProvider.ChatStructured(ctx, messages, schema)
	})
}

type guardedStreaming struct {
	*guarded
	stream aiprovider.StreamingLLMProvider
}

func (g *guardedStreaming) ChatStream(ctx context.Context, messages []aiprovider.Message, onEvent func(aiprovider.ChatStreamEvent)) (*aiprovider.ChatResponse, error) {
	return resilience.Do(ctx, g.breaker, func(ctx context.Context) (*aiprovider.ChatResponse, error) {
		return g.stream.ChatStream(ctx, messages, onEvent)
	})
}

func (g *guardedStreaming) ChatStructuredStream(ctx context.Context, messages []aiprovider.Message, schema aiprovider.JSONSchema, onEvent func(aiprovider.ChatStreamEvent)) (*aiprovider.ChatResponse, error) {
	return resilience.Do(ctx, g.breaker, func(ctx context.Context) (*aiprovider.ChatResponse, error) {
		return g.stream.ChatStructuredStream(ctx, messages, schema, onEvent)
	})
}

// Compile-time proof that the guards keep the interfaces they claim.
var (
	_ aiprovider.LLMProvider          = (*guarded)(nil)
	_ aiprovider.StreamingLLMProvider = (*guardedStreaming)(nil)
)
