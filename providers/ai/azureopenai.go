package aiprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/neokapi/neokapi/core/httputil"
)

const defaultAzureAPIVersion = "2024-10-21"

// TokenProvider is a function that returns a Bearer token for Azure OpenAI authentication.
// When set on AzureOpenAIProvider, Bearer token auth is used instead of API key auth.
// This enables managed identity authentication without depending on the Azure SDK.
type TokenProvider func(ctx context.Context) (string, error)

// AzureOpenAIProvider implements LLMProvider for Azure OpenAI Service.
// Azure OpenAI uses the same request/response format as OpenAI but with
// different URL structure and authentication.
type AzureOpenAIProvider struct {
	config        Config
	deployment    string // Azure deployment name (defaults to config.Model)
	apiVersion    string
	client        *http.Client
	tokenProvider TokenProvider // optional; if set, Bearer token auth is used instead of api-key
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
		client:     httputil.NewResilientClient(),
	}
}

// NewAzureOpenAITokenProvider creates an Azure OpenAI provider that uses
// a TokenProvider for authentication instead of an API key. This is the
// preferred method for Azure deployments using managed identities.
func NewAzureOpenAITokenProvider(endpoint, deployment string, tp TokenProvider) *AzureOpenAIProvider {
	return &AzureOpenAIProvider{
		config: Config{
			BaseURL:   endpoint,
			Model:     deployment,
			MaxTokens: 4096,
		},
		deployment:    deployment,
		apiVersion:    defaultAzureAPIVersion,
		client:        httputil.NewResilientClient(),
		tokenProvider: tp,
	}
}

func (p *AzureOpenAIProvider) Name() ProviderID { return AzureOpenAI }

func (p *AzureOpenAIProvider) InputModalities() []Modality { return []Modality{ModalityImage} }

func (p *AzureOpenAIProvider) Translate(ctx context.Context, req TranslateRequest) (*TranslateResponse, error) {
	system := fmt.Sprintf(
		"You are a software localization specialist. Your task is to translate user interface strings from %s to %s. "+
			"These are UI labels, error messages, and status texts from a software application. "+
			"Return ONLY the translated text, nothing else. Preserve any placeholders.",
		req.SourceLanguage, req.TargetLocale,
	)

	var user strings.Builder
	user.WriteString(req.Source)
	user.WriteString(req.Directives())

	resp, err := p.Chat(ctx, []Message{
		TextMessage("system", system),
		TextMessage("user", user.String()),
	})
	if err != nil {
		return nil, err
	}

	return &TranslateResponse{
		Translation: resp.Content,
		Confidence:  0.85,
		Model:       resp.Model,
		Usage:       resp.Usage,
	}, nil
}

func (p *AzureOpenAIProvider) Chat(ctx context.Context, messages []Message) (*ChatResponse, error) {
	apiMessages, err := toOpenAIMessages(messages)
	if err != nil {
		return nil, err
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

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("azureopenai: create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if p.tokenProvider != nil {
		token, err := p.tokenProvider(ctx)
		if err != nil {
			return nil, fmt.Errorf("azureopenai: get token: %w", err)
		}
		httpReq.Header.Set("Authorization", "Bearer "+token)
	} else {
		httpReq.Header.Set("api-key", p.config.APIKey)
	}

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
		return nil, errors.New("azureopenai: no choices in response")
	}

	return &ChatResponse{
		Content: apiResp.Choices[0].Message.Content,
		Model:   apiResp.Model,
		Usage:   apiResp.Usage.toTokenUsage(),
	}, nil
}

func (p *AzureOpenAIProvider) ChatStructured(ctx context.Context, messages []Message, schema JSONSchema) (*ChatResponse, error) {
	apiMessages, err := toOpenAIMessages(messages)
	if err != nil {
		return nil, err
	}

	body := openaiRequest{
		Model:    p.deployment,
		Messages: apiMessages,
		ResponseFormat: &openaiResponseFormat{
			Type: "json_schema",
			JSONSchema: &openaiJSONSchemaRef{
				Name:        schema.Name,
				Description: schema.Description,
				Schema:      schema.Schema,
				Strict:      schema.Strict,
			},
		},
	}
	if p.config.MaxTokens > 0 {
		body.MaxTokens = &p.config.MaxTokens
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("azureopenai: marshal request: %w", err)
	}

	endpoint := strings.TrimRight(p.config.BaseURL, "/")
	url := fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=%s",
		endpoint, p.deployment, p.apiVersion)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("azureopenai: create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if p.tokenProvider != nil {
		token, err := p.tokenProvider(ctx)
		if err != nil {
			return nil, fmt.Errorf("azureopenai: get token: %w", err)
		}
		httpReq.Header.Set("Authorization", "Bearer "+token)
	} else {
		httpReq.Header.Set("api-key", p.config.APIKey)
	}

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

	var apiResp openaiResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("azureopenai: unmarshal response: %w", err)
	}

	if len(apiResp.Choices) == 0 {
		return nil, errors.New("azureopenai: no choices in response")
	}

	return &ChatResponse{
		Content: apiResp.Choices[0].Message.Content,
		Model:   apiResp.Model,
		Usage:   apiResp.Usage.toTokenUsage(),
	}, nil
}

func (p *AzureOpenAIProvider) Close() error { return nil }
