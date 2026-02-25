package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const azureAPIVersion = "2024-10-21"

// AzureOpenAIProvider implements LLMProvider for Azure OpenAI Service.
// Azure OpenAI uses deployment-based URLs and api-key header authentication
// instead of the standard OpenAI Bearer token approach.
type AzureOpenAIProvider struct {
	config Config
	client *http.Client
}

// NewAzureOpenAIProvider creates a new Azure OpenAI provider.
// cfg.BaseURL is the Azure endpoint (e.g., "https://my-openai.openai.azure.com").
// cfg.Model is the deployment name (e.g., "gpt-4o").
func NewAzureOpenAIProvider(cfg Config) *AzureOpenAIProvider {
	if cfg.Model == "" {
		cfg.Model = "gpt-4o"
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 4096
	}
	// Trim trailing slash from base URL
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	return &AzureOpenAIProvider{
		config: cfg,
		client: &http.Client{},
	}
}

func (p *AzureOpenAIProvider) Name() string { return "azure_openai" }

func (p *AzureOpenAIProvider) Translate(ctx context.Context, req TranslateRequest) (*TranslateResponse, error) {
	var prompt strings.Builder
	prompt.WriteString(fmt.Sprintf(
		"Translate the following text from %s to %s. Return ONLY the translation, no explanation.\n\nText: %s",
		req.SourceLocale, req.TargetLocale, req.Source,
	))

	if len(req.Glossary) > 0 {
		prompt.WriteString("\n\nGlossary:\n")
		for term, translation := range req.Glossary {
			prompt.WriteString(fmt.Sprintf("- %s → %s\n", term, translation))
		}
	}

	resp, err := p.Chat(ctx, []Message{
		{Role: "user", Content: prompt.String()},
	})
	if err != nil {
		return nil, err
	}

	return &TranslateResponse{
		Translation: resp.Content,
		Confidence:  0.85,
		Model:       resp.Model,
	}, nil
}

func (p *AzureOpenAIProvider) Chat(ctx context.Context, messages []Message) (*ChatResponse, error) {
	apiMessages := make([]openaiMessage, len(messages))
	for i, m := range messages {
		apiMessages[i] = openaiMessage(m)
	}

	// Azure OpenAI does not use the model field in the request body;
	// the deployment name is part of the URL.
	body := openaiRequest{
		Messages: apiMessages,
	}
	if p.config.MaxTokens > 0 {
		body.MaxTokens = &p.config.MaxTokens
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("azure_openai: marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=%s",
		p.config.BaseURL, p.config.Model, azureAPIVersion)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("azure_openai: create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("api-key", p.config.APIKey)

	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("azure_openai: request: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("azure_openai: read response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("azure_openai: API error %d: %s", httpResp.StatusCode, string(respBody))
	}

	var apiResp openaiResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("azure_openai: unmarshal response: %w", err)
	}

	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("azure_openai: no choices in response")
	}

	return &ChatResponse{
		Content: apiResp.Choices[0].Message.Content,
		Model:   apiResp.Model,
	}, nil
}

func (p *AzureOpenAIProvider) Close() error { return nil }
