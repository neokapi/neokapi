package aiprovider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/httputil"
)

// geminiStreamTimeout bounds a single streaming request. Gemini 3 models use
// thinking by default, which can take well over 30s, so this is generous.
const geminiStreamTimeout = 5 * time.Minute

// GeminiProvider implements LLMProvider for the Google Gemini API.
type GeminiProvider struct {
	config Config
	client *http.Client
	// streamClient is a non-retrying client used for the SSE streaming
	// endpoint. Retries make no sense once a stream has started (the body is
	// consumed incrementally), so this client uses the plain base transport
	// rather than the resilient client's retryTransport.
	streamClient *http.Client
}

// NewGeminiProvider creates a new Gemini provider.
func NewGeminiProvider(cfg Config) *GeminiProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://generativelanguage.googleapis.com"
	}
	if cfg.Model == "" {
		cfg.Model = "gemini-3-flash-preview"
	}
	if cfg.MaxTokens == 0 {
		cfg.MaxTokens = 4096
	}
	// Gemini 3 models use thinking by default, which can take well over 30s.
	// Use a longer timeout than the default resilient client.
	client := httputil.NewResilientClient()
	client.Timeout = geminiStreamTimeout

	// Streaming uses a separate, non-retrying client built on the plain base
	// transport (httputil.NewClient, no retryTransport). Per-request deadlines
	// are applied via context.WithTimeout in doStreamRequest, so leave this
	// client's Timeout at zero to avoid double-bounding the stream.
	streamClient := httputil.NewClient()
	streamClient.Timeout = 0

	return &GeminiProvider{
		config:       cfg,
		client:       client,
		streamClient: streamClient,
	}
}

func (p *GeminiProvider) Name() ProviderID { return Gemini }

