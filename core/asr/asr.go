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
	"fmt"
	"sync"
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

var (
	mu          sync.RWMutex
	factories   = map[string]Factory{}
	defaultName string
)

// RegisterEngine registers a named engine factory. The first engine registered
// becomes the default. Registering a duplicate name overwrites it. A host wires
// the engine that discovers and drives the plugin; framework-only builds
// register none, so ASR is absent.
func RegisterEngine(name string, f Factory) {
	mu.Lock()
	defer mu.Unlock()
	if f == nil {
		return
	}
	factories[name] = f
	if defaultName == "" {
		defaultName = name
	}
}

// Available reports whether the named engine ("" = default) is registered.
func Available(name string) bool {
	mu.RLock()
	defer mu.RUnlock()
	if name == "" {
		name = defaultName
	}
	if name == "" {
		return false
	}
	_, ok := factories[name]
	return ok
}

// Open opens the named engine ("" = default), returning ErrNoEngine if none is
// registered. The caller owns the returned Engine and must Close it.
func Open(name string) (Engine, error) {
	mu.RLock()
	if name == "" {
		name = defaultName
	}
	f, ok := factories[name]
	mu.RUnlock()
	if !ok {
		if name == "" {
			return nil, ErrNoEngine
		}
		return nil, fmt.Errorf("asr: engine %q not registered: %w", name, ErrNoEngine)
	}
	return f()
}

// ResetForTest clears the registry. It exists for tests that register a fake
// engine and must not leak it across cases.
func ResetForTest() {
	mu.Lock()
	defer mu.Unlock()
	factories = map[string]Factory{}
	defaultName = ""
}
