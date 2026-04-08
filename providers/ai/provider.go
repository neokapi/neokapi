package aiprovider

import (
	"context"
	"errors"

	"github.com/neokapi/neokapi/core/model"
)

// LLMProvider defines the interface for LLM service providers.
type LLMProvider interface {
	// Name returns the provider identifier (e.g., "anthropic", "openai").
	Name() string

	// Translate translates text using the LLM.
	Translate(ctx context.Context, req TranslateRequest) (*TranslateResponse, error)

	// Chat sends a general chat message and returns the response.
	Chat(ctx context.Context, messages []Message) (*ChatResponse, error)

	// ChatStructured sends a chat message and constrains the response to match
	// the given JSON schema. The response Content will be valid JSON conforming
	// to the schema. This uses provider-specific mechanisms: OpenAI/Azure use
	// response_format with json_schema, Anthropic uses tool use, Ollama uses
	// the format field.
	ChatStructured(ctx context.Context, messages []Message, schema JSONSchema) (*ChatResponse, error)

	// Close releases provider resources.
	Close() error
}

// JSONSchema defines a JSON schema for structured LLM output.
type JSONSchema struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Schema      map[string]any `json:"schema"`
	Strict      bool           `json:"strict,omitempty"`
}

// Message represents a chat message.
type Message struct {
	Role    string `json:"role"`    // "system", "user", "assistant"
	Content string `json:"content"` // Message text
}

// TranslateRequest contains parameters for a translation request.
type TranslateRequest struct {
	Source         string            `json:"source"`
	SourceLanguage model.LocaleID    `json:"source_language"`
	TargetLocale   model.LocaleID    `json:"target_locale"`
	Context        string            `json:"context,omitempty"`
	Glossary       map[string]string `json:"glossary,omitempty"`
	Format         string            `json:"format,omitempty"` // e.g., "html", "plain"
}

// TokenUsage holds token consumption data from an AI provider call.
type TokenUsage struct {
	InputTokens         int `json:"input_tokens"`
	OutputTokens        int `json:"output_tokens"`
	CacheCreationTokens int `json:"cache_creation_tokens,omitempty"`
	CacheReadTokens     int `json:"cache_read_tokens,omitempty"`
}

// TotalTokens returns the sum of input and output tokens.
func (u TokenUsage) TotalTokens() int {
	return u.InputTokens + u.OutputTokens
}

// Add returns the sum of two TokenUsage values.
func (u TokenUsage) Add(other TokenUsage) TokenUsage {
	return TokenUsage{
		InputTokens:         u.InputTokens + other.InputTokens,
		OutputTokens:        u.OutputTokens + other.OutputTokens,
		CacheCreationTokens: u.CacheCreationTokens + other.CacheCreationTokens,
		CacheReadTokens:     u.CacheReadTokens + other.CacheReadTokens,
	}
}

// TranslateResponse contains the translation result.
type TranslateResponse struct {
	Translation string     `json:"translation"`
	Confidence  float64    `json:"confidence"`
	Model       string     `json:"model"`
	Usage       TokenUsage `json:"usage"`
}

// ChatResponse contains the chat result.
type ChatResponse struct {
	Content string     `json:"content"`
	Model   string     `json:"model"`
	Usage   TokenUsage `json:"usage"`
}

// StreamEventType identifies the kind of streaming event.
type StreamEventType int

const (
	// StreamEventThinking indicates the model is reasoning. Content holds the
	// incremental thinking summary (Gemini) or chain-of-thought text.
	StreamEventThinking StreamEventType = iota

	// StreamEventContent carries a chunk of the actual response text.
	StreamEventContent

	// StreamEventDone signals the stream has completed. Usage is populated.
	StreamEventDone
)

// ChatStreamEvent represents one event in a streaming LLM response.
type ChatStreamEvent struct {
	Type    StreamEventType
	Content string     // text chunk (thinking summary or output content)
	Usage   TokenUsage // cumulative usage; populated on StreamEventDone
	Model   string     // model name; populated on StreamEventDone
}

// StreamingLLMProvider extends LLMProvider with streaming variants of Chat.
// Providers that support streaming implement this interface in addition to
// LLMProvider. Consumers can type-assert:
//
//	if sp, ok := provider.(StreamingLLMProvider); ok { ... }
type StreamingLLMProvider interface {
	LLMProvider

	// ChatStream sends a chat message and streams the response.
	// onEvent is called synchronously for each chunk; it must not block.
	// The final complete ChatResponse is returned when the stream ends.
	ChatStream(ctx context.Context, messages []Message, onEvent func(ChatStreamEvent)) (*ChatResponse, error)

	// ChatStructuredStream is ChatStructured with streaming progress.
	ChatStructuredStream(ctx context.Context, messages []Message, schema JSONSchema, onEvent func(ChatStreamEvent)) (*ChatResponse, error)
}

// ProgressEvent reports translation progress from an AI tool. It is emitted
// once per block and, when streaming is available, includes live thinking
// status from the model.
type ProgressEvent struct {
	// Block is the 1-based index of the block being processed.
	Block int
	// TotalBlocks is the total number of translatable blocks (0 if unknown).
	TotalBlocks int
	// Thinking is the latest thinking summary from the model (empty when not streaming).
	Thinking string
	// Done is true when this block's translation is complete.
	Done bool
}

// QAIssue represents a quality assurance issue found in a translation.
type QAIssue struct {
	Type        string `json:"type"`     // "terminology", "fluency", "accuracy", "consistency"
	Severity    string `json:"severity"` // "error", "warning", "info"
	Description string `json:"description"`
	Suggestion  string `json:"suggestion,omitempty"`
}

// ProviderInfo describes a registered AI provider.
type ProviderInfo struct {
	// Name is the provider identifier (e.g. "anthropic").
	Name string
	// Label is the human-readable display name (e.g. "Anthropic").
	Label string
}

// Providers returns the list of available AI providers in display order.
// This is the canonical source of truth for provider names — used by tool
// schemas, CLI flags, and UI dropdowns.
func Providers() []ProviderInfo {
	return []ProviderInfo{
		{Name: "anthropic", Label: "Anthropic"},
		{Name: "openai", Label: "OpenAI"},
		{Name: "gemini", Label: "Gemini"},
		{Name: "azureopenai", Label: "Azure OpenAI"},
		{Name: "ollama", Label: "Ollama"},
	}
}

// ProviderNames returns just the provider name strings.
func ProviderNames() []string {
	providers := Providers()
	names := make([]string, len(providers))
	for i, p := range providers {
		names[i] = p.Name
	}
	return names
}

// Config holds common provider configuration.
type Config struct {
	APIKey      string  `json:"api_key"`
	BaseURL     string  `json:"base_url,omitempty"`
	Model       string  `json:"model"`
	MaxTokens   int     `json:"max_tokens,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
}

// Validate checks that the config has required fields.
func (c *Config) Validate() error {
	if c.Model == "" {
		return errors.New("model is required")
	}
	return nil
}
