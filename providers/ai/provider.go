package aiprovider

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/neokapi/neokapi/core/ai/prompt"
	"github.com/neokapi/neokapi/core/model"
)

// LLMProvider defines the interface for LLM service providers.
type LLMProvider interface {
	// Name returns the provider identifier (e.g., Anthropic, OpenAI).
	Name() ProviderID

	// InputModalities returns the non-text inputs this provider accepts (text is
	// always accepted). A caller checks a message's media parts against this
	// before a call rather than discovering the limit at request time.
	InputModalities() []Modality

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

// Message roles. A prompt declares roles semantically; each provider is
// responsible for transporting them the way its API expects — Anthropic lifts
// system into a top-level parameter, Claude Code passes it as
// --append-system-prompt, OpenAI carries it as an ordinary message. Callers
// never adapt to the provider.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// Message represents a chat message. Content is an ordered list of parts so one
// message can mix text with image/audio/video — a text-only message is a single
// text part (see TextMessage).
type Message struct {
	Role  string        `json:"role"` // RoleSystem, RoleUser, RoleAssistant
	Parts []ContentPart `json:"parts"`
	// Sections attributes the message's text to what produced each piece of it
	// (framework rule, voice profile, terms, the source block). Set when
	// the message came from a prompt builder; --explain renders it so a prompt can
	// be traced back to its origins. Providers ignore it — they send Parts.
	Sections []prompt.Section `json:"sections,omitempty"`
}

// TextMessage builds a text-only message — the common case.
func TextMessage(role, text string) Message {
	return Message{Role: role, Parts: []ContentPart{TextPart(text)}}
}

// SplitSystem separates system-role messages from the rest of the conversation,
// joining their text with blank lines. Providers whose API carries system
// instructions out-of-band use this so a "system" role never leaks into the
// message list — Anthropic rejects such a message outright.
func SplitSystem(messages []Message) (system string, rest []Message) {
	var sys []string
	rest = make([]Message, 0, len(messages))
	for _, m := range messages {
		if m.Role == RoleSystem {
			if t := strings.TrimSpace(m.Text()); t != "" {
				sys = append(sys, t)
			}
			continue
		}
		rest = append(rest, m)
	}
	return strings.Join(sys, "\n\n"), rest
}

// Text returns the concatenated text of the message's text parts (media parts
// contribute nothing), for providers and callers that want the plain text.
func (m Message) Text() string {
	var b strings.Builder
	for _, p := range m.Parts {
		if p.Kind == ContentText {
			b.WriteString(p.Text)
		}
	}
	return b.String()
}

// TranslateRequest contains parameters for a translation request.
type TranslateRequest struct {
	Source         string            `json:"source"`
	SourceLanguage model.LocaleID    `json:"source_language"`
	TargetLocale   model.LocaleID    `json:"target_locale"`
	PreferredTerms map[string]string `json:"preferred_terms,omitempty"`
	Format         string            `json:"format,omitempty"` // e.g., "html", "plain"
	// VoiceGuide is voice profile guidance (rendered from a VoiceProfile) that the
	// model should apply while translating, so output is on-brand at generation
	// time rather than only checked afterwards. Empty when no profile is bound.
	VoiceGuide string `json:"voice_guide,omitempty"`
	// Instruction is a caller-supplied directive the model should apply while
	// translating — a reviewer's "keep it informal", or the findings a fix pass
	// must resolve.
	Instruction string `json:"instruction,omitempty"`
	// PreserveTags marks a source that carries inline codes rendered as
	// placeholder-tagged text, so the prompt instructs the model to reproduce
	// every tag exactly. Source stays pure content either way.
	PreserveTags bool `json:"preserve_tags,omitempty"`
	// BlockContext is reference material about this block — its key, its
	// neighbours. The model reads it; it is never translated and never returned.
	//
	// (This replaces a `Context string` field that was declared and never once
	// filled — a placeholder for exactly this feature, left empty for years while
	// the model got no context at all.)
	BlockContext prompt.Context `json:"block_context,omitzero"`
}

// Prompt returns the prompt builder for this request. Prompt construction lives
// in core/ai/prompt so that every path — provider, tool, streaming, batch —
// renders identical bytes.
// PromptTurns renders the turns for this request, including its context.
func (req TranslateRequest) PromptTurns() []prompt.Turn {
	return req.Prompt().SingleWithContext(req.Source, req.PreserveTags, req.BlockContext)
}

func (req TranslateRequest) Prompt() prompt.Translate {
	return prompt.Translate{
		SourceLocale:   req.SourceLanguage,
		TargetLocale:   req.TargetLocale,
		Instruction:    req.Instruction,
		VoiceGuide:     req.VoiceGuide,
		PreferredTerms: req.PreferredTerms,
	}
}

// Directives returns the deterministic instruction + voice + terminology
// block shared by every translation prompt.
func (req TranslateRequest) Directives() string { return req.Prompt().Directives() }

// MessagesFromTurns converts a rendered prompt into provider messages. Each
// provider then transports the roles as its API requires.
func MessagesFromTurns(turns []prompt.Turn) []Message {
	msgs := make([]Message, 0, len(turns))
	for _, t := range turns {
		m := TextMessage(t.Role, t.Text)
		m.Sections = t.Sections
		msgs = append(msgs, m)
	}
	return msgs
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
	// Truncated reports that the model stopped at its output cap, so Translation
	// is the beginning of a translation rather than a translation. It carries the
	// same signal ChatResponse.Truncated does, because a caller that commits the
	// fragment as a finished target corrupts the document silently — the text
	// looks like a translation, just a shorter one.
	Truncated bool `json:"truncated,omitempty"`
}

// ChatResponse contains the chat result.
type ChatResponse struct {
	Content string     `json:"content"`
	Model   string     `json:"model"`
	Usage   TokenUsage `json:"usage"`
	// Truncated reports that the model stopped because it hit the output cap,
	// not because it was finished. The content is then a fragment — with
	// structured output, invalid JSON.
	//
	// This is surfaced rather than left to the caller's JSON decoder to trip
	// over, because the two failures need opposite responses: a malformed reply
	// is a model problem, while a truncated reply means we asked for more than
	// the model can emit and should ask for less.
	Truncated bool `json:"truncated,omitempty"`
}

// ErrOutputTruncated reports a reply cut short by the model's output cap. A
// caller that batches can catch this and retry with a smaller batch rather than
// failing the run.
var ErrOutputTruncated = errors.New("model output truncated: the reply hit the output token limit")

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
	// ClaudeCode runs prompts through the locally installed Claude Code CLI
	// (`claude`), billing the user's Claude subscription instead of an API
	// key. Keyless but not local — content still travels to Anthropic. See
	// claudecode.go.
	ClaudeCode ProviderID = "claude-code"
	// Demo is a deterministic, offline provider that returns illustrative
	// (not real-model) output. Used by the browser playground so AI commands
	// run with no API keys. See demo.go.
	Demo ProviderID = "demo"
)