func (p *GeminiProvider) Translate(ctx context.Context, req TranslateRequest) (*TranslateResponse, error) {
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

func (p *GeminiProvider) Chat(ctx context.Context, messages []Message) (*ChatResponse, error) {
	contents := messagesToGeminiContents(messages)

	body := geminiRequest{
		Contents: contents,
		GenerationConfig: &geminiGenerationConfig{
			MaxOutputTokens: p.config.MaxTokens,
			ThinkingConfig:  &geminiThinkingConfig{ThinkingBudget: 0},
		},
	}

	resp, err := p.doRequest(ctx, body)
	if err != nil {
		return nil, err
	}

	text, err := extractGeminiText(resp)
	if err != nil {
		return nil, err
	}

	return &ChatResponse{
		Content: text,
		Model:   resp.ModelVersion,
		Usage:   resp.UsageMetadata.toTokenUsage(),
	}, nil
}

func (p *GeminiProvider) ChatStructured(ctx context.Context, messages []Message, schema JSONSchema) (*ChatResponse, error) {
	contents := messagesToGeminiContents(messages)

	body := geminiRequest{
		Contents: contents,
		GenerationConfig: &geminiGenerationConfig{
			MaxOutputTokens:  p.config.MaxTokens,
			ResponseMIMEType: "application/json",
			ResponseSchema:   stripAdditionalProperties(schema.Schema),
			ThinkingConfig:   &geminiThinkingConfig{ThinkingBudget: 0},
		},
	}

	resp, err := p.doRequest(ctx, body)
	if err != nil {
		return nil, err
	}

	text, err := extractGeminiText(resp)
	if err != nil {
		return nil, err
	}

	return &ChatResponse{
		Content: text,
		Model:   resp.ModelVersion,
		Usage:   resp.UsageMetadata.toTokenUsage(),
	}, nil
}

// ChatStream implements StreamingLLMProvider. It uses Gemini's streamGenerateContent
// endpoint with thinking enabled, streaming incremental thinking summaries and
// content chunks via the onEvent callback.
func (p *GeminiProvider) ChatStream(ctx context.Context, messages []Message, onEvent func(ChatStreamEvent)) (*ChatResponse, error) {
	contents := messagesToGeminiContents(messages)

	body := geminiRequest{
		Contents: contents,
		GenerationConfig: &geminiGenerationConfig{
			MaxOutputTokens: p.config.MaxTokens,
			ThinkingConfig:  &geminiThinkingConfig{IncludeThoughts: true},
		},
	}

	return p.doStreamRequest(ctx, body, onEvent)
}

// ChatStructuredStream implements StreamingLLMProvider with JSON schema constraints.
func (p *GeminiProvider) ChatStructuredStream(ctx context.Context, messages []Message, schema JSONSchema, onEvent func(ChatStreamEvent)) (*ChatResponse, error) {
	contents := messagesToGeminiContents(messages)

	body := geminiRequest{
		Contents: contents,
		GenerationConfig: &geminiGenerationConfig{
			MaxOutputTokens:  p.config.MaxTokens,
			ResponseMIMEType: "application/json",
			ResponseSchema:   stripAdditionalProperties(schema.Schema),
			ThinkingConfig:   &geminiThinkingConfig{IncludeThoughts: true},
		},
	}

	return p.doStreamRequest(ctx, body, onEvent)
}

func (p *GeminiProvider) Close() error { return nil }

// Compile-time check that GeminiProvider implements StreamingLLMProvider.
var _ StreamingLLMProvider = (*GeminiProvider)(nil)

func (p *GeminiProvider) doStreamRequest(ctx context.Context, body geminiRequest, onEvent func(ChatStreamEvent)) (*ChatResponse, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("gemini: marshal request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/v1beta/models/%s:streamGenerateContent",
		p.config.BaseURL, p.config.Model)
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("gemini: parse URL: %w", err)
	}
	q := u.Query()
	q.Set("alt", "sse")
	u.RawQuery = q.Encode()

	// Apply an explicit deadline so the stream cannot hang indefinitely. The
	// streamClient has no Timeout of its own (Client.Timeout would not bound a
	// long-lived SSE body usefully), so the deadline lives on the context.
	ctx, cancel := context.WithTimeout(ctx, geminiStreamTimeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("gemini: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	// Pass the API key in a header rather than the URL query string so it does
	// not leak into transport-error messages (which include the request URL).
	httpReq.Header.Set("x-goog-api-key", p.config.APIKey)

	// Use the non-retrying stream client: once a stream starts, replaying the
	// request makes no sense, and the resilient client's retryTransport would
	// otherwise back off and retry transient failures here.
	httpResp, err := p.streamClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gemini: stream request: %w", err)
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(httpResp.Body)
		return nil, fmt.Errorf("gemini: API error %d: %s", httpResp.StatusCode, string(respBody))
	}

	// Parse SSE stream. Gemini sends "data: {json}\n\n" chunks.
	var (
		contentBuf strings.Builder
		modelVer   string
		usage      TokenUsage
	)

	scanner := bufio.NewScanner(httpResp.Body)
	// A single SSE "data:" line can exceed bufio.Scanner's default 64KB cap
	// (e.g. a large content or thinking chunk). Raise the max token size so the
	// stream is not aborted with bufio.ErrTooLong.
	scanner.Buffer(make([]byte, 0, 64*1024), 8<<20)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		var chunk geminiResponse
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue // skip malformed chunks
		}

		if chunk.ModelVersion != "" {
			modelVer = chunk.ModelVersion
		}
		if chunk.UsageMetadata.TotalTokenCount > 0 {
			usage = chunk.UsageMetadata.toTokenUsage()
		}

		for _, cand := range chunk.Candidates {
			for _, part := range cand.Content.Parts {
				if part.Thought {
					onEvent(ChatStreamEvent{
						Type:    StreamEventThinking,
						Content: part.Text,
					})
				} else if part.Text != "" {
					contentBuf.WriteString(part.Text)
					onEvent(ChatStreamEvent{
						Type:    StreamEventContent,
						Content: part.Text,
					})
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("gemini: read stream: %w", err)
	}

	onEvent(ChatStreamEvent{
		Type:  StreamEventDone,
		Usage: usage,
		Model: modelVer,
	})

	return &ChatResponse{
		Content: contentBuf.String(),
		Model:   modelVer,
		Usage:   usage,
	}, nil
}

func (p *GeminiProvider) doRequest(ctx context.Context, body geminiRequest) (*geminiResponse, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("gemini: marshal request: %w", err)
	}

	endpoint := fmt.Sprintf("%s/v1beta/models/%s:generateContent",
		p.config.BaseURL, p.config.Model)
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("gemini: parse URL: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("gemini: create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	// Pass the API key in a header rather than the URL query string so it does
	// not leak into transport-error messages (which include the request URL).
	httpReq.Header.Set("x-goog-api-key", p.config.APIKey)

	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gemini: request: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("gemini: read response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gemini: API error %d: %s", httpResp.StatusCode, string(respBody))
	}

	var apiResp geminiResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("gemini: unmarshal response: %w", err)
	}

	return &apiResp, nil
}

