package aiprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/neokapi/neokapi/core/httputil"
)

// DefaultAnthropicModel is the model a fresh provider uses when the caller names
// none. The registry advertises this same constant, so what `kapi models` shows a
// user and what they actually get cannot drift apart — they did, and the registry
// went on advertising a model that had been retired.
const DefaultAnthropicModel = "claude-sonnet-5"

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
		cfg.Model = DefaultAnthropicModel
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
	return standardTranslate(ctx, p.Name(), p.Chat, req, 0.85)
}

// Limits reports what the configured model can emit. Unknown models fall back to
// a conservative ceiling, so an unrecognized model yields smaller batches rather
// than truncated replies.
func (p *AnthropicProvider) Limits() Limits {
	if l, ok := LimitsForModel(p.config.Model); ok {
		return l
	}
	return Limits{MaxOutputTokens: ConservativeMaxOutputTokens}
}

// buildRequest assembles the wire request. System-role messages are lifted into
// the top-level system parameter: the Messages API accepts only user and
// assistant roles and rejects a "system" role inside messages[].
//
// max_tokens is the *caller's* budget for this request, clamped to the model's
// ceiling — not a fixed constant. A batch asks for what it could need; asking
// for a flat 4096 is how a large batch came back truncated.
func (p *AnthropicProvider) buildRequest(ctx context.Context, messages []Message) (anthropicRequest, error) {
	system, convo := SplitSystem(messages)
	apiMessages, err := toAnthropicMessages(convo)
	if err != nil {
		return anthropicRequest{}, err
	}
	return anthropicRequest{
		Model:       p.config.Model,
		MaxTokens:   p.maxTokens(ctx),
		Temperature: p.config.Temperature,
		System:      system,
		Messages:    apiMessages,
	}, nil
}

// maxTokens resolves this request's output budget: what the caller asked for,
// clamped to what the model can actually emit, falling back to the configured
// default.
func (p *AnthropicProvider) maxTokens(ctx context.Context) int {
	ceiling := p.Limits().EffectiveMaxOutputTokens()
	want := maxOutputTokensFrom(ctx, p.config.MaxTokens)
	if want <= 0 || want > ceiling {
		return ceiling
	}
	return want
}

// post sends a request to /v1/messages and decodes the response.
func (p *AnthropicProvider) post(ctx context.Context, body anthropicRequest) (*anthropicResponse, error) {
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

	respBody, err := readCappedBody(httpResp.Body)
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
	return &apiResp, nil
}

func (p *AnthropicProvider) Chat(ctx context.Context, messages []Message) (*ChatResponse, error) {
	body, err := p.buildRequest(ctx, messages)
	if err != nil {
		return nil, err
	}

	apiResp, err := p.post(ctx, body)
	if err != nil {
		return nil, err
	}

	var content strings.Builder
	for _, block := range apiResp.Content {
		if block.Type == "text" {
			content.WriteString(block.Text)
		}
	}

	return &ChatResponse{
		Content:   content.String(),
		Model:     apiResp.Model,
		Usage:     apiResp.Usage.toTokenUsage(),
		Truncated: apiResp.truncated(),
	}, nil
}

func (p *AnthropicProvider) ChatStructured(ctx context.Context, messages []Message, schema JSONSchema) (*ChatResponse, error) {
	body, err := p.buildRequest(ctx, messages)
	if err != nil {
		return nil, err
	}

	toolName := schema.Name
	if toolName == "" {
		toolName = "structured_output"
	}
	body.Tools = []anthropicTool{{
		Name:        toolName,
		Description: schema.Description,
		InputSchema: schema.Schema,
	}}
	body.ToolChoice = &anthropicToolChoice{Type: "tool", Name: toolName}

	apiResp, err := p.post(ctx, body)
	if err != nil {
		return nil, err
	}

	// Extract the tool_use input as JSON.
	for _, block := range apiResp.Content {
		if block.Type == "tool_use" && block.Name == toolName {
			inputJSON, err := json.Marshal(block.Input)
			if err != nil {
				return nil, fmt.Errorf("anthropic: marshal tool input: %w", err)
			}
			return &ChatResponse{
				Content:   string(inputJSON),
				Model:     apiResp.Model,
				Usage:     apiResp.Usage.toTokenUsage(),
				Truncated: apiResp.truncated(),
			}, nil
		}
	}

	// A run that asked for more than the model could emit lands here with no
	// tool_use block at all. Say which of the two happened — the caller can
	// shrink a batch, but it cannot fix a malformed response.
	if apiResp.truncated() {
		return nil, fmt.Errorf("anthropic: %w (max_tokens=%d)", ErrOutputTruncated, body.MaxTokens)
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
	Model     string `json:"model"`
	MaxTokens int    `json:"max_tokens"`
	// Temperature is omitted when the caller did not set one, which leaves the
	// API's own default in place. It used to be omitted always.
	Temperature *float64             `json:"temperature,omitempty"`
	System      string               `json:"system,omitempty"`
	Messages    []anthropicMessage   `json:"messages"`
	Tools       []anthropicTool      `json:"tools,omitempty"`
	ToolChoice  *anthropicToolChoice `json:"tool_choice,omitempty"`
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
	Content    []anthropicContentBlock `json:"content"`
	Model      string                  `json:"model"`
	StopReason string                  `json:"stop_reason"`
	Usage      anthropicUsage          `json:"usage"`
}

// truncated reports that the model ran into the output cap mid-reply.
func (r *anthropicResponse) truncated() bool { return r.StopReason == "max_tokens" }

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
