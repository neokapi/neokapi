// Package asr is the framework seam for automatic speech recognition — turning
// recorded speech (audio) into timing-anchored text Blocks, the audio
// counterpart of core/vision's OCR (AD-030). It defines the Engine interface and
// a name-keyed engine registry, mirroring core/vision and core/segment.
//
// The interface is intentionally small so backends plug in: the out-of-process
// kapi-asr plugin (cgo + a Whisper-family ONNX model) is the native one; a
// browser/WASM build can register a transformers.js-backed engine. Like vision,
// the engine is PATH-based: the host passes an audio file path, never bytes, so
// the audio lives only in the engine's process.
package asr

import (
	"context"
	"errors"

	"github.com/neokapi/neokapi/core/engine"
)

// Segment is one recognized span of speech: its text, its time bounds in
// milliseconds from the start of the media, and the model's confidence in [0,1].
type Segment struct {
	Text       string
	StartMS    int64
	EndMS      int64
	Confidence float64
}

// Result is the recognized speech of one audio input: the ordered segments plus
// the detected (or configured) language, where the engine reports it.
type Result struct {
	Segments []Segment
	Language string
}

// Options tunes transcription. All fields are advisory.
type Options struct {
	// Lang is an advisory language hint (BCP-47, e.g. "en", "nb"); empty lets the
	// engine auto-detect.
	Lang string
}

// Engine transcribes audio files. Implementations are typically backed by the
// out-of-process kapi-asr plugin and load models lazily. An Engine is used
// sequentially by one caller; callers Close it when done.
//
// Transcribe takes a filesystem PATH, not bytes, by design: the host must never
// load a large audio track into memory. The plugin opens and decodes the file
// itself, so the audio bytes live only in the plugin process.
type Engine interface {
	// Transcribe recognizes speech in the audio file at audioPath. The path must
	// be readable by the engine's process (the local filesystem).
	Transcribe(ctx context.Context, audioPath string, opts Options) (*Result, error)
	// Close releases the engine (e.g. terminates the plugin subprocess).
	Close() error
}

// Factory opens an Engine, performing whatever discovery/spawn the backend needs
// (e.g. locating and launching the kapi-asr plugin).
type Factory func() (Engine, error)

// ErrNoEngine is returned by Open when no ASR engine is registered — the
// kapi-asr plugin is not installed, or no host wired one up.
var ErrNoEngine = errors.New("asr: no engine registered (install the kapi-asr plugin)")

// registry holds the ASR engine factories. The package-level functions below
// delegate to it, keeping the seam's small API while sharing the mechanism with
// core/vision via core/engine.
var registry = engine.NewRegistry[Engine]("asr", ErrNoEngine)

// RegisterEngine registers a named engine factory. The first engine registered
// becomes the default. Registering a duplicate name overwrites it. A host wires
// the engine that discovers and drives the plugin; framework-only builds
// register none, so ASR is absent.
func RegisterEngine(name string, f Factory) { registry.Register(name, f) }

// Available reports whether the named engine ("" = default) is registered.
func Available(name string) bool { return registry.Available(name) }

// Open opens the named engine ("" = default), returning ErrNoEngine if none is
// registered. The caller owns the returned Engine and must Close it.
func Open(name string) (Engine, error) { return registry.Open(name) }

// ResetForTest clears the registry. It exists for tests that register a fake
// engine and must not leak it across cases.
func ResetForTest() { registry.ResetForTest() }
