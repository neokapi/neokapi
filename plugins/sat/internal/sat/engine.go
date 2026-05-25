package sat

import "errors"

// Engine segments text into interior sentence boundaries (rune offsets) using
// a SaT model. Implementations load models lazily and cache them per name.
//
// Two implementations exist:
//   - the real ONNX-backed engine (build tag `onnx`, in engine_onnx.go), which
//     links the onnxruntime and tokenizer native libraries; and
//   - the stub engine (default build, in engine_stub.go), which compiles
//     without any native dependency and returns ErrNoONNX from Segment so the
//     module builds and the protocol/algorithm tests stay green on machines
//     without the native libraries.
type Engine interface {
	// Segment returns interior sentence-boundary rune offsets into text using
	// the named model (empty = default) at the given threshold (0 = model
	// default). lang is an advisory hint.
	Segment(text, model, lang string, threshold float64) ([]int, error)
	// Loaded reports whether the named model is currently resident.
	Loaded(model string) bool
	// Close releases all loaded models and the runtime environment.
	Close() error
}

// ErrNoONNX is returned by the stub engine: the binary was built without the
// `onnx` build tag, so no real segmentation backend is linked in.
var ErrNoONNX = errors.New("sat: built without ONNX support (rebuild with -tags onnx and the onnxruntime + tokenizer native libraries)")

// resolveThreshold returns the effective threshold: the model default when t
// is zero (or out of range), else t. It is consumed by the ONNX-backed engine
// (engine_onnx.go); the unused linter cannot see that cross-build-tag use.
//
//nolint:unused // used by the onnx-tagged engine
func resolveThreshold(t float64) float64 {
	if t <= 0 || t >= 1 {
		return DefaultThreshold
	}
	return t
}
