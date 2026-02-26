package provider

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
	p := NewAzureOpenAIProvider(Config{
		BaseURL: "https://myresource.openai.azure.com",
		APIKey:  "test-key",
		Model:   "gpt-4o",
	})
	assert.Equal(t, "azureopenai", p.Name())
}

func TestAzureOpenAIProviderChat(t *testing.T) {
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

	resp, err := p.Chat(context.Background(), []Message{
		{Role: "user", Content: "Hi"},
	})
	require.NoError(t, err)
	assert.Equal(t, "Hello!", resp.Content)
	assert.Equal(t, "gpt-4o", resp.Model)
}

func TestAzureOpenAIProviderTranslate(t *testing.T) {
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

	resp, err := p.Translate(context.Background(), TranslateRequest{
		Source:       "Hello",
		SourceLocale: "en",
		TargetLocale: "fr",
	})
	require.NoError(t, err)
	assert.Equal(t, "Bonjour", resp.Translation)
}

func TestAzureOpenAIProviderClose(t *testing.T) {
	p := NewAzureOpenAIProvider(Config{BaseURL: "https://test.openai.azure.com", Model: "gpt-4o"})
	assert.NoError(t, p.Close())
}
