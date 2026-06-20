// Package llm implements local text generation with Gemma 4: tokenize a
// conversation with the Gemma SentencePiece tokenizer, run the split ONNX export
// (embed_tokens → optional vision/audio encoders → decoder_model_merged) in an
// autoregressive loop, sample the next token, and decode the result.
//
// The file algo pieces that are deterministic and dependency-free — chat
// templating, sampling, stop-sequence detection — live in chat.go and sample.go
// and build and are unit-tested without the ONNX runtime or tokenizer native
// libraries. The cgo-backed engine lives behind the `onnx` build tag in
// engine_onnx.go; the default build uses engine_stub.go.
package llm

import "errors"

// Media references one non-text input by local filesystem path.
type Media struct {
	Kind string // "image", "audio", "video"
	Path string
	MIME string
}

// Message is one conversation turn handed to the engine.
type Message struct {
	Role  string // "system", "user", "assistant"
	Text  string
	Media []Media
}

// GenerateParams bundles a generation request for the engine.
type GenerateParams struct {
	Messages    []Message
	Model       string // empty = default
	MaxTokens   int    // 0 = engine default
	Temperature float64
	TopP        float64
	Schema      []byte // optional JSON schema (best-effort steering)
}

// Result is a completed generation.
type Result struct {
	Text         string
	InputTokens  int
	OutputTokens int
}

// Engine generates text from a conversation using a Gemma model. Implementations
// load models lazily and cache them per name.
//
// Two implementations exist:
//   - the real ONNX-backed engine (build tag `onnx`, in engine_onnx.go), which
//     links the onnxruntime and tokenizer native libraries; and
//   - the stub engine (default build, in engine_stub.go), which compiles without
//     any native dependency and returns ErrNoONNX from Generate so the module
//     builds and the algorithm tests stay green on machines without the native
//     libraries.
type Engine interface {
	// Generate runs the model on params and returns the completion.
	Generate(params GenerateParams) (Result, error)
	// Loaded reports whether the named model is currently resident.
	Loaded(model string) bool
	// Modalities returns the non-text input modalities the engine accepts
	// ("image", "audio", "video").
	Modalities() []string
	// Close releases all loaded models and the runtime environment.
	Close() error
}

// ErrNoONNX is returned by the stub engine: the binary was built without the
// `onnx` build tag, so no real generation backend is linked in.
var ErrNoONNX = errors.New("llm: built without ONNX support (rebuild with -tags onnx and the onnxruntime + tokenizer native libraries)")

// DefaultMaxTokens bounds a generation when the request does not. Gemma 4
// supports a 128K context; this is a practical per-call output budget.
const DefaultMaxTokens = 1024
