package aiprovider

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// LLMProvider defines the interface for LLM service providers.
type LLMProvider interface {
	// Name returns the provider identifier (e.g., Anthropic, OpenAI).
	Name() ProviderID

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
	// VoiceGuide is brand voice guidance (rendered from a VoiceProfile) that the
	// model should apply while translating, so output is on-brand at generation
	// time rather than only checked afterwards. Empty when no profile is bound.
	VoiceGuide string `json:"voice_guide,omitempty"`
}

// Directives returns the deterministic brand-voice + glossary block appended to
// translation prompts. Glossary terms are sorted so the same request always
// yields byte-identical prompt text. Returns "" when neither is set.
func (req TranslateRequest) Directives() string {
	var b strings.Builder
	if g := strings.TrimSpace(req.VoiceGuide); g != "" {
		b.WriteString("\n\nBrand voice (apply when translating):\n")
		b.WriteString(g)
		b.WriteString("\n")
	}
	if len(req.Glossary) > 0 {
		b.WriteString("\n\nGlossary:\n")
		keys := slices.Sorted(maps.Keys(req.Glossary))
		for _, k := range keys {
			fmt.Fprintf(&b, "- %s → %s\n", k, req.Glossary[k])
		}
	}
	return b.String()
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

// ProviderID is a type-safe identifier for an AI provider.
type ProviderID string

// String returns the string representation.
func (id ProviderID) String() string { return string(id) }

// Known AI provider identifiers.
const (
	Anthropic   ProviderID = "anthropic"
	OpenAI      ProviderID = "openai"
	Gemini      ProviderID = "gemini"
	AzureOpenAI ProviderID = "azureopenai"
	Ollama      ProviderID = "ollama"
	// Demo is a deterministic, offline provider that returns illustrative
	// (not real-model) output. Used by the browser playground so AI commands
	// run with no API keys. See demo.go.
	Demo ProviderID = "demo"
)

// ProviderFactory creates an LLMProvider from a Config.
type ProviderFactory func(cfg Config) LLMProvider

// ProviderInfo describes a registered AI provider.
type ProviderInfo struct {
	// Name is the provider identifier (e.g. "anthropic").
	Name ProviderID
	// Label is the human-readable display name (e.g. "Anthropic").
	Label string
}

// providerRegistration bundles a factory with metadata.
type providerRegistration struct {
	Info    ProviderInfo
	Factory ProviderFactory
	Aliases []ProviderID // alternative names (e.g., "azure_openai" → AzureOpenAI)
}

// globalProviders is the default provider registry, populated by init().
var globalProviders []providerRegistration

func init() {
	RegisterProvider(ProviderInfo{Name: Anthropic, Label: "Anthropic"},
		func(cfg Config) LLMProvider { return NewAnthropicProvider(cfg) })
	RegisterProvider(ProviderInfo{Name: OpenAI, Label: "OpenAI"},
		func(cfg Config) LLMProvider { return NewOpenAIProvider(cfg) })
	RegisterProvider(ProviderInfo{Name: Gemini, Label: "Gemini"},
		func(cfg Config) LLMProvider { return NewGeminiProvider(cfg) })
	RegisterProviderWithAliases(ProviderInfo{Name: AzureOpenAI, Label: "Azure OpenAI"},
		func(cfg Config) LLMProvider { return NewAzureOpenAIProvider(cfg) },
		"azure_openai")
	RegisterProvider(ProviderInfo{Name: Ollama, Label: "Ollama"},
		func(cfg Config) LLMProvider { return NewOllamaProvider(cfg) })
	RegisterProvider(ProviderInfo{Name: Demo, Label: "Demo (illustrative)"},
		func(cfg Config) LLMProvider { return NewDemoProvider(cfg) })
}

// RegisterProvider registers a new AI provider factory. Plugins can call this
// to add custom providers that will appear in tool schemas and CLI flags.
func RegisterProvider(info ProviderInfo, factory ProviderFactory) {
	globalProviders = append(globalProviders, providerRegistration{
		Info:    info,
		Factory: factory,
	})
}

// RegisterProviderWithAliases registers a provider with alternative name aliases.
func RegisterProviderWithAliases(info ProviderInfo, factory ProviderFactory, aliases ...ProviderID) {
	globalProviders = append(globalProviders, providerRegistration{
		Info:    info,
		Factory: factory,
		Aliases: aliases,
	})
}

// NewProvider creates an LLMProvider by looking up the registered factory for
// the given provider name. Returns an error if the provider is not registered.
func NewProvider(name ProviderID, cfg Config) (LLMProvider, error) {
	for _, reg := range globalProviders {
		if reg.Info.Name == name {
			return reg.Factory(cfg), nil
		}
		for _, alias := range reg.Aliases {
			if alias == name {
				return reg.Factory(cfg), nil
			}
		}
	}
	return nil, fmt.Errorf("unknown AI provider: %s (supported: %s)", name, strings.Join(ProviderNames(), ", "))
}

// Providers returns the list of available AI providers in display order.
// This is the canonical source of truth for provider names — used by tool
// schemas, CLI flags, and UI dropdowns.
func Providers() []ProviderInfo {
	infos := make([]ProviderInfo, len(globalProviders))
	for i, reg := range globalProviders {
		infos[i] = reg.Info
	}
	return infos
}

// ProviderNames returns just the provider name strings.
func ProviderNames() []string {
	names := make([]string, len(globalProviders))
	for i, reg := range globalProviders {
		names[i] = string(reg.Info.Name)
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

// standardTranslate runs the common "translate this text, return only the
// translation" prompt through a provider's Chat method and wraps the response.
// Most LLM providers share this exact flow, differing only in the confidence
// they report; providers needing a different prompt strategy (e.g. Azure's
// system-prompted variant) implement Translate directly.
func standardTranslate(
	ctx context.Context,
	chat func(context.Context, []Message) (*ChatResponse, error),
	req TranslateRequest,
	confidence float64,
) (*TranslateResponse, error) {
	prompt := fmt.Sprintf(
		"Translate the following text from %s to %s. Return ONLY the translation, no explanation.\n\nText: %s",
		req.SourceLanguage, req.TargetLocale, req.Source,
	) + req.Directives()

	resp, err := chat(ctx, []Message{{Role: "user", Content: prompt}})
	if err != nil {
		return nil, err
	}
	return &TranslateResponse{
		Translation: resp.Content,
		Confidence:  confidence,
		Model:       resp.Model,
		Usage:       resp.Usage,
	}, nil
}
