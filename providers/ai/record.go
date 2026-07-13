package aiprovider

import (
	"context"
	"sync"

	"github.com/neokapi/neokapi/core/ai/prompt"
)

// Exchange is a single LLM call: the messages actually sent on the wire, and
// what came back. It is what `kapi ... --explain` renders — the answer to
// "what does kapi send to the model on my behalf?".
type Exchange struct {
	// Prompt is the prompt ID (e.g. prompt.IDTranslateSingle), and Version the
	// prompt revision. Both are empty when the caller attached no prompt.Meta.
	Prompt  string `json:"prompt,omitempty"`
	Version string `json:"prompt_version,omitempty"`

	Provider string `json:"provider"`
	Model    string `json:"model,omitempty"`

	// Messages are the messages handed to the provider. What finally reaches the
	// vendor is this, transported per that vendor's API (Anthropic lifts the
	// system turn into a top-level parameter, and so on).
	Messages []Message `json:"messages"`
	// Schema is the structured-output contract, when the call constrains output.
	Schema *JSONSchema `json:"schema,omitempty"`

	Response string     `json:"response,omitempty"`
	Usage    TokenUsage `json:"usage,omitzero"`
	Err      string     `json:"error,omitempty"`
}

// recorder is the process-wide observer of LLM calls. It is a diagnostic hook,
// like tracing: set once when the user asks to see prompts, nil otherwise, so
// an ordinary run pays nothing for it.
var (
	recorderMu sync.RWMutex
	recorder   func(Exchange)
)

// SetRecorder installs (or clears, with nil) the process-wide LLM call recorder.
// Every provider built through NewProvider is wrapped while one is set.
func SetRecorder(fn func(Exchange)) {
	recorderMu.Lock()
	defer recorderMu.Unlock()
	recorder = fn
}

// currentRecorder returns the installed recorder, or nil.
func currentRecorder() func(Exchange) {
	recorderMu.RLock()
	defer recorderMu.RUnlock()
	return recorder
}

// record emits an exchange when a recorder is installed.
func record(ctx context.Context, providerName ProviderID, msgs []Message, schema *JSONSchema, resp *ChatResponse, err error) {
	rec := currentRecorder()
	if rec == nil {
		return
	}

	ex := Exchange{
		Provider: string(providerName),
		Messages: msgs,
		Schema:   schema,
	}
	if m, ok := prompt.MetaFrom(ctx); ok {
		ex.Prompt = m.ID
		ex.Version = m.Version
	}
	if resp != nil {
		ex.Model = resp.Model
		ex.Response = resp.Content
		ex.Usage = resp.Usage
	}
	if err != nil {
		ex.Err = err.Error()
	}
	rec(ex)
}

// recordingProvider captures every call made through it. It deliberately does
// not override Translate: each provider implements Translate as standardTranslate
// over its own Chat, which would bypass this wrapper's Chat — so the Translate
// exchange is recorded inside standardTranslate instead, and recording it here
// too would double-count it.
type recordingProvider struct {
	LLMProvider
}

func (w *recordingProvider) Chat(ctx context.Context, messages []Message) (*ChatResponse, error) {
	resp, err := w.LLMProvider.Chat(ctx, messages)
	record(ctx, w.Name(), messages, nil, resp, err)
	return resp, err
}

func (w *recordingProvider) ChatStructured(ctx context.Context, messages []Message, schema JSONSchema) (*ChatResponse, error) {
	resp, err := w.LLMProvider.ChatStructured(ctx, messages, schema)
	record(ctx, w.Name(), messages, &schema, resp, err)
	return resp, err
}

// recordingStreamingProvider preserves the StreamingLLMProvider interface: the
// translate tool type-asserts for it to surface live thinking progress, so a
// wrapper that dropped it would silently disable streaming.
type recordingStreamingProvider struct {
	*recordingProvider
	streaming StreamingLLMProvider
}

func (w *recordingStreamingProvider) ChatStream(ctx context.Context, messages []Message, onEvent func(ChatStreamEvent)) (*ChatResponse, error) {
	resp, err := w.streaming.ChatStream(ctx, messages, onEvent)
	record(ctx, w.Name(), messages, nil, resp, err)
	return resp, err
}

func (w *recordingStreamingProvider) ChatStructuredStream(ctx context.Context, messages []Message, schema JSONSchema, onEvent func(ChatStreamEvent)) (*ChatResponse, error) {
	resp, err := w.streaming.ChatStructuredStream(ctx, messages, schema, onEvent)
	record(ctx, w.Name(), messages, &schema, resp, err)
	return resp, err
}

// withRecording wraps p so its calls are captured, preserving streaming support.
func withRecording(p LLMProvider) LLMProvider {
	base := &recordingProvider{LLMProvider: p}
	if s, ok := p.(StreamingLLMProvider); ok {
		return &recordingStreamingProvider{recordingProvider: base, streaming: s}
	}
	return base
}
