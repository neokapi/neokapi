package aiprovider

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEveryProviderThatAcceptsABaseURLCallsIt is the guard on the endpoint a
// credential names. A self-hosted or proxied deployment is reached by setting
// Config.BaseURL, and a provider that read the field and then built its request
// URL from a constant would authenticate against the vendor's public endpoint
// with the user's key, which is a leak rather than a misconfiguration.
//
// Every call goes through the registry, so the wiring from a provider id to its
// factory is covered along with the provider itself.
func TestEveryProviderThatAcceptsABaseURLCallsIt(t *testing.T) {
	cases := []struct {
		id    ProviderID
		reply string
	}{
		{Anthropic, anthropicReply},
		{OpenAI, openaiReply},
		{Gemini, geminiReply},
		{AzureOpenAI, openaiReply},
		{Ollama, `{"message":{"content":"ok"},"done":true}`},
	}
	for _, c := range cases {
		t.Run(string(c.id), func(t *testing.T) {
			srv, seen := captureBody(t, c.reply)
			p, err := NewProvider(c.id, Config{BaseURL: srv.URL, Model: "m", APIKey: "k"})
			require.NoError(t, err)

			_, _ = p.Chat(context.Background(), []Message{TextMessage(RoleUser, "hi")})
			assert.NotEmpty(t, *seen, "the configured endpoint was never called")
		})
	}
}

// TestProvidersWithNoEndpointOfTheirOwn records why the two remaining kinds are
// absent above: one talks to a local CLI and one answers offline, so neither
// has an address to point anywhere.
func TestProvidersWithNoEndpointOfTheirOwn(t *testing.T) {
	for _, id := range []ProviderID{ClaudeCode, Demo} {
		t.Run(string(id), func(t *testing.T) {
			p, err := NewProvider(id, Config{BaseURL: "http://127.0.0.1:1", Model: "m"})
			require.NoError(t, err)
			assert.Equal(t, id, p.Name())
		})
	}
}

// TestDefaultEndpointsSurviveAnEmptyBaseURL: leaving the field empty must keep
// the vendor's own endpoint, so a credential with no --base-url behaves as it
// always has.
func TestDefaultEndpointsSurviveAnEmptyBaseURL(t *testing.T) {
	for _, c := range []struct {
		id   ProviderID
		want string
	}{
		{Anthropic, "https://api.anthropic.com"},
		{OpenAI, "https://api.openai.com"},
		{Gemini, "https://generativelanguage.googleapis.com"},
		{Ollama, DefaultOllamaBaseURL},
	} {
		t.Run(string(c.id), func(t *testing.T) {
			p, err := NewProvider(c.id, Config{Model: "m", APIKey: "k"})
			require.NoError(t, err)
			assert.Equal(t, c.want, providerBaseURL(t, p))
		})
	}
}

// providerBaseURL reads the endpoint a built provider will call.
func providerBaseURL(t *testing.T, p LLMProvider) string {
	t.Helper()
	switch v := p.(type) {
	case *AnthropicProvider:
		return v.config.BaseURL
	case *OpenAIProvider:
		return v.config.BaseURL
	case *GeminiProvider:
		return v.config.BaseURL
	case *OllamaProvider:
		return v.config.BaseURL
	default:
		t.Fatalf("no endpoint accessor for %T", p)
		return ""
	}
}
