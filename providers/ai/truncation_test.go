package aiprovider

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/safeio"
)

// Every provider computes "did the model stop because it ran out of room", and
// the value of that signal is entirely in whether it reaches the caller. These
// tests pin the wire fact (a finish reason) to the field the translate path
// reads, per provider and per method, because a provider that quietly drops it
// turns a recoverable "ask for less" into a corrupted target or a dead run.

// azureChatCompletion is an Azure/OpenAI reply with a caller-chosen finish reason.
func azureChatCompletion(content, finishReason string) openaiResponse {
	return openaiResponse{
		Choices: []openaiChoice{{
			FinishReason: finishReason,
			Message:      openaiMessage{Content: content},
		}},
		Model: "gpt-4o",
	}
}

func TestAzureOpenAIReportsTruncation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		finishReason string
		want         bool
	}{
		{name: "complete reply", finishReason: "stop", want: false},
		{name: "hit the output cap", finishReason: "length", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(azureChatCompletion(`{"translations":[{"id":"s1","te`, tt.finishReason))
			}))
			defer srv.Close()

			p := NewAzureOpenAIProvider(Config{BaseURL: srv.URL, APIKey: "k", Model: "gpt-4o"})

			chat, err := p.Chat(t.Context(), []Message{TextMessage(RoleUser, "Hi")})
			require.NoError(t, err)
			assert.Equal(t, tt.want, chat.Truncated, "Chat must report the finish reason")

			structured, err := p.ChatStructured(t.Context(), []Message{TextMessage(RoleUser, "Hi")},
				JSONSchema{Name: "batch_translations", Schema: map[string]any{"type": "object"}})
			require.NoError(t, err)
			assert.Equal(t, tt.want, structured.Truncated, "ChatStructured must report the finish reason")
		})
	}
}

func TestGeminiStreamReportsTruncation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		finishReason string
		want         bool
	}{
		{name: "complete reply", finishReason: "STOP", want: false},
		{name: "hit the output cap", finishReason: geminiFinishMaxTokens, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				flusher, _ := w.(http.Flusher)
				// A stream carries content first and the finish reason last, so a
				// reader that only inspects the first chunk sees nothing.
				chunks := []geminiResponse{
					{Candidates: []geminiCandidate{{Content: geminiContent{Parts: []geminiPart{{Text: `{"translations":[`}}}}}},
					{
						Candidates:    []geminiCandidate{{Content: geminiContent{Parts: []geminiPart{{Text: `{"id":"s1","te`}}}, FinishReason: tt.finishReason}},
						ModelVersion:  "gemini-3.5-flash",
						UsageMetadata: geminiUsageMetadata{PromptTokenCount: 5, CandidatesTokenCount: 9, TotalTokenCount: 14},
					},
				}
				for _, c := range chunks {
					b, _ := json.Marshal(c)
					_, _ = fmt.Fprintf(w, "data: %s\n\n", b)
					if flusher != nil {
						flusher.Flush()
					}
				}
			}))
			defer srv.Close()

			p := NewGeminiProvider(Config{BaseURL: srv.URL, APIKey: "k"})

			resp, err := p.ChatStream(t.Context(), []Message{TextMessage(RoleUser, "Hi")}, func(ChatStreamEvent) {})
			require.NoError(t, err)
			assert.Equal(t, tt.want, resp.Truncated, "ChatStream must report the stream's finish reason")
			assert.Equal(t, `{"translations":[{"id":"s1","te`, resp.Content, "content accumulates regardless")

			structured, err := p.ChatStructuredStream(t.Context(), []Message{TextMessage(RoleUser, "Hi")},
				JSONSchema{Name: "batch_translations", Schema: map[string]any{"type": "object"}},
				func(ChatStreamEvent) {})
			require.NoError(t, err)
			assert.Equal(t, tt.want, structured.Truncated, "ChatStructuredStream must report the stream's finish reason")
		})
	}
}

// The translate path reads TranslateResponse, not ChatResponse. StandardTranslate
// is the only bridge between the two for every provider that has one, so a
// truncation dropped here is dropped for all of them at once.
func TestStandardTranslateCarriesTruncation(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(azureChatCompletion("Hei, ver", "length"))
	}))
	defer srv.Close()

	p := NewAzureOpenAIProvider(Config{BaseURL: srv.URL, APIKey: "k", Model: "gpt-4o"})

	resp, err := p.Translate(t.Context(), TranslateRequest{
		Source: "Hello, world", SourceLanguage: "en", TargetLocale: "nb",
	})
	require.NoError(t, err)
	assert.True(t, resp.Truncated, "a translation cut off at the cap must say so")
	assert.Equal(t, "Hei, ver", resp.Translation)
}

// A provider's response body is written by a remote endpoint and read whole into
// memory. The cap is generous enough that no real reply approaches it, and it
// fails rather than truncating — a partial body would parse as a malformed reply
// and be blamed on the model.
func TestProviderResponseBodyIsCapped(t *testing.T) {
	t.Parallel()

	body, err := readCappedBody(strings.NewReader("a normal reply"))
	require.NoError(t, err)
	assert.Equal(t, "a normal reply", string(body))

	// A transport failure mid-body still surfaces as a read error.
	_, err = readCappedBody(iotest.ErrReader(io.ErrUnexpectedEOF))
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)

	// An endpoint that never stops writing.
	_, err = readCappedBody(io.LimitReader(neverEndingReader{}, MaxProviderResponseBytes+1))
	require.Error(t, err)
	assert.ErrorIs(t, err, safeio.ErrByteBudget)
}

type neverEndingReader struct{}

func (neverEndingReader) Read(p []byte) (int, error) { return len(p), nil }

// A provider outside this package needs to read the request budget kapi sized
// for it; without an exported accessor it can only send its own fixed default.
func TestMaxOutputTokensFromIsReadableByProviders(t *testing.T) {
	t.Parallel()

	n, ok := MaxOutputTokensFrom(t.Context())
	assert.False(t, ok)
	assert.Zero(t, n)

	n, ok = MaxOutputTokensFrom(WithMaxOutputTokens(t.Context(), 9000))
	assert.True(t, ok)
	assert.Equal(t, 9000, n)

	// A non-positive budget is not a budget.
	n, ok = MaxOutputTokensFrom(WithMaxOutputTokens(t.Context(), 0))
	assert.False(t, ok)
	assert.Zero(t, n)
}