// IsLocalProvider reports whether a registered provider keeps content on the
// machine (no remote egress). It is a registration property — a provider sets
// ProviderInfo.Local at RegisterProvider time — not a hardcoded list, so the
// framework needs no knowledge of specific local backends. Plugin-registered
// providers declare themselves local the same way the built-in Ollama/Demo do.
// An unregistered or
// non-local provider returns false (cloud, fail-closed). The flow placement pass
// uses this to refine a tool's remote-source-egress side effect (AD-006).
func IsLocalProvider(id ProviderID) bool {
	globalProvidersMu.RLock()
	defer globalProvidersMu.RUnlock()
	for _, reg := range globalProviders {
		if reg.Info.Name == id || slices.Contains(reg.Aliases, id) {
			return reg.Info.Local
		}
	}
	return false
}

// ProviderFactory creates an LLMProvider from a Config.
type ProviderFactory func(cfg Config) LLMProvider

// ProviderInfo describes a registered AI provider.
type ProviderInfo struct {
	// Name is the provider identifier (e.g. "anthropic").
	Name ProviderID
	// Label is the human-readable display name (e.g. "Anthropic").
	Label string
	// Local reports that the provider runs on-device with no remote egress and
	// needs no API key (Ollama, Demo, and plugin providers like Gemma). Drives
	// IsLocalProvider and the keyless credential/UX paths.
	Local bool
	// Keyless reports that the provider needs no API key even though it may
	// egress content (claude-code authenticates via the local Claude Code
	// login). Local providers are implicitly keyless; use ProviderInfo.NeedsKey
	// to combine the two.
	Keyless bool
	// Subscription reports that usage is billed to a personal subscription the
	// user already pays for (no per-token cost) — drives the "runs on your
	// subscription" wording in plans instead of a $ estimate.
	Subscription bool
	// DefaultModel is the model a fresh provider uses when the caller names none.
	// It is the same value each provider applies as its Config.Model fallback,
	// surfaced here so `kapi models` can list a provider's default without
	// constructing it. Empty when the provider has no built-in default.
	DefaultModel string
	// ModelPrefixes are lower-case model-name prefixes that uniquely identify
	// this provider (e.g. "claude" → anthropic, "gpt" → openai). ProviderForModel
	// uses them to infer a provider from a bare model name — the "name a model,
	// get a provider" convention. Local backends like Ollama leave this empty and
	// are matched as the catch-all (see ProviderForModel).
	ModelPrefixes []string
}

