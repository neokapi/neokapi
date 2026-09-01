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

// Recorder observes one LLM call. It receives the call's context, so a recorder
// can read whatever the caller stamped there (a prompt id via prompt.Meta, or a
// host's own scope) and attribute the exchange to the work that made it.
type Recorder func(ctx context.Context, ex Exchange)

// recorders are the process-wide observers of LLM calls. It is a diagnostic
// hook, like tracing: empty on an ordinary run, which then pays nothing.
//
// A list rather than one slot, because two observers legitimately want the same
// calls and neither may silently displace the other. `kapi --explain-prompts`
// collects one run's exchanges and renders them at the end; a desktop session
// keeps a rolling activity log for the whole time the app is open. A single slot
// meant whichever started second turned the first one off, and the only symptom
// was a transcript that came back empty.
var (
	recorderMu sync.RWMutex
	recorders  []*Recorder
)

// AddRecorder registers an observer of every LLM call and returns the function
// that removes it. Every provider built through NewProvider while at least one
// recorder is registered is wrapped to feed them.
//
// A provider already constructed is NOT retroactively wrapped, so register
// before the work starts.
func AddRecorder(fn Recorder) (remove func()) {
	if fn == nil {
		return func() {}
	}
	handle := &fn
	recorderMu.Lock()
	recorders = append(recorders, handle)
	recorderMu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			recorderMu.Lock()
			defer recorderMu.Unlock()
			for i, h := range recorders {
				if h == handle {
					recorders = append(recorders[:i], recorders[i+1:]...)
					return
				}
			}
		})
	}
}

// currentRecorders returns the registered observers.
func currentRecorders() []Recorder {
	recorderMu.RLock()
	defer recorderMu.RUnlock()
	if len(recorders) == 0 {
		return nil
	}
	out := make([]Recorder, 0, len(recorders))
	for _, h := range recorders {
		out = append(out, *h)
	}
	return out
}

// recordersInstalled reports whether any observer is registered.
func recordersInstalled() bool {
	recorderMu.RLock()
	defer recorderMu.RUnlock()
	return len(recorders) > 0
}

// record emits an exchange to every registered observer.
func record(ctx context.Context, providerName ProviderID, msgs []Message, schema *JSONSchema, resp *ChatResponse, err error) {
	recs := currentRecorders()
	if len(recs) == 0 {
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
	for _, rec := range recs {
		rec(ctx, ex)
	}
}

// recordingProvider captures every call made through it. It deliberately does
// not override Translate: each provider implements Translate as StandardTranslate
// over its own Chat, which would bypass this wrapper's Chat — so the Translate
// exchange is recorded inside StandardTranslate instead, and recording it here
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

// Recording wraps a provider so every call it serves is reported to the
// registered recorders, preserving streaming support.
//
// NewProvider applies this on its own while a recorder is registered, which
// covers every provider the recipe or the config builds, plugin-contributed
// ones included. Use it directly for a provider constructed some other way:
// an embedded host that holds its own client, or a test that hands a tool a
// mock and still wants the exchange observed.
//
// A decorator like Traced, and for the same reason: the behaviour is identical
// for every backend, so one wrapper cannot drift between them.
func Recording(p LLMProvider) LLMProvider {
	if p == nil {
		return nil
	}
	return withRecording(p)
}

// withRecording wraps p so its calls are captured, preserving streaming support.
func withRecording(p LLMProvider) LLMProvider {
	base := &recordingProvider{LLMProvider: p}
	if s, ok := p.(StreamingLLMProvider); ok {
		return &recordingStreamingProvider{recordingProvider: base, streaming: s}
	}
	return base
}
