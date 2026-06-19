package aiprovider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAzureOpenAIProviderName(t *testing.T) {
	t.Parallel()
	p := NewAzureOpenAIProvider(Config{
		BaseURL: "https://myresource.openai.azure.com",
		APIKey:  "test-key",
		Model:   "gpt-4o",
	})
	assert.Equal(t, AzureOpenAI, p.Name())
}

func TestAzureOpenAIProviderChat(t *testing.T) {
	t.Parallel()
	// Mock server that verifies Azure-specific URL and auth.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify Azure-style URL path.
		assert.Contains(t, r.URL.Path, "/openai/deployments/gpt-4o/chat/completions")
		assert.Contains(t, r.URL.RawQuery, "api-version=")

		// Verify api-key header (not Bearer token).
		assert.Equal(t, "test-key", r.Header.Get("api-key"))
		assert.Empty(t, r.Header.Get("Authorization"))

		resp := openaiResponse{
			Choices: []openaiChoice{{Message: openaiMessage{Content: "Hello!"}}},
			Model:   "gpt-4o",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := NewAzureOpenAIProvider(Config{
		BaseURL: srv.URL,
		APIKey:  "test-key",
		Model:   "gpt-4o",
	})

	resp, err := p.Chat(t.Context(), []Message{
		TextMessage("user", "Hi"),
	})
	require.NoError(t, err)
	assert.Equal(t, "Hello!", resp.Content)
	assert.Equal(t, "gpt-4o", resp.Model)
}

func TestAzureOpenAIProviderTranslate(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openaiResponse{
			Choices: []openaiChoice{{Message: openaiMessage{Content: "Bonjour"}}},
			Model:   "gpt-4o",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := NewAzureOpenAIProvider(Config{
		BaseURL: srv.URL,
		APIKey:  "test-key",
		Model:   "gpt-4o",
	})

	resp, err := p.Translate(t.Context(), TranslateRequest{
		Source:         "Hello",
		SourceLanguage: "en",
		TargetLocale:   "fr",
	})
	require.NoError(t, err)
	assert.Equal(t, "Bonjour", resp.Translation)
}

func TestAzureOpenAIProviderTokenAuth(t *testing.T) {
	t.Parallel()
	// Mock server that verifies Bearer token auth (not api-key).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/openai/deployments/gpt-4o/chat/completions")
		assert.Equal(t, "Bearer test-managed-token", r.Header.Get("Authorization"))
		assert.Empty(t, r.Header.Get("api-key"))

		resp := openaiResponse{
			Choices: []openaiChoice{{Message: openaiMessage{Content: "Hei"}}},
			Model:   "gpt-4o",
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	tp := func(ctx context.Context) (string, error) {
		return "test-managed-token", nil
	}
	p := NewAzureOpenAITokenProvider(srv.URL, "gpt-4o", tp)

	resp, err := p.Chat(t.Context(), []Message{
		TextMessage("user", "Translate Hello to Norwegian"),
	})
	require.NoError(t, err)
	assert.Equal(t, "Hei", resp.Content)
}

func TestAzureOpenAIProviderClose(t *testing.T) {
	t.Parallel()
	p := NewAzureOpenAIProvider(Config{BaseURL: "https://test.openai.azure.com", Model: "gpt-4o"})
	require.NoError(t, p.Close())
}
