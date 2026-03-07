package config

import (
	"fmt"
	"sync"
)

// Transformer converts a spec from one apiVersion namespace to another.
type Transformer interface {
	Transform(spec map[string]any) (map[string]any, error)
}

// TransformerFunc is a function adapter for Transformer.
type TransformerFunc func(spec map[string]any) (map[string]any, error)

func (f TransformerFunc) Transform(spec map[string]any) (map[string]any, error) { return f(spec) }

// TransformRegistry maps (from, to) apiVersion pairs to transformers.
type TransformRegistry struct {
	mu         sync.RWMutex
	transforms map[string]map[string]Transformer // from → to → transformer
}

// NewTransformRegistry creates an empty TransformRegistry.
func NewTransformRegistry() *TransformRegistry {
	return &TransformRegistry{
		transforms: make(map[string]map[string]Transformer),
	}
}

// Register adds a transformer for converting from one apiVersion to another.
func (r *TransformRegistry) Register(from, to string, t Transformer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.transforms[from] == nil {
		r.transforms[from] = make(map[string]Transformer)
	}
	r.transforms[from][to] = t
}

// Transform converts a spec from one apiVersion to another using a registered transformer.
func (r *TransformRegistry) Transform(from, to string, spec map[string]any) (map[string]any, error) {
	r.mu.RLock()
	targets, ok := r.transforms[from]
	if !ok {
		r.mu.RUnlock()
		return nil, fmt.Errorf("no transforms registered from %q", from)
	}
	t, ok := targets[to]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("no transform registered from %q to %q", from, to)
	}
	return t.Transform(spec)
}

// Has reports whether a transform is registered for the given (from, to) pair.
func (r *TransformRegistry) Has(from, to string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	targets, ok := r.transforms[from]
	if !ok {
		return false
	}
	_, ok = targets[to]
	return ok
}

// DefaultTransforms is the global transform registry.
var DefaultTransforms = NewTransformRegistry()
