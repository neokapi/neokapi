package aiprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeminiProviderName(t *testing.T) {
	t.Parallel()
	p := NewGeminiProvider(Config{APIKey: "test-key"})
	assert.Equal(t, "gemini", p.Name())
}

func TestGeminiProviderDefaults(t *testing.T) {
	t.Parallel()
	p := NewGeminiProvider(Config{})
	assert.Equal(t, "https://generativelanguage.googleapis.com", p.config.BaseURL)
	assert.Equal(t, "gemini-3-flash-preview", p.config.Model)
	assert.Equal(t, 4096, p.config.MaxTokens)
}

func TestGeminiProviderChat(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/v1beta/models/gemini-3-flash-preview:generateContent")
		assert.Equal(t, "test-key", r.URL.Query().Get("key"))

		var req geminiRequest
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Len(t, req.Contents, 1)
		assert.Equal(t, "user", req.Contents[0].Role)

		resp := geminiResponse{
			Candidates: []geminiCandidate{{
				Content: geminiContent{
					Role:  "model",
					Parts: []geminiPart{{Text: "Hello!"}},
				},
			}},
			ModelVersion: "gemini-3-flash-preview",
			UsageMetadata: geminiUsageMetadata{
				PromptTokenCount:     10,
				CandidatesTokenCount: 5,
				TotalTokenCount:      15,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := NewGeminiProvider(Config{
		BaseURL: srv.URL,
		APIKey:  "test-key",
	})

	resp, err := p.Chat(context.Background(), []Message{
		{Role: "user", Content: "Hi"},
	})
	require.NoError(t, err)
	assert.Equal(t, "Hello!", resp.Content)
	assert.Equal(t, "gemini-3-flash-preview", resp.Model)
	assert.Equal(t, 10, resp.Usage.InputTokens)
	assert.Equal(t, 5, resp.Usage.OutputTokens)
}

func TestGeminiProviderTranslate(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := geminiResponse{
			Candidates: []geminiCandidate{{
				Content: geminiContent{
					Role:  "model",
					Parts: []geminiPart{{Text: "Bonjour"}},
				},
			}},
			ModelVersion: "gemini-3-flash-preview",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := NewGeminiProvider(Config{
		BaseURL: srv.URL,
		APIKey:  "test-key",
	})

	resp, err := p.Translate(context.Background(), TranslateRequest{
		Source:         "Hello",
		SourceLanguage: "en",
		TargetLocale:   "fr",
	})
	require.NoError(t, err)
	assert.Equal(t, "Bonjour", resp.Translation)
	assert.Equal(t, 0.85, resp.Confidence)
}

func TestGeminiProviderChatStructured(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req geminiRequest
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "application/json", req.GenerationConfig.ResponseMIMEType)
		assert.NotNil(t, req.GenerationConfig.ResponseSchema)

		resp := geminiResponse{
			Candidates: []geminiCandidate{{
				Content: geminiContent{
					Role:  "model",
					Parts: []geminiPart{{Text: `{"translation":"Bonjour","confidence":0.9}`}},
				},
			}},
			ModelVersion: "gemini-3-flash-preview",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := NewGeminiProvider(Config{
		BaseURL: srv.URL,
		APIKey:  "test-key",
	})

	schema := JSONSchema{
		Name: "translation",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"translation": map[string]any{"type": "string"},
				"confidence":  map[string]any{"type": "number"},
			},
		},
	}

	resp, err := p.ChatStructured(context.Background(), []Message{
		{Role: "user", Content: "Translate Hello to French"},
	}, schema)
	require.NoError(t, err)
	assert.Contains(t, resp.Content, "Bonjour")
}

func TestGeminiProviderSystemMessage(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req geminiRequest
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&req))

		// System message should be prepended to first user message.
		assert.Len(t, req.Contents, 1)
		assert.Equal(t, "user", req.Contents[0].Role)
		assert.Contains(t, req.Contents[0].Parts[0].Text, "You are a translator")
		assert.Contains(t, req.Contents[0].Parts[0].Text, "Translate this")

		resp := geminiResponse{
			Candidates: []geminiCandidate{{
				Content: geminiContent{
					Role:  "model",
					Parts: []geminiPart{{Text: "OK"}},
				},
			}},
			ModelVersion: "gemini-3-flash-preview",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := NewGeminiProvider(Config{
		BaseURL: srv.URL,
		APIKey:  "test-key",
	})

	_, err := p.Chat(context.Background(), []Message{
		{Role: "system", Content: "You are a translator"},
		{Role: "user", Content: "Translate this"},
	})
	require.NoError(t, err)
}

