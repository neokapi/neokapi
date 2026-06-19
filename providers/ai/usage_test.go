package aiprovider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenUsageTotalTokens(t *testing.T) {
	t.Parallel()
	u := TokenUsage{InputTokens: 100, OutputTokens: 50}
	assert.Equal(t, 150, u.TotalTokens())
}

func TestTokenUsageAdd(t *testing.T) {
	t.Parallel()
	a := TokenUsage{InputTokens: 100, OutputTokens: 50, CacheCreationTokens: 10, CacheReadTokens: 5}
	b := TokenUsage{InputTokens: 200, OutputTokens: 75, CacheCreationTokens: 20, CacheReadTokens: 15}
	sum := a.Add(b)
	assert.Equal(t, 300, sum.InputTokens)
	assert.Equal(t, 125, sum.OutputTokens)
	assert.Equal(t, 30, sum.CacheCreationTokens)
	assert.Equal(t, 20, sum.CacheReadTokens)
}

func TestTokenUsageZeroValue(t *testing.T) {
	t.Parallel()
	var u TokenUsage
	assert.Equal(t, 0, u.TotalTokens())
	assert.Equal(t, TokenUsage{}, u.Add(TokenUsage{}))
}

func TestOpenAIProviderParsesUsage(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "Hello!"}},
			},
			"model": "gpt-4o",
			"usage": map[string]any{
				"prompt_tokens":     150,
				"completion_tokens": 42,
				"total_tokens":      192,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := NewOpenAIProvider(Config{BaseURL: srv.URL, APIKey: "test", Model: "gpt-4o"})
	resp, err := p.Chat(t.Context(), []Message{TextMessage("user", "Hi")})
	require.NoError(t, err)
	assert.Equal(t, 150, resp.Usage.InputTokens)
	assert.Equal(t, 42, resp.Usage.OutputTokens)
	assert.Equal(t, 192, resp.Usage.TotalTokens())
}

func TestOpenAIProviderTranslatePropagatesUsage(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "Bonjour"}},
			},
			"model": "gpt-4o",
			"usage": map[string]any{
				"prompt_tokens":     100,
				"completion_tokens": 10,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := NewOpenAIProvider(Config{BaseURL: srv.URL, APIKey: "test", Model: "gpt-4o"})
	resp, err := p.Translate(t.Context(), TranslateRequest{
		Source: "Hello", SourceLanguage: "en", TargetLocale: "fr",
	})
	require.NoError(t, err)
	assert.Equal(t, 100, resp.Usage.InputTokens)
	assert.Equal(t, 10, resp.Usage.OutputTokens)
}

func TestAzureOpenAIProviderParsesUsage(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openaiResponse{
			Choices: []openaiChoice{{Message: openaiMessage{Content: "Hei"}}},
			Model:   "gpt-4o",
			Usage:   openaiUsage{PromptTokens: 200, CompletionTokens: 30},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := NewAzureOpenAIProvider(Config{BaseURL: srv.URL, APIKey: "test", Model: "gpt-4o"})
	resp, err := p.Chat(t.Context(), []Message{TextMessage("user", "Hi")})
	require.NoError(t, err)
	assert.Equal(t, 200, resp.Usage.InputTokens)
	assert.Equal(t, 30, resp.Usage.OutputTokens)
}

func TestAnthropicProviderParsesUsage(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": "Hello!"},
			},
			"model": "claude-sonnet-4-20250514",
			"usage": map[string]any{
				"input_tokens":                150,
				"output_tokens":               42,
				"cache_creation_input_tokens": 10,
				"cache_read_input_tokens":     5,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := NewAnthropicProvider(Config{BaseURL: srv.URL, APIKey: "test", Model: "claude-sonnet-4-20250514"})
	resp, err := p.Chat(t.Context(), []Message{TextMessage("user", "Hi")})
	require.NoError(t, err)
	assert.Equal(t, 150, resp.Usage.InputTokens)
	assert.Equal(t, 42, resp.Usage.OutputTokens)
	assert.Equal(t, 10, resp.Usage.CacheCreationTokens)
	assert.Equal(t, 5, resp.Usage.CacheReadTokens)
	assert.Equal(t, 192, resp.Usage.TotalTokens())
}

func TestAnthropicProviderChatStructuredParsesUsage(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"content": []map[string]any{
				{"type": "tool_use", "id": "call1", "name": "structured_output", "input": map[string]any{"result": "ok"}},
			},
			"model": "claude-sonnet-4-20250514",
			"usage": map[string]any{
				"input_tokens":  300,
				"output_tokens": 80,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := NewAnthropicProvider(Config{BaseURL: srv.URL, APIKey: "test", Model: "claude-sonnet-4-20250514"})
	resp, err := p.ChatStructured(t.Context(), []Message{TextMessage("user", "Hi")}, JSONSchema{
		Name:   "structured_output",
		Schema: map[string]any{"type": "object"},
	})
	require.NoError(t, err)
	assert.Equal(t, 300, resp.Usage.InputTokens)
	assert.Equal(t, 80, resp.Usage.OutputTokens)
}

func TestMockProviderReturnsUsage(t *testing.T) {
	t.Parallel()
	mock := NewMockProvider()

	resp, err := mock.Translate(t.Context(), TranslateRequest{
		Source: "Hello", SourceLanguage: "en", TargetLocale: "fr",
	})
	require.NoError(t, err)
	assert.Equal(t, 10, resp.Usage.InputTokens)
	assert.Equal(t, 20, resp.Usage.OutputTokens)

	chatResp, err := mock.Chat(t.Context(), []Message{TextMessage("user", "Hi")})
	require.NoError(t, err)
	assert.Equal(t, 10, chatResp.Usage.InputTokens)
	assert.Equal(t, 20, chatResp.Usage.OutputTokens)

	structResp, err := mock.ChatStructured(t.Context(), []Message{TextMessage("user", "Hi")}, JSONSchema{
		Name:   "test",
		Schema: map[string]any{"type": "object"},
	})
	require.NoError(t, err)
	assert.Equal(t, 10, structResp.Usage.InputTokens)
	assert.Equal(t, 20, structResp.Usage.OutputTokens)
}

func TestProviderUsageZeroWhenNotInResponse(t *testing.T) {
	t.Parallel()
	// Verify that when the API doesn't return usage, we get zero values.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "Hello"}},
			},
			"model": "gpt-4o",
			// no "usage" field
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := NewOpenAIProvider(Config{BaseURL: srv.URL, APIKey: "test", Model: "gpt-4o"})
	resp, err := p.Chat(t.Context(), []Message{TextMessage("user", "Hi")})
	require.NoError(t, err)
	assert.Equal(t, 0, resp.Usage.InputTokens)
	assert.Equal(t, 0, resp.Usage.OutputTokens)
	assert.Equal(t, 0, resp.Usage.TotalTokens())
}
