package aiguard

import (
	"context"
	"errors"
	"testing"

	"github.com/neokapi/neokapi/bowrain/resilience"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errProviderDown = errors.New("bedrock: API error 503: service unavailable")

// fakeProvider is a minimal LLMProvider whose every call fails with err (or
// succeeds when err is nil), counting the attempts that actually reached it.
type fakeProvider struct {
	id       aiprovider.ProviderID
	err      error
	attempts int
}

func (p *fakeProvider) Name() aiprovider.ProviderID            { return p.id }
func (p *fakeProvider) InputModalities() []aiprovider.Modality { return nil }
func (p *fakeProvider) Close() error                           { return nil }
func (p *fakeProvider) Translate(context.Context, aiprovider.TranslateRequest) (*aiprovider.TranslateResponse, error) {
	p.attempts++
	if p.err != nil {
		return nil, p.err
	}
	return &aiprovider.TranslateResponse{}, nil
}

func (p *fakeProvider) Chat(context.Context, []aiprovider.Message) (*aiprovider.ChatResponse, error) {
	p.attempts++
	if p.err != nil {
		return nil, p.err
	}
	return &aiprovider.ChatResponse{Content: "ok"}, nil
}

func (p *fakeProvider) ChatStructured(context.Context, []aiprovider.Message, aiprovider.JSONSchema) (*aiprovider.ChatResponse, error) {
	p.attempts++
	if p.err != nil {
		return nil, p.err
	}
	return &aiprovider.ChatResponse{Content: "{}"}, nil
}

// fakeStreamingProvider adds the streaming half so the guard's interface
// preservation can be asserted.
type fakeStreamingProvider struct {
	*fakeProvider
	streamed int
}

func (p *fakeStreamingProvider) ChatStream(_ context.Context, _ []aiprovider.Message, _ func(aiprovider.ChatStreamEvent)) (*aiprovider.ChatResponse, error) {
	p.streamed++
	p.attempts++
	if p.err != nil {
		return nil, p.err
	}
	return &aiprovider.ChatResponse{Content: "streamed"}, nil
}

func (p *fakeStreamingProvider) ChatStructuredStream(_ context.Context, _ []aiprovider.Message, _ aiprovider.JSONSchema, _ func(aiprovider.ChatStreamEvent)) (*aiprovider.ChatResponse, error) {
	p.streamed++
	p.attempts++
	if p.err != nil {
		return nil, p.err
	}
	return &aiprovider.ChatResponse{Content: "{}"}, nil
}

func fastRegistry() *resilience.Registry {
	r := resilience.NewRegistry()
	r.Configure(resilience.Overrides{
		MinimumRequests: new(4),
		FailureRatio:    new(0.5),
		CooldownSeconds: new(60),
	})
	return r
}

func TestGuardStopsCallingADownProvider(t *testing.T) {
	reg := fastRegistry()
	inner := &fakeProvider{id: "bedrock", err: errProviderDown}
	prov := WrapWith(inner, reg)

	msgs := []aiprovider.Message{aiprovider.TextMessage("user", "hi")}
	for range 4 {
		_, err := prov.Chat(t.Context(), msgs)
		require.ErrorIs(t, err, errProviderDown, "before tripping, the provider's own error passes through")
	}
	require.Equal(t, 4, inner.attempts)

	// Circuit open: further calls must not reach the provider at all — no
	// request sent, no tokens spent, no quota consumed.
	_, err := prov.Chat(t.Context(), msgs)
	assert.True(t, resilience.IsUnavailable(err), "expected an open-circuit rejection, got %v", err)
	assert.Equal(t, 4, inner.attempts, "an open circuit must not attempt the call")

	u, ok := resilience.AsUnavailable(err)
	require.True(t, ok)
	assert.Equal(t, "ai:bedrock", u.Dependency)
	assert.Equal(t, resilience.KindAI, u.Kind)
}

func TestGuardCoversEveryCallShape(t *testing.T) {
	msgs := []aiprovider.Message{aiprovider.TextMessage("user", "hi")}
	schema := aiprovider.JSONSchema{Name: "s", Schema: map[string]any{}}

	// Each entry trips its own breaker, proving that shape is guarded rather
	// than silently bypassing the wrapper.
	cases := map[string]func(aiprovider.LLMProvider) error{
		"Chat": func(p aiprovider.LLMProvider) error {
			_, err := p.Chat(context.Background(), msgs)
			return err
		},
		"ChatStructured": func(p aiprovider.LLMProvider) error {
			_, err := p.ChatStructured(context.Background(), msgs, schema)
			return err
		},
		"Translate": func(p aiprovider.LLMProvider) error {
			_, err := p.Translate(context.Background(), aiprovider.TranslateRequest{})
			return err
		},
		"ChatStream": func(p aiprovider.LLMProvider) error {
			_, err := p.(aiprovider.StreamingLLMProvider).ChatStream(context.Background(), msgs, func(aiprovider.ChatStreamEvent) {})
			return err
		},
		"ChatStructuredStream": func(p aiprovider.LLMProvider) error {
			_, err := p.(aiprovider.StreamingLLMProvider).ChatStructuredStream(context.Background(), msgs, schema, func(aiprovider.ChatStreamEvent) {})
			return err
		},
	}

	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			reg := fastRegistry()
			inner := &fakeStreamingProvider{fakeProvider: &fakeProvider{id: "bedrock", err: errProviderDown}}
			prov := WrapWith(inner, reg)

			for range 4 {
				require.ErrorIs(t, call(prov), errProviderDown)
			}
			before := inner.attempts
			assert.True(t, resilience.IsUnavailable(call(prov)))
			assert.Equal(t, before, inner.attempts, "%s must be guarded", name)
		})
	}
}