// messagesToGeminiContents converts chat messages to Gemini contents format.
// System messages are prepended to the first user message since Gemini uses
// a separate systemInstruction field (handled at the request level if needed).
func messagesToGeminiContents(messages []Message) []geminiContent {
	var contents []geminiContent
	var systemText string

	for _, m := range messages {
		if m.Role == "system" {
			systemText += m.Content + "\n"
			continue
		}

		role := m.Role
		if role == "assistant" {
			role = "model"
		}

		content := m.Content
		if systemText != "" && role == "user" {
			content = systemText + "\n" + content
			systemText = ""
		}

		contents = append(contents, geminiContent{
			Role:  role,
			Parts: []geminiPart{{Text: content}},
		})
	}

	return contents
}

func extractGeminiText(resp *geminiResponse) (string, error) {
	if len(resp.Candidates) == 0 {
		return "", errors.New("gemini: no candidates in response")
	}

	// Skip thinking parts (thought: true) — only collect actual output.
	var text strings.Builder
	for _, part := range resp.Candidates[0].Content.Parts {
		if part.Thought {
			continue
		}
		text.WriteString(part.Text)
	}

	return text.String(), nil
}

// stripAdditionalProperties recursively removes "additionalProperties" keys
// from a JSON schema map. Gemini's API does not support this field.
func stripAdditionalProperties(s map[string]any) map[string]any {
	if s == nil {
		return nil
	}
	out := make(map[string]any, len(s))
	for k, v := range s {
		if k == "additionalProperties" {
			continue
		}
		if m, ok := v.(map[string]any); ok {
			out[k] = stripAdditionalProperties(m)
		} else {
			out[k] = v
		}
	}
	return out
}

// Gemini API types

type geminiContent struct {
	Role  string       `json:"role"`
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text    string `json:"text"`
	Thought bool   `json:"thought,omitempty"`
}

type geminiRequest struct {
	Contents         []geminiContent         `json:"contents"`
	GenerationConfig *geminiGenerationConfig `json:"generationConfig,omitempty"`
}

type geminiThinkingConfig struct {
	ThinkingBudget  int  `json:"thinkingBudget"`
	IncludeThoughts bool `json:"includeThoughts,omitempty"`
}

type geminiGenerationConfig struct {
	MaxOutputTokens  int                   `json:"maxOutputTokens,omitempty"`
	ResponseMIMEType string                `json:"responseMimeType,omitempty"`
	ResponseSchema   map[string]any        `json:"responseSchema,omitempty"`
	ThinkingConfig   *geminiThinkingConfig `json:"thinkingConfig,omitempty"`
}

type geminiResponse struct {
	Candidates    []geminiCandidate   `json:"candidates"`
	ModelVersion  string              `json:"modelVersion"`
	UsageMetadata geminiUsageMetadata `json:"usageMetadata"`
}

type geminiCandidate struct {
	Content geminiContent `json:"content"`
}

type geminiUsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

func (u geminiUsageMetadata) toTokenUsage() TokenUsage {
	return TokenUsage{
		InputTokens:  u.PromptTokenCount,
		OutputTokens: u.CandidatesTokenCount,
	}
}
