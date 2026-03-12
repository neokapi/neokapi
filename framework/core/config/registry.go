package config

import (
	"fmt"
	"sync"
)

// SpecDecoder decodes a raw spec map into a typed configuration value.
type SpecDecoder interface {
	Decode(spec map[string]any) (any, error)
}

// SpecDecoderFunc is a function adapter for SpecDecoder.
type SpecDecoderFunc func(spec map[string]any) (any, error)

func (f SpecDecoderFunc) Decode(spec map[string]any) (any, error) { return f(spec) }

// Registry maps Kind values to their SpecDecoders.
type Registry struct {
	mu       sync.RWMutex
	decoders map[Kind]SpecDecoder
}

// NewRegistry creates a new empty Registry.
func NewRegistry() *Registry {
	return &Registry{
		decoders: make(map[Kind]SpecDecoder),
	}
}

// Register associates a Kind with a decoder.
func (r *Registry) Register(kind Kind, decoder SpecDecoder) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.decoders[kind] = decoder
}

// Decode looks up the decoder for the envelope's Kind and decodes the spec.
func (r *Registry) Decode(env *Envelope) (any, error) {
	r.mu.RLock()
	decoder, ok := r.decoders[env.Kind]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("no decoder registered for kind %q", env.Kind)
	}
	return decoder.Decode(env.Spec)
}

// Has reports whether a decoder is registered for the given Kind.
func (r *Registry) Has(kind Kind) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.decoders[kind]
	return ok
}

// DefaultRegistry is the global spec decoder registry.
var DefaultRegistry = NewRegistry()