// NeedsKey reports whether the provider requires an API key: cloud providers
// do; local (Ollama, Demo) and keyless-subscription (claude-code) ones do not.
func (i ProviderInfo) NeedsKey() bool { return !i.Local && !i.Keyless }

// ProviderInfoFor returns the registered metadata for a provider id (aliases
// included). ok is false for unregistered providers.
func ProviderInfoFor(id ProviderID) (ProviderInfo, bool) {
	globalProvidersMu.RLock()
	defer globalProvidersMu.RUnlock()
	for _, reg := range globalProviders {
		if reg.Info.Name == id || slices.Contains(reg.Aliases, id) {
			return reg.Info, true
		}
	}
	return ProviderInfo{}, false
}

// providerRegistration bundles a factory with metadata.
type providerRegistration struct {
	Info    ProviderInfo
	Factory ProviderFactory
	Aliases []ProviderID // alternative names (e.g., "azure_openai" → AzureOpenAI)
}

// globalProviders is the default provider registry, populated by init().
// Plugins register after init() (possibly from other goroutines), so every
// access goes through globalProvidersMu — the same locked-registry pattern
// mtprovider uses for its config factories.
var (
	globalProvidersMu sync.RWMutex
	globalProviders   []providerRegistration
)

func init() {
	RegisterProvider(ProviderInfo{Name: Anthropic, Label: "Anthropic", DefaultModel: DefaultAnthropicModel, ModelPrefixes: []string{"claude"}},
		func(cfg Config) LLMProvider { return NewAnthropicProvider(cfg) })
	RegisterProvider(ProviderInfo{Name: OpenAI, Label: "OpenAI", DefaultModel: DefaultOpenAIModel, ModelPrefixes: []string{"gpt", "o1", "o3", "o4", "chatgpt"}},
		func(cfg Config) LLMProvider { return NewOpenAIProvider(cfg) })
	RegisterProvider(ProviderInfo{Name: Gemini, Label: "Gemini", DefaultModel: DefaultGeminiModel, ModelPrefixes: []string{"gemini"}},
		func(cfg Config) LLMProvider { return NewGeminiProvider(cfg) })
	// Azure shares OpenAI's model names but is endpoint-specific, so it declares
	// no prefixes — inferring it from "gpt-*" would be wrong. Choose it explicitly.
	RegisterProviderWithAliases(ProviderInfo{Name: AzureOpenAI, Label: "Azure OpenAI", DefaultModel: DefaultAzureOpenAIModel},
		func(cfg Config) LLMProvider { return NewAzureOpenAIProvider(cfg) },
		"azure_openai")
	// claude-code declares no ModelPrefixes: "claude-*" ids belong to the
	// anthropic API provider, and the bare aliases (sonnet, opus, haiku) are
	// too generic to infer from. Choose it explicitly (or via kapi models setup).
	RegisterProvider(ProviderInfo{Name: ClaudeCode, Label: "Claude Code", Keyless: true, Subscription: true, DefaultModel: DefaultClaudeCodeModel},
		func(cfg Config) LLMProvider { return NewClaudeCodeProvider(cfg) })
	RegisterProvider(ProviderInfo{Name: Ollama, Label: "Ollama", Local: true, DefaultModel: DefaultOllamaModel},
		func(cfg Config) LLMProvider { return NewOllamaProvider(cfg) })
	RegisterProvider(ProviderInfo{Name: Demo, Label: "Demo (illustrative)", Local: true},
		func(cfg Config) LLMProvider { return NewDemoProvider(cfg) })
}

