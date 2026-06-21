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

// ollamaKeepAlive keeps a loaded model resident between requests. kapi
// translates one block at a time on local providers, so without this the model
// would unload and reload (cold start) on every block. "10m" mirrors Ollama's
// own default but is set explicitly so the warm window doesn't depend on the
// server's configuration.
const ollamaKeepAlive = "10m"

// DefaultOllamaModel is the model kapi pulls and uses for local translation when
// the caller names none. llama3.2:3b is the strongest small local model for
// kapi's constrained-translation path (exact glossary + brand voice + verbatim
// inline placeholders + translation-only output) at ~90 tok/s on Metal.
const DefaultOllamaModel = "llama3.2:3b"

// ollamaTranslateTemperature is the default sampling temperature for Ollama when
// the caller leaves Config.Temperature unset (0). kapi's local-model use is
// overwhelmingly translation and QA, where deterministic, glossary-faithful
// output matters far more than creative variety — Ollama's own default (0.8) is
// too loose and degrades terminology obedience.
const ollamaTranslateTemperature = 0.2

// OllamaProvider implements LLMProvider (and StreamingLLMProvider) for local
// Ollama models. Ollama runs models on the GPU (Metal on Apple Silicon — now via
// Apple's MLX backend — CUDA/Vulkan elsewhere), so it is the fast on-device path
// when an Ollama server is available.
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
		cfg.Model = DefaultOllamaModel
	}
	return &OllamaProvider{
		config: cfg,
		client: httputil.NewResilientClient(),
	}
}

func (p *OllamaProvider) Name() ProviderID { return Ollama }

func (p *OllamaProvider) InputModalities() []Modality { return []Modality{ModalityImage} }

func (p *OllamaProvider) Translate(ctx context.Context, req TranslateRequest) (*TranslateResponse, error) {
	return standardTranslate(ctx, p.Chat, req, 0.7)
}

func (p *OllamaProvider) Chat(ctx context.Context, messages []Message) (*ChatResponse, error) {
	return p.chat(ctx, messages, nil)
}

func (p *OllamaProvider) ChatStructured(ctx context.Context, messages []Message, schema JSONSchema) (*ChatResponse, error) {
	return p.chat(ctx, messages, schema.Schema)
}

// ChatStream sends a chat message and streams the response token-by-token. It
// satisfies StreamingLLMProvider, giving the translate/QA tools live progress
// instead of one long opaque wait on a local model.
func (p *OllamaProvider) ChatStream(ctx context.Context, messages []Message, onEvent func(ChatStreamEvent)) (*ChatResponse, error) {
	return p.stream(ctx, messages, nil, onEvent)
}

// ChatStructuredStream is ChatStructured with streaming progress.
func (p *OllamaProvider) ChatStructuredStream(ctx context.Context, messages []Message, schema JSONSchema, onEvent func(ChatStreamEvent)) (*ChatResponse, error) {
	return p.stream(ctx, messages, schema.Schema, onEvent)
}

func (p *OllamaProvider) Close() error { return nil }

// chat performs a non-streaming /api/chat request. format is nil for free-form
// chat or a JSON schema for structured output.
func (p *OllamaProvider) chat(ctx context.Context, messages []Message, format any) (*ChatResponse, error) {
	req, err := p.buildRequest(messages, format, false)
	if err != nil {
		return nil, err
	}

	httpResp, err := p.do(ctx, req)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, fmt.Errorf("ollama: read response: %w", err)
	}
	if httpResp.StatusCode != http.StatusOK {
		return nil, p.statusError(httpResp.StatusCode, respBody)
	}

	var apiResp ollamaChatResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("ollama: unmarshal response: %w", err)
	}
	return apiResp.toChatResponse(), nil
}

// stream performs a streaming /api/chat request, invoking onEvent for each
// thinking/content chunk and once more on completion, and returns the assembled
// response.
func (p *OllamaProvider) stream(ctx context.Context, messages []Message, format any, onEvent func(ChatStreamEvent)) (*ChatResponse, error) {
	req, err := p.buildRequest(messages, format, true)
	if err != nil {
		return nil, err
	}

	httpResp, err := p.do(ctx, req)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(httpResp.Body)
		return nil, p.statusError(httpResp.StatusCode, body)
	}

	dec := json.NewDecoder(httpResp.Body)
	var content strings.Builder
	var final ollamaChatResponse
	for {
		var chunk ollamaChatResponse
		if err := dec.Decode(&chunk); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("ollama: decode stream: %w", err)
		}
		if t := chunk.Message.Thinking; t != "" && onEvent != nil {
			onEvent(ChatStreamEvent{Type: StreamEventThinking, Content: t})
		}
		if c := chunk.Message.Content; c != "" {
			content.WriteString(c)
			if onEvent != nil {
				onEvent(ChatStreamEvent{Type: StreamEventContent, Content: c})
			}
		}
		if chunk.Done {
			final = chunk
		}
	}

	resp := final.toChatResponse()
	resp.Content = content.String()
	if onEvent != nil {
		onEvent(ChatStreamEvent{Type: StreamEventDone, Usage: resp.Usage, Model: resp.Model})
	}
	return resp, nil
}

