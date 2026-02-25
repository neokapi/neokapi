package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/gokapi/gokapi/core/model"
)

func TestAzureOpenAIProvider_Name(t *testing.T) {
	p := NewAzureOpenAIProvider(Config{})
	assert.Equal(t, "azure_openai", p.Name())
}

func TestAzureOpenAIProvider_Defaults(t *testing.T) {
	p := NewAzureOpenAIProvider(Config{})
	assert.Equal(t, "gpt-4o", p.config.Model)
	assert.Equal(t, 4096, p.config.MaxTokens)
}

func TestAzureOpenAIProvider_TrailingSlash(t *testing.T) {
	p := NewAzureOpenAIProvider(Config{BaseURL: "https://my-openai.openai.azure.com/"})
	assert.Equal(t, "https://my-openai.openai.azure.com", p.config.BaseURL)
}

func TestAzureOpenAIProvider_Chat(t *testing.T) {
	var gotHeaders http.Header
	var gotPath string
	var gotQuery string
	var gotBody openaiRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery

		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		resp := openaiResponse{
			Model: "gpt-4o",
			Choices: []openaiChoice{
				{Message: openaiMessage{Role: "assistant", Content: "Hello from Azure!"}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := NewAzureOpenAIProvider(Config{
		BaseURL: srv.URL,
		Model:   "my-gpt4o-deployment",
		APIKey:  "test-azure-key",
	})

	resp, err := p.Chat(context.Background(), []Message{
		{Role: "user", Content: "Hello"},
	})
	require.NoError(t, err)

	// Verify response
	assert.Equal(t, "Hello from Azure!", resp.Content)
	assert.Equal(t, "gpt-4o", resp.Model)

	// Verify URL construction: deployment name in path
	assert.Equal(t, "/openai/deployments/my-gpt4o-deployment/chat/completions", gotPath)

	// Verify api-version query parameter
	assert.Equal(t, "api-version=2024-10-21", gotQuery)

	// Verify api-key header (not Bearer token)
	assert.Equal(t, "test-azure-key", gotHeaders.Get("api-key"))
	assert.Empty(t, gotHeaders.Get("Authorization"))

	// Verify model is NOT in the request body
	assert.Empty(t, gotBody.Model)
}

func TestAzureOpenAIProvider_ChatError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid key"}}`))
	}))
	defer srv.Close()

	p := NewAzureOpenAIProvider(Config{
		BaseURL: srv.URL,
		Model:   "deployment",
		APIKey:  "bad-key",
	})

	_, err := p.Chat(context.Background(), []Message{
		{Role: "user", Content: "Hello"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "API error 401")
}

func TestAzureOpenAIProvider_Translate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body openaiRequest
		_ = json.NewDecoder(r.Body).Decode(&body)

		// Verify the prompt contains source and target locale
		content := body.Messages[0].Content
		assert.Contains(t, content, "en-US")
		assert.Contains(t, content, "fr-FR")
		assert.Contains(t, content, "Hello world")

		resp := openaiResponse{
			Model: "gpt-4o",
			Choices: []openaiChoice{
				{Message: openaiMessage{Role: "assistant", Content: "Bonjour le monde"}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := NewAzureOpenAIProvider(Config{
		BaseURL: srv.URL,
		Model:   "gpt-4o",
		APIKey:  "test-key",
	})

	resp, err := p.Translate(context.Background(), TranslateRequest{
		Source:       "Hello world",
		SourceLocale: model.LocaleID("en-US"),
		TargetLocale: model.LocaleID("fr-FR"),
	})
	require.NoError(t, err)
	assert.Equal(t, "Bonjour le monde", resp.Translation)
	assert.Equal(t, 0.85, resp.Confidence)
	assert.Equal(t, "gpt-4o", resp.Model)
}

func TestAzureOpenAIProvider_NoChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := openaiResponse{Model: "gpt-4o", Choices: []openaiChoice{}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := NewAzureOpenAIProvider(Config{
		BaseURL: srv.URL,
		Model:   "deployment",
		APIKey:  "key",
	})

	_, err := p.Chat(context.Background(), []Message{
		{Role: "user", Content: "Hello"},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no choices in response")
}
