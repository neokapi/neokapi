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

// AnthropicProvider implements LLMProvider for Anthropic Claude API.
type AnthropicProvider struct {
	config Config
	client *http.Client
}

// NewAnthropicProvider creates a new Anthropic provider.
func NewAnthropicProvider(cfg Config) *AnthropicProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.anthropic.com"
	}
	if cfg.Model == "" {
		cfg.Model = "claude-sonnet-4-20250514"
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 4096
	}
	return &AnthropicProvider{
		config: cfg,
		client: httputil.NewResilientClient(),
	}
}

func (p *AnthropicProvider) Name() ProviderID { return Anthropic }

func (p *AnthropicProvider) InputModalities() []Modality { return []Modality{ModalityImage} }

func (p *AnthropicProvider) Translate(ctx context.Context, req TranslateRequest) (*TranslateResponse, error) {
	return standardTranslate(ctx, p.Chat, req, 0.85)
}

func (p *AnthropicProvider) Chat(ctx context.Context, messages []Message) (*ChatResponse, error) {
	apiMessages, err := toAnthropicMessages(messages)
	if err != nil {
		return nil, err
	}

	body := anthropicRequest{
		Model:     p.config.Model,
		MaxTokens: p.config.MaxTokens,
		Messages:  apiMessages,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("anthropic: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.config.BaseURL+"/v1/messages", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("anthropic: create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.config.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic: request: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("anthropic: read response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic: API error %d: %s", httpResp.StatusCode, string(respBody))
	}

	var apiResp anthropicResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("anthropic: unmarshal response: %w", err)
	}

	var content strings.Builder
	for _, block := range apiResp.Content {
		if block.Type == "text" {
			content.WriteString(block.Text)
		}
	}

	return &ChatResponse{
		Content: content.String(),
		Model:   apiResp.Model,
		Usage:   apiResp.Usage.toTokenUsage(),
	}, nil
}

func (p *AnthropicProvider) ChatStructured(ctx context.Context, messages []Message, schema JSONSchema) (*ChatResponse, error) {
	apiMessages, err := toAnthropicMessages(messages)
	if err != nil {
		return nil, err
	}

	toolName := schema.Name
	if toolName == "" {
		toolName = "structured_output"
	}

	body := anthropicRequest{
		Model:     p.config.Model,
		MaxTokens: p.config.MaxTokens,
		Messages:  apiMessages,
		Tools: []anthropicTool{{
			Name:        toolName,
			Description: schema.Description,
			InputSchema: schema.Schema,
		}},
		ToolChoice: &anthropicToolChoice{
			Type: "tool",
			Name: toolName,
		},
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("anthropic: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.config.BaseURL+"/v1/messages", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("anthropic: create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.config.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic: request: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("anthropic: read response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic: API error %d: %s", httpResp.StatusCode, string(respBody))
	}

	var apiResp anthropicResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("anthropic: unmarshal response: %w", err)
	}

	usage := apiResp.Usage.toTokenUsage()

	// Extract the tool_use input as JSON.
	for _, block := range apiResp.Content {
		if block.Type == "tool_use" && block.Name == toolName {
			inputJSON, err := json.Marshal(block.Input)
			if err != nil {
				return nil, fmt.Errorf("anthropic: marshal tool input: %w", err)
			}
			return &ChatResponse{
				Content: string(inputJSON),
				Model:   apiResp.Model,
				Usage:   usage,
			}, nil
		}
	}

	return nil, errors.New("anthropic: no tool_use block in structured response")
}

func (p *AnthropicProvider) Close() error { return nil }

// Anthropic API types
type anthropicMessage struct {
	Role    string              `json:"role"`
	Content []anthropicReqBlock `json:"content"`
}

type anthropicReqBlock struct {
	Type   string                `json:"type"`             // "text" | "image"
	Text   string                `json:"text,omitempty"`   // type == "text"
	Source *anthropicImageSource `json:"source,omitempty"` // type == "image"
}

type anthropicImageSource struct {
	Type      string `json:"type"`       // "base64"
	MediaType string `json:"media_type"` // e.g. "image/png"
	Data      string `json:"data"`       // base64-encoded bytes
}

func toAnthropicMessages(messages []Message) ([]anthropicMessage, error) {
	out := make([]anthropicMessage, len(messages))
	for i, m := range messages {
		blocks := make([]anthropicReqBlock, 0, len(m.Parts))
		for _, part := range m.Parts {
			switch part.Kind {
			case ContentText:
				blocks = append(blocks, anthropicReqBlock{Type: "text", Text: part.Text})
			case ContentImage:
				b64, mime, err := resolveMediaBase64(part.Media)
				if err != nil {
					return nil, fmt.Errorf("anthropic: %w", err)
				}
				blocks = append(blocks, anthropicReqBlock{
					Type:   "image",
					Source: &anthropicImageSource{Type: "base64", MediaType: mime, Data: b64},
				})
			default:
				return nil, fmt.Errorf("anthropic: unsupported content kind %q (provider accepts text, image)", part.Kind)
			}
		}
		out[i] = anthropicMessage{Role: m.Role, Content: blocks}
	}
	return out, nil
}

type anthropicRequest struct {
	Model      string               `json:"model"`
	MaxTokens  int                  `json:"max_tokens"`
	Messages   []anthropicMessage   `json:"messages"`
	Tools      []anthropicTool      `json:"tools,omitempty"`
	ToolChoice *anthropicToolChoice `json:"tool_choice,omitempty"`
}

type anthropicTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	InputSchema map[string]any `json:"input_schema"`
}

type anthropicToolChoice struct {
	Type string `json:"type"` // "tool"
	Name string `json:"name"`
}

type anthropicResponse struct {
	Content []anthropicContentBlock `json:"content"`
	Model   string                  `json:"model"`
	Usage   anthropicUsage          `json:"usage"`
}

type anthropicUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

func (u anthropicUsage) toTokenUsage() TokenUsage {
	return TokenUsage{
		InputTokens:         u.InputTokens,
		OutputTokens:        u.OutputTokens,
		CacheCreationTokens: u.CacheCreationInputTokens,
		CacheReadTokens:     u.CacheReadInputTokens,
	}
}

type anthropicContentBlock struct {
	Type  string `json:"type"`            // "text" or "tool_use"
	Text  string `json:"text,omitempty"`  // for "text" type
	ID    string `json:"id,omitempty"`    // for "tool_use" type
	Name  string `json:"name,omitempty"`  // for "tool_use" type
	Input any    `json:"input,omitempty"` // for "tool_use" type
}
