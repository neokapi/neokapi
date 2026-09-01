package aiprovider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/httputil"
)

// geminiStreamTimeout bounds a single streaming request. Gemini 3 models use
// thinking by default, which can take well over 30s, so this is generous.
const geminiStreamTimeout = 5 * time.Minute

// DefaultGeminiModel is the model a fresh provider uses when the caller names none.
const DefaultGeminiModel = "gemini-3.5-flash"

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
		cfg.Model = DefaultGeminiModel
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

func (p *GeminiProvider) InputModalities() []Modality {
	return []Modality{ModalityImage, ModalityAudio, ModalityVideo}
}

func (p *GeminiProvider) Translate(ctx context.Context, req TranslateRequest) (*TranslateResponse, error) {
	return StandardTranslate(ctx, p.Name(), p.Chat, req, 0.85)
}

func (p *GeminiProvider) Chat(ctx context.Context, messages []Message) (*ChatResponse, error) {
	contents, err := messagesToGeminiContents(messages)
	if err != nil {
		return nil, err
	}

	body := geminiRequest{
		Contents: contents,
		GenerationConfig: &geminiGenerationConfig{
			Temperature:     p.config.Temperature,
			MaxOutputTokens: p.maxTokens(ctx),
			ThinkingConfig:  noThinking(p.config.Model),
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
		Content:   text,
		Model:     resp.ModelVersion,
		Usage:     resp.UsageMetadata.toTokenUsage(),
		Truncated: resp.truncated(),
	}, nil
}

func (p *GeminiProvider) ChatStructured(ctx context.Context, messages []Message, schema JSONSchema) (*ChatResponse, error) {
	contents, err := messagesToGeminiContents(messages)
	if err != nil {
		return nil, err
	}

	body := geminiRequest{
		Contents: contents,
		GenerationConfig: &geminiGenerationConfig{
			Temperature:      p.config.Temperature,
			MaxOutputTokens:  p.maxTokens(ctx),
			ResponseMIMEType: "application/json",
			ResponseSchema:   stripAdditionalProperties(schema.Schema),
			ThinkingConfig:   noThinking(p.config.Model),
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
		Content:   text,
		Model:     resp.ModelVersion,
		Usage:     resp.UsageMetadata.toTokenUsage(),
		Truncated: resp.truncated(),
	}, nil
}

// ChatStream implements StreamingLLMProvider. It uses Gemini's streamGenerateContent
// endpoint with thinking enabled, streaming incremental thinking summaries and
// content chunks via the onEvent callback.
func (p *GeminiProvider) ChatStream(ctx context.Context, messages []Message, onEvent func(ChatStreamEvent)) (*ChatResponse, error) {
	contents, err := messagesToGeminiContents(messages)
	if err != nil {
		return nil, err
	}

	body := geminiRequest{
		Contents: contents,
		GenerationConfig: &geminiGenerationConfig{
			Temperature:     p.config.Temperature,
			MaxOutputTokens: p.maxTokens(ctx),
			ThinkingConfig:  &geminiThinkingConfig{IncludeThoughts: true},
		},
	}

	return p.doStreamRequest(ctx, body, onEvent)
}

// ChatStructuredStream implements StreamingLLMProvider with JSON schema constraints.
func (p *GeminiProvider) ChatStructuredStream(ctx context.Context, messages []Message, schema JSONSchema, onEvent func(ChatStreamEvent)) (*ChatResponse, error) {
	contents, err := messagesToGeminiContents(messages)
	if err != nil {
		return nil, err
	}

	body := geminiRequest{
		Contents: contents,
		GenerationConfig: &geminiGenerationConfig{
			Temperature:      p.config.Temperature,
			MaxOutputTokens:  p.maxTokens(ctx),
			ResponseMIMEType: "application/json",
			ResponseSchema:   stripAdditionalProperties(schema.Schema),
			ThinkingConfig:   &geminiThinkingConfig{IncludeThoughts: true},
		},
	}

	return p.doStreamRequest(ctx, body, onEvent)
}

// Limits reports what the configured model can emit.
func (p *GeminiProvider) Limits() Limits {
	if l, ok := LimitsForModel(p.config.Model); ok {
		return l
	}
	return Limits{MaxOutputTokens: ConservativeMaxOutputTokens}
}

// maxTokens resolves this request's output budget, clamped to the model ceiling.
// Always sent explicitly: Gemini's own default is a small fraction of what the
// model can emit, and relying on it truncates long replies without saying so.
//
// A thinking model gets the ceiling regardless of what the caller asked for.
// Callers size their budget for the *answer* ("this batch of segments should cost
// about N tokens to translate"), but Gemini draws thoughts from the same
// maxOutputTokens allowance. A model that thinks for 800 tokens under a 900-token
// budget emits a truncated answer — measured against gemini-3.1-pro-preview, that
// meant translations cut off mid-tag (`<ph id="2`), which is unusable. Tokens are
// billed on what is emitted, not on what is permitted, so raising the cap costs
// nothing and only removes a way to silently corrupt output.
func (p *GeminiProvider) maxTokens(ctx context.Context) int {
	ceiling := p.Limits().EffectiveMaxOutputTokens()
	if requiresThinking(p.config.Model) {
		return ceiling
	}
	want := maxOutputTokensFrom(ctx, p.config.MaxTokens)
	if want <= 0 || want > ceiling {
		return ceiling
	}
	return want
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
		respBody, _ := readCappedBody(httpResp.Body)
		return nil, fmt.Errorf("gemini: API error %d: %s", httpResp.StatusCode, string(respBody))
	}

	// Parse SSE stream. Gemini sends "data: {json}\n\n" chunks.
	var (
		contentBuf strings.Builder
		modelVer   string
		usage      TokenUsage
		// finishReason is the last non-empty one seen. A stream reports it on the
		// final chunk, and MAX_TOKENS there means the accumulated content is a
		// fragment — the same fact the blocking path reads off the response, and
		// the only thing that distinguishes a cut-off reply from a complete one.
		finishReason string
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
			if cand.FinishReason != "" {
				finishReason = cand.FinishReason
			}
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
		Content:   contentBuf.String(),
		Model:     modelVer,
		Usage:     usage,
		Truncated: finishReason == geminiFinishMaxTokens,
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

	respBody, err := readCappedBody(httpResp.Body)
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
func messagesToGeminiContents(messages []Message) ([]geminiContent, error) {
	var contents []geminiContent
	var systemText string

	for _, m := range messages {
		if m.Role == "system" {
			systemText += m.Text() + "\n"
			continue
		}

		role := m.Role
		if role == "assistant" {
			role = "model"
		}

		prefix := ""
		if systemText != "" && role == "user" {
			prefix = systemText + "\n"
			systemText = ""
		}

		parts := make([]geminiPart, 0, len(m.Parts))
		for _, part := range m.Parts {
			switch part.Kind {
			case ContentText:
				text := part.Text
				if prefix != "" {
					text = prefix + text
					prefix = ""
				}
				parts = append(parts, geminiPart{Text: text})
			case ContentImage, ContentAudio, ContentVideo:
				b64, mime, err := resolveMediaBase64(part.Media)
				if err != nil {
					return nil, fmt.Errorf("gemini: %w", err)
				}
				parts = append(parts, geminiPart{InlineData: &geminiInlineData{MimeType: mime, Data: b64}})
			default:
				return nil, fmt.Errorf("gemini: unsupported content kind %q", part.Kind)
			}
		}
		// A pending system prefix with no text part to attach to becomes its own part.
		if prefix != "" {
			parts = append([]geminiPart{{Text: prefix}}, parts...)
		}

		contents = append(contents, geminiContent{Role: role, Parts: parts})
	}

	return contents, nil
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
	Text       string            `json:"text,omitempty"`
	InlineData *geminiInlineData `json:"inlineData,omitempty"`
	Thought    bool              `json:"thought,omitempty"`
}

type geminiInlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"` // base64-encoded bytes
}

type geminiRequest struct {
	Contents         []geminiContent         `json:"contents"`
	GenerationConfig *geminiGenerationConfig `json:"generationConfig,omitempty"`
}

type geminiThinkingConfig struct {
	ThinkingBudget  int  `json:"thinkingBudget"`
	IncludeThoughts bool `json:"includeThoughts,omitempty"`
}

// noThinking returns the thinking config for a non-streaming call: thinking off,
// which is what translation and the structured tool calls want — they are
// transformations, not reasoning problems, and thinking only buys latency.
//
// Except that the *pro* models cannot do that. They reject a zero budget outright
// ("Budget 0 is invalid. This model only works in thinking mode"), so sending one
// made every pro model unusable in kapi, at every batch size, with a 400. For
// those we omit the config entirely and take the model's own default.
//
// The discriminator is the family name, because that is what the API contract is
// actually keyed on; Google names every model in the family `…-pro…`
// (gemini-2.5-pro, gemini-3-pro-preview, gemini-3.1-pro-preview). An unknown
// model that turns out to require thinking degrades to the same 400 it would have
// given anyway, and a new pro model needs no code change.
func noThinking(model string) *geminiThinkingConfig {
	if requiresThinking(model) {
		return nil
	}
	return &geminiThinkingConfig{ThinkingBudget: 0}
}

// requiresThinking reports whether a model refuses to run with thinking disabled.
func requiresThinking(model string) bool {
	return strings.Contains(strings.ToLower(model), "-pro")
}

type geminiGenerationConfig struct {
	MaxOutputTokens int `json:"maxOutputTokens,omitempty"`
	// Omitted when the caller did not set one. It used to be omitted always.
	Temperature      *float64              `json:"temperature,omitempty"`
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
	Content      geminiContent `json:"content"`
	FinishReason string        `json:"finishReason"`
}

// geminiFinishMaxTokens is the finishReason Gemini reports for a reply cut off
// by maxOutputTokens, on both the blocking and the streaming endpoint.
const geminiFinishMaxTokens = "MAX_TOKENS"

// truncated reports a reply cut off by maxOutputTokens. Gemini is the easiest
// provider to truncate silently: its own default output cap is far below the
// model's ceiling, so a batch that fits the model can still overflow the default.
func (r *geminiResponse) truncated() bool {
	return len(r.Candidates) > 0 && r.Candidates[0].FinishReason == geminiFinishMaxTokens
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
