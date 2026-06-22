package aiprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// decodeOllamaRequest reads the captured /api/chat request body.
func decodeOllamaRequest(t *testing.T, r *http.Request) ollamaChatRequest {
	t.Helper()
	var req ollamaChatRequest
	require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
	return req
}

func TestOllamaChatSendsOptionsAndKeepAlive(t *testing.T) {
	var got ollamaChatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/chat", r.URL.Path)
		got = decodeOllamaRequest(t, r)
		_ = json.NewEncoder(w).Encode(ollamaChatResponse{
			Model:           "llama3.2:3b",
			Message:         ollamaMessage{Role: "assistant", Content: "bonjour"},
			Done:            true,
			PromptEvalCount: 3,
			EvalCount:       2,
		})
	}))
	defer srv.Close()

	p := NewOllamaProvider(Config{BaseURL: srv.URL, Model: "llama3.2:3b", MaxTokens: 128})
	resp, err := p.Chat(context.Background(), []Message{TextMessage("user", "hi")})
	require.NoError(t, err)
	assert.Equal(t, "bonjour", resp.Content)
	assert.Equal(t, 3, resp.Usage.InputTokens)
	assert.Equal(t, 2, resp.Usage.OutputTokens)

	// Deterministic translation defaults: low temp, num_predict from MaxTokens,
	// keep-alive set, reasoning disabled, non-streaming.
	require.NotNil(t, got.Options)
	require.NotNil(t, got.Options.Temperature)
	assert.InDelta(t, ollamaTranslateTemperature, *got.Options.Temperature, 1e-9)
	require.NotNil(t, got.Options.NumPredict)
	assert.Equal(t, 128, *got.Options.NumPredict)
	assert.Equal(t, ollamaKeepAlive, got.KeepAlive)
	require.NotNil(t, got.Think)
	assert.False(t, *got.Think)
	assert.False(t, got.Stream)
}

func TestOllamaChatRespectsExplicitTemperature(t *testing.T) {
	var got ollamaChatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = decodeOllamaRequest(t, r)
		_ = json.NewEncoder(w).Encode(ollamaChatResponse{Done: true})
	}))
	defer srv.Close()

	p := NewOllamaProvider(Config{BaseURL: srv.URL, Model: "m", Temperature: 0.9})
	_, err := p.Chat(context.Background(), []Message{TextMessage("user", "hi")})
	require.NoError(t, err)
	require.NotNil(t, got.Options.Temperature)
	assert.InDelta(t, 0.9, *got.Options.Temperature, 1e-9)
}

func TestOllamaChatStructuredPassesFormat(t *testing.T) {
	schema := JSONSchema{Schema: map[string]any{"type": "object"}}
	var got ollamaChatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = decodeOllamaRequest(t, r)
		_ = json.NewEncoder(w).Encode(ollamaChatResponse{Message: ollamaMessage{Content: "{}"}, Done: true})
	}))
	defer srv.Close()

	p := NewOllamaProvider(Config{BaseURL: srv.URL, Model: "m"})
	_, err := p.ChatStructured(context.Background(), []Message{TextMessage("user", "hi")}, schema)
	require.NoError(t, err)
	assert.NotNil(t, got.Format)
}

func TestOllamaChatStreamEmitsEvents(t *testing.T) {
	// Emit a thinking chunk, two content chunks, then a done frame with usage.
	frames := []ollamaChatResponse{
		{Message: ollamaMessage{Role: "assistant", Thinking: "pondering"}},
		{Message: ollamaMessage{Role: "assistant", Content: "bon"}},
		{Message: ollamaMessage{Role: "assistant", Content: "jour"}},
		{Model: "llama3.2:3b", Done: true, PromptEvalCount: 5, EvalCount: 4},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := decodeOllamaRequest(t, r)
		assert.True(t, got.Stream)
		enc := json.NewEncoder(w)
		for _, f := range frames {
			assert.NoError(t, enc.Encode(f))
		}
	}))
	defer srv.Close()

	p := NewOllamaProvider(Config{BaseURL: srv.URL, Model: "m"})
	var thinking, content []string
	var doneUsage TokenUsage
	resp, err := p.ChatStream(context.Background(), []Message{TextMessage("user", "hi")}, func(e ChatStreamEvent) {
		switch e.Type {
		case StreamEventThinking:
			thinking = append(thinking, e.Content)
		case StreamEventContent:
			content = append(content, e.Content)
		case StreamEventDone:
			doneUsage = e.Usage
		}
	})
	require.NoError(t, err)
	assert.Equal(t, "bonjour", resp.Content)
	assert.Equal(t, []string{"pondering"}, thinking)
	assert.Equal(t, []string{"bon", "jour"}, content)
	assert.Equal(t, 5, doneUsage.InputTokens)
	assert.Equal(t, 4, doneUsage.OutputTokens)
	assert.Equal(t, "llama3.2:3b", resp.Model)
}

func TestOllamaTransportErrorIsActionable(t *testing.T) {
	// Point at a closed server to force a connection failure.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	p := NewOllamaProvider(Config{BaseURL: url, Model: "m"})
	_, err := p.Chat(context.Background(), []Message{TextMessage("user", "hi")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is it running")
	assert.Contains(t, err.Error(), "ollama.com")
}

func TestOllamaModelNotFoundSuggestsPull(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"model 'qwen3:1.7b' not found"}`))
	}))
	defer srv.Close()

	p := NewOllamaProvider(Config{BaseURL: srv.URL, Model: "qwen3:1.7b"})
	_, err := p.Chat(context.Background(), []Message{TextMessage("user", "hi")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ollama pull qwen3:1.7b")
}

func TestOllamaImplementsStreamingProvider(t *testing.T) {
	var p LLMProvider = NewOllamaProvider(Config{Model: "m"})
	_, ok := p.(StreamingLLMProvider)
	assert.True(t, ok, "OllamaProvider must satisfy StreamingLLMProvider")
}

func TestOllamaDefaultModel(t *testing.T) {
	p := NewOllamaProvider(Config{})
	assert.Equal(t, "llama3.2:3b", p.config.Model)
	assert.True(t, strings.HasPrefix(p.config.BaseURL, "http"))
}
