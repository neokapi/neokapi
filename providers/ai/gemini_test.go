package aiprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGeminiProviderName(t *testing.T) {
	t.Parallel()
	p := NewGeminiProvider(Config{APIKey: "test-key"})
	assert.Equal(t, Gemini, p.Name())
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
		// API key travels in a header, not the URL query string.
		assert.Equal(t, "test-key", r.Header.Get("x-goog-api-key"))
		assert.Empty(t, r.URL.Query().Get("key"))

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

	resp, err := p.Chat(t.Context(), []Message{
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

	resp, err := p.Translate(t.Context(), TranslateRequest{
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

	resp, err := p.ChatStructured(t.Context(), []Message{
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

	_, err := p.Chat(t.Context(), []Message{
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

	resp, err := p.Chat(t.Context(), []Message{
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

	_, err := p.Chat(t.Context(), []Message{
		{Role: "user", Content: "Hi"},
	})
	require.NoError(t, err)
}

func TestGeminiProviderChatStream(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "streamGenerateContent")
		assert.Equal(t, "sse", r.URL.Query().Get("alt"))
		// API key travels in a header, not the URL query string.
		assert.Equal(t, "test-key", r.Header.Get("x-goog-api-key"))
		assert.Empty(t, r.URL.Query().Get("key"))

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
	resp, err := p.ChatStream(t.Context(), []Message{
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
	resp, err := p.ChatStream(t.Context(), []Message{
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

// TestGeminiProviderTransportErrorDoesNotLeakKey verifies that when a request
// fails at the transport layer, the resulting error string never contains the
// API key. The key now travels in the x-goog-api-key header, so it must not
// appear in the request URL (which transport errors echo back). Covers both the
// blocking (Chat) and streaming (ChatStream) paths.
func TestGeminiProviderTransportErrorDoesNotLeakKey(t *testing.T) {
	t.Parallel()

	const secret = "super-secret-api-key-12345"

	// Point at a port nothing is listening on so the transport fails to connect.
	// Grab a free port, then close the listener immediately.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())
	baseURL := "http://" + addr

	t.Run("blocking", func(t *testing.T) {
		t.Parallel()
		p := NewGeminiProvider(Config{BaseURL: baseURL, APIKey: secret})
		_, err := p.Chat(t.Context(), []Message{{Role: "user", Content: "Hi"}})
		require.Error(t, err)
		assert.NotContains(t, err.Error(), secret, "API key leaked into error: %v", err)
	})

	t.Run("streaming", func(t *testing.T) {
		t.Parallel()
		p := NewGeminiProvider(Config{BaseURL: baseURL, APIKey: secret})
		_, err := p.ChatStream(t.Context(), []Message{{Role: "user", Content: "Hi"}}, func(ChatStreamEvent) {})
		require.Error(t, err)
		assert.NotContains(t, err.Error(), secret, "API key leaked into error: %v", err)
	})
}

// TestGeminiProviderChatStreamLargeDataLine verifies that an SSE "data:" line
// larger than bufio.Scanner's default 64KB cap is parsed without aborting the
// stream with bufio.ErrTooLong.
func TestGeminiProviderChatStreamLargeDataLine(t *testing.T) {
	t.Parallel()

	// Build a content chunk whose JSON-encoded "data:" line exceeds 64KB.
	largeText := strings.Repeat("x", 200*1024)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		chunk := geminiResponse{
			Candidates: []geminiCandidate{{
				Content: geminiContent{
					Role:  "model",
					Parts: []geminiPart{{Text: largeText}},
				},
			}},
			ModelVersion: "gemini-3-flash-preview",
			UsageMetadata: geminiUsageMetadata{
				PromptTokenCount:     1,
				CandidatesTokenCount: 1,
				TotalTokenCount:      2,
			},
		}
		data, _ := json.Marshal(chunk)
		require.Greater(t, len(data), 64*1024, "test chunk must exceed the default scanner cap")
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}))
	defer srv.Close()

	p := NewGeminiProvider(Config{BaseURL: srv.URL, APIKey: "test-key"})

	resp, err := p.ChatStream(t.Context(), []Message{{Role: "user", Content: "Hi"}}, func(ChatStreamEvent) {})
	require.NoError(t, err)
	assert.Equal(t, largeText, resp.Content)
}

// TestGeminiProviderChatStreamRespectsContextDeadline verifies that the
// streaming path honors a context deadline: a server that hangs without sending
// a complete stream must not block forever — the request fails once the caller's
// context is cancelled.
func TestGeminiProviderChatStreamRespectsContextDeadline(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		// Hang until the test releases us (or the server shuts down), never
		// completing the stream.
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release)

	p := NewGeminiProvider(Config{BaseURL: srv.URL, APIKey: "test-key"})

	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := p.ChatStream(ctx, []Message{{Role: "user", Content: "Hi"}}, func(ChatStreamEvent) {})
	require.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(start), geminiStreamTimeout, "stream should honor the caller's deadline, not the 5m cap")
}
