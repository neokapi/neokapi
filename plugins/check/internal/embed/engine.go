// Package embed turns text into a sentence embedding using a small multilingual
// ONNX model, and is the basis for the voice/style-similarity checker. The real
// engine (build tag `onnx`, engine_onnx.go) links onnxruntime + the tokenizer;
// the default build (engine_stub.go) links no native libraries and returns
// ErrNoONNX, so the module and its pure-Go tests build everywhere.
package embed

import "errors"

// Engine produces embeddings from text using a named model (empty = default).
type Engine interface {
	// Embed returns an L2-normalized sentence embedding for text.
	Embed(text, model string) ([]float32, error)
	// Loaded reports whether the named model is resident.
	Loaded(model string) bool
	// Close releases loaded models and the runtime.
	Close() error
}

// ErrNoONNX is returned by the stub engine: the binary was built without the
// `onnx` build tag, so no real embedding backend is linked in.
var ErrNoONNX = errors.New("check: built without ONNX support (rebuild with -tags onnx and the onnxruntime + tokenizer native libraries)")
