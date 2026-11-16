package aiprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync/atomic"
)

// DefaultOpenAIModel is the model a fresh provider uses when the caller names none.
const DefaultOpenAIModel = "gpt-6.1-terra"

// OpenAIProvider implements LLMProvider for OpenAI-compatible APIs.
type OpenAIProvider struct {
	config Config
	client *http.Client

	// tokensParam and omitTemperature record what the endpoint has refused,
	// so only the first request after a refusal pays for the retry.
	tokensParam     atomic.Int32
	omitTemperature atomic.Bool
}

// Values of OpenAIProvider.tokensParam. Zero leaves the choice to the endpoint.
const (
	tokensParamCompletion int32 = 1 + iota
	tokensParamLegacy
)

// NewOpenAIProvider creates a new OpenAI provider.
func NewOpenAIProvider(cfg Config) *OpenAIProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.openai.com"
	}
	if cfg.Model == "" {
		cfg.Model = DefaultOpenAIModel
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 4096
	}
	return &OpenAIProvider{
		config: cfg,
		client: newChatClient(),
	}
}

func (p *OpenAIProvider) Name() ProviderID { return OpenAI }

func (p *OpenAIProvider) InputModalities() []Modality { return []Modality{ModalityImage} }

func (p *OpenAIProvider) Translate(ctx context.Context, req TranslateRequest) (*TranslateResponse, error) {
	return StandardTranslate(ctx, p.Name(), p.Chat, req, 0.85)
}

func (p *OpenAIProvider) Chat(ctx context.Context, messages []Message) (*ChatResponse, error) {
	apiMessages, err := toOpenAIMessages(messages)
	if err != nil {
		return nil, err
	}
	return p.complete(ctx, openaiRequest{
		Model:       p.config.Model,
		Temperature: p.config.Temperature,
		Messages:    apiMessages,
	})
}

func (p *OpenAIProvider) ChatStructured(ctx context.Context, messages []Message, schema JSONSchema) (*ChatResponse, error) {
	apiMessages, err := toOpenAIMessages(messages)
	if err != nil {
		return nil, err
	}
	return p.complete(ctx, openaiRequest{
		Model:       p.config.Model,
		Temperature: p.config.Temperature,
		Messages:    apiMessages,
		ResponseFormat: &openaiResponseFormat{
			Type: "json_schema",
			JSONSchema: &openaiJSONSchemaRef{
				Name:        schema.Name,
				Description: schema.Description,
				Schema:      schema.Schema,
				Strict:      schema.Strict,
			},
		},
	})
}

// complete sends one chat completion. OpenAI's current models take the output
// budget as max_completion_tokens and accept only the default temperature, while
// OpenAI-compatible servers behind a custom BaseURL often know only max_tokens.
// The provider starts from the form its endpoint is most likely to accept, and
// when the API refuses a parameter by name it retries once without it and keeps
// that form for later requests.
func (p *OpenAIProvider) complete(ctx context.Context, body openaiRequest) (*ChatResponse, error) {
	for attempt := 0; ; attempt++ {
		req := body
		if n := p.maxTokens(ctx); n > 0 {
			if p.completionTokens() {
				req.MaxCompletionTokens = &n
			} else {
				req.MaxTokens = &n
			}
		}
		if p.omitTemperature.Load() {
			req.Temperature = nil
		}
		resp, status, respBody, err := p.post(ctx, req)
		if err != nil {
			return nil, err
		}
		if status == http.StatusBadRequest && attempt == 0 && p.adapt(respBody, req) {
			continue
		}
		if status != http.StatusOK {
			return nil, fmt.Errorf("openai: API error %d: %s", status, string(respBody))
		}
		return resp, nil
	}
}

// completionTokens reports whether requests carry max_completion_tokens rather
// than max_tokens: the default for OpenAI's own endpoint, and for any endpoint
// that has refused max_tokens.
func (p *OpenAIProvider) completionTokens() bool {
	switch p.tokensParam.Load() {
	case tokensParamCompletion:
		return true
	case tokensParamLegacy:
		return false
	}
	return p.config.BaseURL == "https://api.openai.com"
}

// adapt reads a 400 response and, when it names a parameter this request sent
// that the model does not take, switches the provider away from it. It reports
// whether anything changed, so the caller retries only then.
func (p *OpenAIProvider) adapt(respBody []byte, sent openaiRequest) bool {
	var e struct {
		Error struct {
			Param string `json:"param"`
			Code  string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal(respBody, &e) != nil {
		return false
	}
	switch e.Error.Param {
	case "max_tokens":
		if sent.MaxTokens != nil {
			p.tokensParam.Store(tokensParamCompletion)
			return true
		}
	case "max_completion_tokens":
		if sent.MaxCompletionTokens != nil {
			p.tokensParam.Store(tokensParamLegacy)
			return true
		}
	case "temperature":
		if sent.Temperature != nil {
			p.omitTemperature.Store(true)
			return true
		}
	}
	return false
}

// post sends the request and decodes a 200 response. A non-200 status comes
// back with its body and no error, so the caller can decide whether to adapt.
func (p *OpenAIProvider) post(ctx context.Context, body openaiRequest) (*ChatResponse, int, []byte, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("openai: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.config.BaseURL+"/v1/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, 0, nil, fmt.Errorf("openai: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.config.APIKey)

	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("openai: request: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := readCappedBody(httpResp.Body)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("openai: read response: %w", err)
	}
	if httpResp.StatusCode != http.StatusOK {
		return nil, httpResp.StatusCode, respBody, nil
	}

	var apiResp openaiResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, 0, nil, fmt.Errorf("openai: unmarshal response: %w", err)
	}
	if len(apiResp.Choices) == 0 {
		return nil, 0, nil, errors.New("openai: no choices in response")
	}
	return &ChatResponse{
		Content:   apiResp.Choices[0].Message.Content,
		Model:     apiResp.Model,
		Usage:     apiResp.Usage.toTokenUsage(),
		Truncated: apiResp.truncated(),
	}, http.StatusOK, nil, nil
}

// Limits reports what the configured model can emit.
func (p *OpenAIProvider) Limits() Limits {
	if l, ok := LimitsForModel(p.config.Model); ok {
		return l
	}
	return Limits{MaxOutputTokens: ConservativeMaxOutputTokens}
}

// maxTokens resolves this request's output budget, clamped to the model ceiling.
func (p *OpenAIProvider) maxTokens(ctx context.Context) int {
	ceiling := p.Limits().EffectiveMaxOutputTokens()
	want := maxOutputTokensFrom(ctx, p.config.MaxTokens)
	if want <= 0 || want > ceiling {
		return ceiling
	}
	return want
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
	Model     string             `json:"model"`
	Messages  []openaiReqMessage `json:"messages"`
	MaxTokens *int               `json:"max_tokens,omitempty"`
	// MaxCompletionTokens replaces MaxTokens on OpenAI's current models.
	MaxCompletionTokens *int `json:"max_completion_tokens,omitempty"`
	// Shared with the Azure provider, which builds the same body.
	Temperature    *float64              `json:"temperature,omitempty"`
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

// truncated reports that the model stopped at the output cap rather than
// finishing. With structured output this yields invalid JSON, so the caller
// needs to know it asked for too much rather than blame the model's formatting.
func (r *openaiResponse) truncated() bool {
	return len(r.Choices) > 0 && r.Choices[0].FinishReason == "length"
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
	FinishReason string        `json:"finish_reason"`
	Message      openaiMessage `json:"message"`
}