func TestGeminiProviderFiltersThinkingParts(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate a response with thinking parts interleaved.
		resp := geminiResponse{
			Candidates: []geminiCandidate{{
				Content: geminiContent{
					Role: "model",
					Parts: []geminiPart{
						{Text: "Let me think about this translation...", Thought: true},
						{Text: "Bonjour le monde"},
					},
				},
			}},
			ModelVersion: "gemini-3-flash-preview",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := NewGeminiProvider(Config{
		BaseURL: srv.URL,
		APIKey:  "test-key",
	})

	resp, err := p.Chat(context.Background(), []Message{
		{Role: "user", Content: "Translate Hello world to French"},
	})
	require.NoError(t, err)
	assert.Equal(t, "Bonjour le monde", resp.Content)
}

func TestGeminiProviderDisablesThinking(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req geminiRequest
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&req))

		// Verify thinking is disabled (budget = 0).
		assert.NotNil(t, req.GenerationConfig)
		assert.NotNil(t, req.GenerationConfig.ThinkingConfig)
		assert.Equal(t, 0, req.GenerationConfig.ThinkingConfig.ThinkingBudget)

		resp := geminiResponse{
			Candidates: []geminiCandidate{{
				Content: geminiContent{
					Role:  "model",
					Parts: []geminiPart{{Text: "OK"}},
				},
			}},
			ModelVersion: "gemini-3-flash-preview",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := NewGeminiProvider(Config{
		BaseURL: srv.URL,
		APIKey:  "test-key",
	})

	_, err := p.Chat(context.Background(), []Message{
		{Role: "user", Content: "Hi"},
	})
	require.NoError(t, err)
}

func TestGeminiProviderChatStream(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "streamGenerateContent")
		assert.Equal(t, "sse", r.URL.Query().Get("alt"))

		var req geminiRequest
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.NotNil(t, req.GenerationConfig.ThinkingConfig)
		assert.True(t, req.GenerationConfig.ThinkingConfig.IncludeThoughts)

		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		// Chunk 1: thinking
		chunk1 := geminiResponse{
			Candidates: []geminiCandidate{{
				Content: geminiContent{
					Role:  "model",
					Parts: []geminiPart{{Text: "Considering translation...", Thought: true}},
				},
			}},
			ModelVersion: "gemini-3-flash-preview",
		}
		data1, _ := json.Marshal(chunk1)
		fmt.Fprintf(w, "data: %s\n\n", data1)
		flusher.Flush()

		// Chunk 2: content
		chunk2 := geminiResponse{
			Candidates: []geminiCandidate{{
				Content: geminiContent{
					Role:  "model",
					Parts: []geminiPart{{Text: "Bonjour"}},
				},
			}},
			ModelVersion: "gemini-3-flash-preview",
			UsageMetadata: geminiUsageMetadata{
				PromptTokenCount:     12,
				CandidatesTokenCount: 8,
				TotalTokenCount:      20,
			},
		}
		data2, _ := json.Marshal(chunk2)
		fmt.Fprintf(w, "data: %s\n\n", data2)
		flusher.Flush()
	}))
	defer srv.Close()

	p := NewGeminiProvider(Config{
		BaseURL: srv.URL,
		APIKey:  "test-key",
	})

	var events []ChatStreamEvent
	resp, err := p.ChatStream(context.Background(), []Message{
		{Role: "user", Content: "Translate Hello to French"},
	}, func(e ChatStreamEvent) {
		events = append(events, e)
	})

	require.NoError(t, err)
	assert.Equal(t, "Bonjour", resp.Content)
	assert.Equal(t, 12, resp.Usage.InputTokens)
	assert.Equal(t, 8, resp.Usage.OutputTokens)

	// Should have: thinking event, content event, done event.
	require.Len(t, events, 3)
	assert.Equal(t, StreamEventThinking, events[0].Type)
	assert.Equal(t, "Considering translation...", events[0].Content)
	assert.Equal(t, StreamEventContent, events[1].Type)
	assert.Equal(t, "Bonjour", events[1].Content)
	assert.Equal(t, StreamEventDone, events[2].Type)
	assert.Equal(t, 12, events[2].Usage.InputTokens)
}

func TestGeminiProviderChatStreamMultipleContentChunks(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		// Three content chunks.
		for _, text := range []string{"Bon", "jour", " le monde"} {
			chunk := geminiResponse{
				Candidates: []geminiCandidate{{
					Content: geminiContent{
						Role:  "model",
						Parts: []geminiPart{{Text: text}},
					},
				}},
				ModelVersion: "gemini-3-flash-preview",
			}
			data, _ := json.Marshal(chunk)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}))
	defer srv.Close()

	p := NewGeminiProvider(Config{
		BaseURL: srv.URL,
		APIKey:  "test-key",
	})

	var contentEvents []string
	resp, err := p.ChatStream(context.Background(), []Message{
		{Role: "user", Content: "Hi"},
	}, func(e ChatStreamEvent) {
		if e.Type == StreamEventContent {
			contentEvents = append(contentEvents, e.Content)
		}
	})

	require.NoError(t, err)
	assert.Equal(t, "Bonjour le monde", resp.Content)
	assert.Equal(t, []string{"Bon", "jour", " le monde"}, contentEvents)
}

func TestGeminiProviderImplementsStreamingInterface(t *testing.T) {
	t.Parallel()
	p := NewGeminiProvider(Config{APIKey: "test-key"})
	var provider LLMProvider = p

	sp, ok := provider.(StreamingLLMProvider)
	assert.True(t, ok, "GeminiProvider should implement StreamingLLMProvider")
	assert.NotNil(t, sp)
}

func TestGeminiProviderClose(t *testing.T) {
	t.Parallel()
	p := NewGeminiProvider(Config{APIKey: "test-key"})
	require.NoError(t, p.Close())
}