// ProviderForModel infers the provider that serves a given model name, by
// convention, so callers can name only a model and let the provider follow —
// e.g. "claude-sonnet-4" → anthropic, "gemma3:4b" → ollama. Resolution order:
//
//  1. A registered provider whose ModelPrefixes match the (lower-cased) name.
//  2. Otherwise the Ollama on-device catch-all when the name looks like an
//     Ollama tag ("name:tag") or is a recommended local model.
//
// Returns ("", false) when no confident inference is possible (e.g. an
// ambiguous bare name) — the caller should then ask the user or require an
// explicit provider. Azure OpenAI is never inferred (it shares gpt-* names but
// needs an endpoint), so a gpt-* model resolves to OpenAI.
func ProviderForModel(modelName string) (ProviderID, bool) {
	m := strings.ToLower(strings.TrimSpace(modelName))
	if m == "" {
		return "", false
	}
	globalProvidersMu.RLock()
	for _, reg := range globalProviders {
		for _, prefix := range reg.Info.ModelPrefixes {
			if strings.HasPrefix(m, prefix) {
				globalProvidersMu.RUnlock()
				return reg.Info.Name, true
			}
		}
	}
	globalProvidersMu.RUnlock()
	// Ollama tags are "<name>:<tag>" (gemma3:4b, llama3.2:3b). Anything that
	// looks like one — or is a curated local pick — is the on-device catch-all.
	if strings.Contains(m, ":") {
		return Ollama, true
	}
	for _, r := range RecommendedOllamaModels {
		if strings.EqualFold(r.Name, modelName) {
			return Ollama, true
		}
	}
	return "", false
}

// RegisterProvider registers a new AI provider factory. Plugins can call this
// to add custom providers that will appear in tool schemas and CLI flags.
func RegisterProvider(info ProviderInfo, factory ProviderFactory) {
	globalProvidersMu.Lock()
	defer globalProvidersMu.Unlock()
	globalProviders = append(globalProviders, providerRegistration{
		Info:    info,
		Factory: factory,
	})
}

// RegisterProviderWithAliases registers a provider with alternative name aliases.
func RegisterProviderWithAliases(info ProviderInfo, factory ProviderFactory, aliases ...ProviderID) {
	globalProvidersMu.Lock()
	defer globalProvidersMu.Unlock()
	globalProviders = append(globalProviders, providerRegistration{
		Info:    info,
		Factory: factory,
		Aliases: aliases,
	})
}

// NewProvider creates an LLMProvider by looking up the registered factory for
// the given provider name. Returns an error if the provider is not registered.
func NewProvider(name ProviderID, cfg Config) (LLMProvider, error) {
	// Resolve the factory under the read lock, but call it (and build the
	// not-found error, which re-enters the registry via ProviderNames) outside
	// it — a factory registering another provider must not deadlock.
	var factory ProviderFactory
	globalProvidersMu.RLock()
	for _, reg := range globalProviders {
		if reg.Info.Name == name || slices.Contains(reg.Aliases, name) {
			factory = reg.Factory
			break
		}
	}
	globalProvidersMu.RUnlock()
	if factory == nil {
		return nil, fmt.Errorf("unknown AI provider: %s (supported: %s)", name, strings.Join(ProviderNames(), ", "))
	}
	p := factory(cfg)
	// Wrapping here rather than at each call site gives --explain complete
	// coverage, including providers contributed by plugins.
	if recordersInstalled() {
		p = withRecording(p)
	}
	return p, nil
}