func TestGuardPreservesStreamingInterface(t *testing.T) {
	inner := &fakeStreamingProvider{fakeProvider: &fakeProvider{id: "bedrock"}}
	prov := WrapWith(inner, fastRegistry())

	s, ok := prov.(aiprovider.StreamingLLMProvider)
	require.True(t, ok, "a streaming provider must not be downgraded by the guard — "+
		"that would silently disable live progress")

	resp, err := s.ChatStream(t.Context(), nil, func(aiprovider.ChatStreamEvent) {})
	require.NoError(t, err)
	assert.Equal(t, "streamed", resp.Content)
	assert.Equal(t, 1, inner.streamed)
}

func TestGuardDoesNotAddStreamingToNonStreamingProvider(t *testing.T) {
	prov := WrapWith(&fakeProvider{id: "demo"}, fastRegistry())
	_, ok := prov.(aiprovider.StreamingLLMProvider)
	assert.False(t, ok, "the guard must not claim a capability the provider lacks")
}

func TestGuardIsIdempotent(t *testing.T) {
	reg := fastRegistry()
	once := WrapWith(&fakeProvider{id: "bedrock"}, reg)
	twice := WrapWith(once, reg)
	assert.Same(t, once, twice,
		"the provider factories are called from several layers; stacking guards "+
			"would count one call many times")
}

func TestGuardPassesThroughNil(t *testing.T) {
	assert.Nil(t, WrapWith(nil, fastRegistry()))
}

func TestGuardKeepsProviderIdentity(t *testing.T) {
	prov := WrapWith(&fakeProvider{id: "bedrock"}, fastRegistry())
	assert.Equal(t, aiprovider.ProviderID("bedrock"), prov.Name())
	assert.NoError(t, prov.Close())
}

func TestGuardDoesNotTripOnBadCredentials(t *testing.T) {
	badKey := errors.New("bedrock: API error 403: not authorized")
	reg := fastRegistry()
	inner := &fakeProvider{id: "bedrock", err: badKey}
	prov := WrapWith(inner, reg)

	// One workspace's wrong key must never short-circuit the platform provider
	// for every other workspace.
	for range 20 {
		_, err := prov.Chat(t.Context(), nil)
		require.ErrorIs(t, err, badKey)
	}
	assert.Equal(t, 20, inner.attempts)
	assert.True(t, Available(prov))
}

func TestBreakerAndAvailableAccessors(t *testing.T) {
	reg := fastRegistry()
	prov := WrapWith(&fakeProvider{id: "bedrock", err: errProviderDown}, reg)

	b := Breaker(prov)
	require.NotNil(t, b)
	assert.Equal(t, "ai:bedrock", b.Name())
	assert.True(t, Available(prov))

	for range 4 {
		_, _ = prov.Chat(t.Context(), nil)
	}
	assert.False(t, Available(prov))

	// An unguarded provider is reported as available, never as broken.
	assert.Nil(t, Breaker(&fakeProvider{id: "raw"}))
	assert.True(t, Available(&fakeProvider{id: "raw"}))
}

func TestBreakerName(t *testing.T) {
	assert.Equal(t, "ai:bedrock", BreakerName("bedrock"))
	assert.Equal(t, "ai:anthropic", BreakerName(aiprovider.Anthropic))
}

func TestNewProviderReturnsGuarded(t *testing.T) {
	prov, err := NewProvider(aiprovider.ProviderID("demo"), aiprovider.Config{})
	require.NoError(t, err)
	require.NotNil(t, prov)
	assert.NotNil(t, Breaker(prov), "the constructor must never hand back an unguarded provider")
}

func TestNewProviderPropagatesConstructionError(t *testing.T) {
	_, err := NewProvider(aiprovider.ProviderID("no-such-provider"), aiprovider.Config{})
	assert.Error(t, err)
}
