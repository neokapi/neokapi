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

// OpenAIProvider implements LLMProvider for OpenAI-compatible APIs.
type OpenAIProvider struct {
	config Config
	client *http.Client
}

// NewOpenAIProvider creates a new OpenAI provider.
func NewOpenAIProvider(cfg Config) *OpenAIProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com"
	}
	if cfg.Model == "" {
		cfg.Model = "gpt-4o"
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 4096
	}
	return &OpenAIProvider{
		config: cfg,
		client: httputil.NewResilientClient(),
	}
}

func (p *OpenAIProvider) Name() ProviderID { return OpenAI }

func (p *OpenAIProvider) Translate(ctx context.Context, req TranslateRequest) (*TranslateResponse, error) {
	var prompt strings.Builder
	prompt.WriteString(fmt.Sprintf(
		"Translate the following text from %s to %s. Return ONLY the translation, no explanation.\n\nText: %s",
		req.SourceLanguage, req.TargetLocale, req.Source,
	))

	prompt.WriteString(req.Directives())

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
		Usage:       resp.Usage,
	}, nil
}

func (p *OpenAIProvider) Chat(ctx context.Context, messages []Message) (*ChatResponse, error) {
	apiMessages := make([]openaiMessage, len(messages))
	for i, m := range messages {
		apiMessages[i] = openaiMessage(m)
	}

	body := openaiRequest{
		Model:    p.config.Model,
		Messages: apiMessages,
	}
	if p.config.MaxTokens > 0 {
		body.MaxTokens = &p.config.MaxTokens
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("openai: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.config.BaseURL+"/v1/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("openai: create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.config.APIKey)

	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai: request: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("openai: read response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai: API error %d: %s", httpResp.StatusCode, string(respBody))
	}

	var apiResp openaiResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("openai: unmarshal response: %w", err)
	}

	if len(apiResp.Choices) == 0 {
		return nil, errors.New("openai: no choices in response")
	}

	return &ChatResponse{
		Content: apiResp.Choices[0].Message.Content,
		Model:   apiResp.Model,
		Usage:   apiResp.Usage.toTokenUsage(),
	}, nil
}

func (p *OpenAIProvider) ChatStructured(ctx context.Context, messages []Message, schema JSONSchema) (*ChatResponse, error) {
	apiMessages := make([]openaiMessage, len(messages))
	for i, m := range messages {
		apiMessages[i] = openaiMessage(m)
	}

	body := openaiRequest{
		Model:    p.config.Model,
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
		return nil, fmt.Errorf("openai: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.config.BaseURL+"/v1/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("openai: create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.config.APIKey)

	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai: request: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("openai: read response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai: API error %d: %s", httpResp.StatusCode, string(respBody))
	}

	var apiResp openaiResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("openai: unmarshal response: %w", err)
	}

	if len(apiResp.Choices) == 0 {
		return nil, errors.New("openai: no choices in response")
	}

	return &ChatResponse{
		Content: apiResp.Choices[0].Message.Content,
		Model:   apiResp.Model,
		Usage:   apiResp.Usage.toTokenUsage(),
	}, nil
}

func (p *OpenAIProvider) Close() error { return nil }

// OpenAI API types
type openaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openaiRequest struct {
	Model          string                `json:"model"`
	Messages       []openaiMessage       `json:"messages"`
	MaxTokens      *int                  `json:"max_tokens,omitempty"`
	ResponseFormat *openaiResponseFormat `json:"response_format,omitempty"`
}

type openaiResponseFormat struct {
	Type       string               `json:"type"` // "json_schema"
	JSONSchema *openaiJSONSchemaRef `json:"json_schema,omitempty"`
}

type openaiJSONSchemaRef struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Schema      map[string]any `json:"schema"`
	Strict      bool           `json:"strict"`
}

type openaiResponse struct {
	Choices []openaiChoice `json:"choices"`
	Model   string         `json:"model"`
	Usage   openaiUsage    `json:"usage"`
}

type openaiUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

func (u openaiUsage) toTokenUsage() TokenUsage {
	return TokenUsage{
		InputTokens:  u.PromptTokens,
		OutputTokens: u.CompletionTokens,
	}
}

type openaiChoice struct {
	Message openaiMessage `json:"message"`
}
