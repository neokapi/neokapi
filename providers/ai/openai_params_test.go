package aiprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// paramServer refuses each parameter named in refuse the way OpenAI does, with a
// 400 naming it, and records every request body it receives.
func paramServer(t *testing.T, refuse ...string) (*httptest.Server, func() []map[string]any) {
	t.Helper()
	var mu sync.Mutex
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		bodies = append(bodies, body)
		mu.Unlock()
		for _, p := range refuse {
			if _, ok := body[p]; ok {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{
					"message": "Unsupported parameter", "param": p, "code": "unsupported_parameter"}})
				return
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model":   "m",
			"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": "ok"}, "finish_reason": "stop"}},
		})
	}))
	t.Cleanup(srv.Close)
	return srv, func() []map[string]any { mu.Lock(); defer mu.Unlock(); return append([]map[string]any(nil), bodies...) }
}

func TestOpenAIAdaptsToRefusedParameters(t *testing.T) {
	temp := 0.3
	tests := []struct {
		name        string
		refuse      []string
		wantFirst   []string // keys the first successful body carries
		wantMissing []string // keys it must not carry
		wantCalls   int      // requests for the first Chat call
	}{
		{name: "compatible server takes max_tokens", wantFirst: []string{"max_tokens", "temperature"}, wantMissing: []string{"max_completion_tokens"}, wantCalls: 1},
		{name: "max_tokens refused", refuse: []string{"max_tokens"}, wantFirst: []string{"max_completion_tokens", "temperature"}, wantMissing: []string{"max_tokens"}, wantCalls: 2},
		{name: "temperature refused", refuse: []string{"temperature"}, wantFirst: []string{"max_tokens"}, wantMissing: []string{"temperature"}, wantCalls: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, bodies := paramServer(t, tt.refuse...)
			p := NewOpenAIProvider(Config{APIKey: "k", Model: "m", BaseURL: srv.URL, Temperature: &temp})

			resp, err := p.Chat(context.Background(), []Message{TextMessage("user", "hi")})
			require.NoError(t, err)
			assert.Equal(t, "ok", resp.Content)
			got := bodies()
			require.Len(t, got, tt.wantCalls)
			last := got[len(got)-1]
			for _, k := range tt.wantFirst {
				assert.Contains(t, last, k)
			}
			for _, k := range tt.wantMissing {
				assert.NotContains(t, last, k)
			}

			// The provider remembers the refusal: the next call needs one request.
			_, err = p.Chat(context.Background(), []Message{TextMessage("user", "again")})
			require.NoError(t, err)
			assert.Len(t, bodies(), tt.wantCalls+1)
		})
	}
}

// Two refusals in one request surface as an error after a single retry rather
// than looping.
func TestOpenAIRetriesOnce(t *testing.T) {
	temp := 0.3
	srv, bodies := paramServer(t, "max_tokens", "temperature")
	p := NewOpenAIProvider(Config{APIKey: "k", Model: "m", BaseURL: srv.URL, Temperature: &temp})

	_, err := p.Chat(context.Background(), []Message{TextMessage("user", "hi")})
	require.Error(t, err)
	assert.Len(t, bodies(), 2)

	// The next call starts from both learned forms and succeeds.
	_, err = p.Chat(context.Background(), []Message{TextMessage("user", "hi")})
	require.NoError(t, err)
}

func TestOpenAIUsesCompletionTokensOnItsOwnEndpoint(t *testing.T) {
	p := NewOpenAIProvider(Config{APIKey: "k"})
	assert.True(t, p.completionTokens())
	assert.False(t, NewOpenAIProvider(Config{APIKey: "k", BaseURL: "http://localhost:11434"}).completionTokens())
}