// Providers returns the list of available AI providers in display order.
// This is the canonical source of truth for provider names — used by tool
// schemas, CLI flags, and UI dropdowns.
func Providers() []ProviderInfo {
	globalProvidersMu.RLock()
	defer globalProvidersMu.RUnlock()
	infos := make([]ProviderInfo, len(globalProviders))
	for i, reg := range globalProviders {
		infos[i] = reg.Info
	}
	return infos
}

// ProviderNames returns just the provider name strings.
func ProviderNames() []string {
	globalProvidersMu.RLock()
	defer globalProvidersMu.RUnlock()
	names := make([]string, len(globalProviders))
	for i, reg := range globalProviders {
		names[i] = string(reg.Info.Name)
	}
	return names
}

// Config holds common provider configuration.
type Config struct {
	APIKey    string `json:"api_key"`
	BaseURL   string `json:"base_url,omitempty"`
	Model     string `json:"model"`
	MaxTokens int    `json:"max_tokens,omitempty"`

	// Temperature is the sampling temperature, or nil to leave the provider's
	// own default alone.
	//
	// A pointer because 0 is a meaningful value: it asks for greedy decoding,
	// which is exactly what an eval that wants to be reproducible should set.
	// As a bare float64 with `omitempty`, 0 and "unset" were the same thing and
	// greedy was unreachable — Bedrock still spells that mistake as
	// `if cfg.Temperature > 0`.
	//
	// Whether it reaches the wire is a per-provider question, and the answer
	// used to be no for four of six: Anthropic, OpenAI, Azure and Gemini
	// accepted the field and never sent it, so a caller asking for determinism
	// silently got whatever the API defaults to. TestEveryProviderSendsTemperature
	// exists to keep that from coming back.
	//
	// Set it with new(expr): new(0.0) for greedy, new(0.7) for sampled.
	Temperature *float64 `json:"temperature,omitempty"`
}

// Validate checks that the config has required fields.
func (c *Config) Validate() error {
	if c.Model == "" {
		return errors.New("model is required")
	}
	return nil
}

// StandardTranslate renders the translate prompt and runs it through a
// provider's Chat method. Every provider's Translate delegates here, differing
// only in the confidence it reports — no provider carries a prompt of its own.
// A provider written outside this package (a plugin's, a test's) delegates here
// too, or it will diverge from the built-ins in both prompt and recording.
//
// It records the exchange itself: a provider's Translate calls that provider's
// own Chat, which bypasses the recording wrapper, so this is the only point that
// sees the call. (The wrapper still records direct Chat/ChatStructured calls —
// batch translation, voice profile, entity extraction — and the two cannot
// double-count, because the Chat reached from here is the unwrapped inner one.)
func StandardTranslate(
	ctx context.Context,
	name ProviderID,
	chat func(context.Context, []Message) (*ChatResponse, error),
	req TranslateRequest,
	confidence float64,
) (*TranslateResponse, error) {
	p := req.Prompt()
	turns := p.SingleWithContext(req.Source, req.PreserveTags, req.BlockContext)
	ctx = prompt.WithMeta(ctx, p.Meta(prompt.IDTranslateSingle))
	msgs := MessagesFromTurns(turns)

	resp, err := chat(ctx, msgs)
	record(ctx, name, msgs, nil, resp, err)
	if err != nil {
		return nil, err
	}
	return &TranslateResponse{
		Translation: resp.Content,
		Confidence:  confidence,
		Model:       resp.Model,
		Usage:       resp.Usage,
		Truncated:   resp.Truncated,
	}, nil
}
