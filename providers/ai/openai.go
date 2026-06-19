package aiprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

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

func (p *OpenAIProvider) InputModalities() []Modality { return []Modality{ModalityImage} }

func (p *OpenAIProvider) Translate(ctx context.Context, req TranslateRequest) (*TranslateResponse, error) {
	return standardTranslate(ctx, p.Chat, req, 0.85)
}

func (p *OpenAIProvider) Chat(ctx context.Context, messages []Message) (*ChatResponse, error) {
	apiMessages, err := toOpenAIMessages(messages)
	if err != nil {
		return nil, err
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
	apiMessages, err := toOpenAIMessages(messages)
	if err != nil {
		return nil, err
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

// openaiMessage is the response-side message (content is always a string).
type openaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openaiReqMessage is the request-side message: Content is a plain string for
// text-only messages, or an array of parts when media is present.
type openaiReqMessage struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type openaiContentPart struct {
	Type     string          `json:"type"` // "text" | "image_url"
	Text     string          `json:"text,omitempty"`
	ImageURL *openaiImageURL `json:"image_url,omitempty"`
}

type openaiImageURL struct {
	URL string `json:"url"` // data: URL or http(s) URL
}

func toOpenAIMessages(messages []Message) ([]openaiReqMessage, error) {
	out := make([]openaiReqMessage, len(messages))
	for i, m := range messages {
		if !hasMedia(m.Parts) {
			out[i] = openaiReqMessage{Role: m.Role, Content: m.Text()}
			continue
		}
		parts := make([]openaiContentPart, 0, len(m.Parts))
		for _, part := range m.Parts {
			switch part.Kind {
			case ContentText:
				parts = append(parts, openaiContentPart{Type: "text", Text: part.Text})
			case ContentImage:
				durl, err := resolveMediaDataURL(part.Media)
				if err != nil {
					return nil, fmt.Errorf("openai: %w", err)
				}
				parts = append(parts, openaiContentPart{Type: "image_url", ImageURL: &openaiImageURL{URL: durl}})
			default:
				return nil, fmt.Errorf("openai: unsupported content kind %q (provider accepts text, image)", part.Kind)
			}
		}
		out[i] = openaiReqMessage{Role: m.Role, Content: parts}
	}
	return out, nil
}

type openaiRequest struct {
	Model          string                `json:"model"`
	Messages       []openaiReqMessage    `json:"messages"`
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