// buildRequest assembles the /api/chat request body shared by streaming and
// non-streaming calls.
func (p *OllamaProvider) buildRequest(messages []Message, format any, stream bool) (*ollamaChatRequest, error) {
	ollamaMessages, err := toOllamaMessages(messages)
	if err != nil {
		return nil, err
	}
	// Reasoning-capable models (e.g. qwen3) otherwise emit a <think> block that
	// consumes the output budget and truncates the translation. kapi wants the
	// answer, not the chain of thought, so reasoning is hard-disabled on every
	// call; non-reasoning models ignore the flag.
	noThink := false
	return &ollamaChatRequest{
		Model:     p.config.Model,
		Messages:  ollamaMessages,
		Stream:    stream,
		Format:    format,
		Options:   p.options(),
		KeepAlive: ollamaKeepAlive,
		Think:     &noThink,
	}, nil
}

// do marshals and POSTs an /api/chat request, translating transport failures
// (e.g. a stopped server) into an actionable error.
func (p *OllamaProvider) do(ctx context.Context, req *ollamaChatRequest) (*http.Response, error) {
	jsonBody, err := json.Marshal(req)
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
		return nil, p.transportError(err)
	}
	return httpResp, nil
}

// options builds the per-request sampling options. A low default temperature is
// applied when the caller leaves it unset (see ollamaTranslateTemperature), and
// MaxTokens maps to Ollama's num_predict.
func (p *OllamaProvider) options() *ollamaOptions {
	temp := p.config.Temperature
	if temp == 0 {
		temp = ollamaTranslateTemperature
	}
	o := &ollamaOptions{Temperature: &temp}
	if p.config.MaxTokens > 0 {
		n := p.config.MaxTokens
		o.NumPredict = &n
	}
	return o
}

// transportError turns a connection-level failure into guidance: the most common
// cause is that no Ollama server is running at BaseURL.
func (p *OllamaProvider) transportError(err error) error {
	return ollamaUnreachableError(p.config.BaseURL, err)
}

// statusError turns a non-200 response into guidance. A 404 almost always means
// the requested model has not been pulled yet.
func (p *OllamaProvider) statusError(status int, body []byte) error {
	msg := strings.TrimSpace(string(body))
	if status == http.StatusNotFound || strings.Contains(strings.ToLower(msg), "not found") {
		return fmt.Errorf("ollama: model %q is not installed — pull it first with `ollama pull %s` (server said: %s)", p.config.Model, p.config.Model, msg)
	}
	return fmt.Errorf("ollama: API error %d: %s", status, msg)
}

// Ollama API types

type ollamaMessage struct {
	Role     string   `json:"role"`
	Content  string   `json:"content"`
	Thinking string   `json:"thinking,omitempty"` // reasoning summary (thinking models)
	Images   []string `json:"images,omitempty"`   // base64-encoded images (vision models)
}

func toOllamaMessages(messages []Message) ([]ollamaMessage, error) {
	out := make([]ollamaMessage, len(messages))
	for i, m := range messages {
		om := ollamaMessage{Role: m.Role, Content: m.Text()}
		for _, part := range m.Parts {
			switch part.Kind {
			case ContentText:
				// text is already folded into Content via m.Text()
			case ContentImage:
				b64, _, err := resolveMediaBase64(part.Media)
				if err != nil {
					return nil, fmt.Errorf("ollama: %w", err)
				}
				om.Images = append(om.Images, b64)
			default:
				return nil, fmt.Errorf("ollama: unsupported content kind %q (provider accepts text, image)", part.Kind)
			}
		}
		out[i] = om
	}
	return out, nil
}

// ollamaOptions carries Ollama's per-request sampling parameters. Fields are
// pointers so an unset value is omitted and Ollama applies its own default.
type ollamaOptions struct {
	Temperature *float64 `json:"temperature,omitempty"`
	NumPredict  *int     `json:"num_predict,omitempty"`
	TopP        *float64 `json:"top_p,omitempty"`
}

type ollamaChatRequest struct {
	Model     string          `json:"model"`
	Messages  []ollamaMessage `json:"messages"`
	Stream    bool            `json:"stream"`
	Format    any             `json:"format,omitempty"` // JSON schema for structured output
	Options   *ollamaOptions  `json:"options,omitempty"`
	KeepAlive string          `json:"keep_alive,omitempty"`
	Think     *bool           `json:"think,omitempty"` // false disables reasoning <think> blocks
}

type ollamaChatResponse struct {
	Model           string        `json:"model"`
	Message         ollamaMessage `json:"message"`
	Done            bool          `json:"done"`
	PromptEvalCount int           `json:"prompt_eval_count"`
	EvalCount       int           `json:"eval_count"`
}

func (r ollamaChatResponse) toChatResponse() *ChatResponse {
	return &ChatResponse{
		Content: r.Message.Content,
		Model:   r.Model,
		Usage: TokenUsage{
			InputTokens:  r.PromptEvalCount,
			OutputTokens: r.EvalCount,
		},
	}
}
