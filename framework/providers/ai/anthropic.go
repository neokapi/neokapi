package provider

import (
	"bytes"
	"context"
	"encoding/json"
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

func (p *AnthropicProvider) Name() string { return "anthropic" }

func (p *AnthropicProvider) Translate(ctx context.Context, req TranslateRequest) (*TranslateResponse, error) {
	var prompt strings.Builder
	prompt.WriteString(fmt.Sprintf(
		"Translate the following text from %s to %s. Return ONLY the translation, no explanation.\n\nText: %s",
		req.SourceLanguage, req.TargetLocale, req.Source,
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

func (p *AnthropicProvider) Chat(ctx context.Context, messages []Message) (*ChatResponse, error) {
	apiMessages := make([]anthropicMessage, len(messages))
	for i, m := range messages {
		apiMessages[i] = anthropicMessage(m)
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

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.config.BaseURL+"/v1/messages", bytes.NewReader(jsonBody))
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
	}, nil
}

func (p *AnthropicProvider) ChatStructured(ctx context.Context, messages []Message, schema JSONSchema) (*ChatResponse, error) {
	apiMessages := make([]anthropicMessage, len(messages))
	for i, m := range messages {
		apiMessages[i] = anthropicMessage(m)
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

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.config.BaseURL+"/v1/messages", bytes.NewReader(jsonBody))
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
			}, nil
		}
	}

	return nil, fmt.Errorf("anthropic: no tool_use block in structured response")
}

func (p *AnthropicProvider) Close() error { return nil }

// Anthropic API types
type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
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
}

type anthropicContentBlock struct {
	Type  string `json:"type"`            // "text" or "tool_use"
	Text  string `json:"text,omitempty"`  // for "text" type
	ID    string `json:"id,omitempty"`    // for "tool_use" type
	Name  string `json:"name,omitempty"`  // for "tool_use" type
	Input any    `json:"input,omitempty"` // for "tool_use" type
}
