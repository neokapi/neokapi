package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
		client: &http.Client{},
	}
}

func (p *OpenAIProvider) Name() string { return "openai" }

func (p *OpenAIProvider) Translate(ctx context.Context, req TranslateRequest) (*TranslateResponse, error) {
	prompt := fmt.Sprintf(
		"Translate the following text from %s to %s. Return ONLY the translation, no explanation.\n\nText: %s",
		req.SourceLocale, req.TargetLocale, req.Source,
	)

	if len(req.Glossary) > 0 {
		prompt += "\n\nGlossary:\n"
		for term, translation := range req.Glossary {
			prompt += fmt.Sprintf("- %s → %s\n", term, translation)
		}
	}

	resp, err := p.Chat(ctx, []Message{
		{Role: "user", Content: prompt},
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

func (p *OpenAIProvider) Chat(ctx context.Context, messages []Message) (*ChatResponse, error) {
	apiMessages := make([]openaiMessage, len(messages))
	for i, m := range messages {
		apiMessages[i] = openaiMessage{
			Role:    m.Role,
			Content: m.Content,
		}
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

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.config.BaseURL+"/v1/chat/completions", bytes.NewReader(jsonBody))
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
		return nil, fmt.Errorf("openai: no choices in response")
	}

	return &ChatResponse{
		Content: apiResp.Choices[0].Message.Content,
		Model:   apiResp.Model,
	}, nil
}

func (p *OpenAIProvider) Close() error { return nil }

// OpenAI API types
type openaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openaiRequest struct {
	Model     string          `json:"model"`
	Messages  []openaiMessage `json:"messages"`
	MaxTokens *int            `json:"max_tokens,omitempty"`
}

type openaiResponse struct {
	Choices []openaiChoice `json:"choices"`
	Model   string         `json:"model"`
}

type openaiChoice struct {
	Message openaiMessage `json:"message"`
}
