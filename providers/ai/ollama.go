package aiprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/neokapi/neokapi/core/httputil"
)

// OllamaProvider implements LLMProvider for local Ollama models.
type OllamaProvider struct {
	config Config
	client *http.Client
}

// NewOllamaProvider creates a new Ollama provider.
func NewOllamaProvider(cfg Config) *OllamaProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "http://localhost:11434"
	}
	if cfg.Model == "" {
		cfg.Model = "llama3"
	}
	return &OllamaProvider{
		config: cfg,
		client: httputil.NewResilientClient(),
	}
}

func (p *OllamaProvider) Name() ProviderID { return Ollama }

func (p *OllamaProvider) Translate(ctx context.Context, req TranslateRequest) (*TranslateResponse, error) {
	return standardTranslate(ctx, p.Chat, req, 0.7)
}

func (p *OllamaProvider) Chat(ctx context.Context, messages []Message) (*ChatResponse, error) {
	ollamaMessages := make([]ollamaMessage, len(messages))
	for i, m := range messages {
		ollamaMessages[i] = ollamaMessage(m)
	}

	body := ollamaChatRequest{
		Model:    p.config.Model,
		Messages: ollamaMessages,
		Stream:   false,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("ollama: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.config.BaseURL+"/api/chat", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("ollama: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama: request: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("ollama: read response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama: API error %d: %s", httpResp.StatusCode, string(respBody))
	}

	var apiResp ollamaChatResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("ollama: unmarshal response: %w", err)
	}

	return &ChatResponse{
		Content: apiResp.Message.Content,
		Model:   apiResp.Model,
		Usage: TokenUsage{
			InputTokens:  apiResp.PromptEvalCount,
			OutputTokens: apiResp.EvalCount,
		},
	}, nil
}

func (p *OllamaProvider) ChatStructured(ctx context.Context, messages []Message, schema JSONSchema) (*ChatResponse, error) {
	ollamaMessages := make([]ollamaMessage, len(messages))
	for i, m := range messages {
		ollamaMessages[i] = ollamaMessage(m)
	}

	body := ollamaChatRequest{
		Model:    p.config.Model,
		Messages: ollamaMessages,
		Stream:   false,
		Format:   schema.Schema,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("ollama: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.config.BaseURL+"/api/chat", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("ollama: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama: request: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("ollama: read response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama: API error %d: %s", httpResp.StatusCode, string(respBody))
	}

	var apiResp ollamaChatResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("ollama: unmarshal response: %w", err)
	}

	return &ChatResponse{
		Content: apiResp.Message.Content,
		Model:   apiResp.Model,
		Usage: TokenUsage{
			InputTokens:  apiResp.PromptEvalCount,
			OutputTokens: apiResp.EvalCount,
		},
	}, nil
}

func (p *OllamaProvider) Close() error { return nil }

// Ollama API types
type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
	Format   any             `json:"format,omitempty"` // JSON schema for structured output
}

type ollamaChatResponse struct {
	Model           string        `json:"model"`
	Message         ollamaMessage `json:"message"`
	PromptEvalCount int           `json:"prompt_eval_count"`
	EvalCount       int           `json:"eval_count"`
}
