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

const defaultAzureAPIVersion = "2024-10-21"

// AzureOpenAIProvider implements LLMProvider for Azure OpenAI Service.
// Azure OpenAI uses the same request/response format as OpenAI but with
// different URL structure and authentication.
type AzureOpenAIProvider struct {
	config     Config
	deployment string // Azure deployment name (defaults to config.Model)
	apiVersion string
	client     *http.Client
}

// NewAzureOpenAIProvider creates a new Azure OpenAI provider.
// config.BaseURL should be the Azure endpoint (e.g. https://myresource.openai.azure.com).
// config.Model is used as the deployment name.
func NewAzureOpenAIProvider(cfg Config) *AzureOpenAIProvider {
	if cfg.Model == "" {
		cfg.Model = "gpt-4o"
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 4096
	}
	return &AzureOpenAIProvider{
		config:     cfg,
		deployment: cfg.Model,
		apiVersion: defaultAzureAPIVersion,
		client:     &http.Client{},
	}
}

func (p *AzureOpenAIProvider) Name() string { return "azureopenai" }

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

	body := openaiRequest{
		Model:    p.deployment,
		Messages: apiMessages,
	}
	if p.config.MaxTokens > 0 {
		body.MaxTokens = &p.config.MaxTokens
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("azureopenai: marshal request: %w", err)
	}

	// Azure OpenAI URL format:
	// https://{resource}.openai.azure.com/openai/deployments/{deployment}/chat/completions?api-version={version}
	endpoint := strings.TrimRight(p.config.BaseURL, "/")
	url := fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=%s",
		endpoint, p.deployment, p.apiVersion)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("azureopenai: create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("api-key", p.config.APIKey)

	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("azureopenai: request: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("azureopenai: read response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("azureopenai: API error %d: %s", httpResp.StatusCode, string(respBody))
	}

	// Azure OpenAI returns the same response format as OpenAI.
	var apiResp openaiResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("azureopenai: unmarshal response: %w", err)
	}

	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("azureopenai: no choices in response")
	}

	return &ChatResponse{
		Content: apiResp.Choices[0].Message.Content,
		Model:   apiResp.Model,
	}, nil
}

func (p *AzureOpenAIProvider) Close() error { return nil }
