package config

import (
	"fmt"
	"sync"
)

// Transformer converts a spec from one kind to another.
type Transformer interface {
	Transform(spec map[string]any) (map[string]any, error)
}

// TransformerFunc is a function adapter for Transformer.
type TransformerFunc func(spec map[string]any) (map[string]any, error)

func (f TransformerFunc) Transform(spec map[string]any) (map[string]any, error) { return f(spec) }

// TransformRegistry maps (fromKind, toKind) pairs to transformers.
type TransformRegistry struct {
	mu         sync.RWMutex
	transforms map[Kind]map[Kind]Transformer
}

// NewTransformRegistry creates an empty TransformRegistry.
func NewTransformRegistry() *TransformRegistry {
	return &TransformRegistry{
		transforms: make(map[Kind]map[Kind]Transformer),
	}
}

// Register adds a transformer for converting from one kind to another.
func (r *TransformRegistry) Register(from, to Kind, t Transformer) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.transforms[from] == nil {
		r.transforms[from] = make(map[Kind]Transformer)
	}
	r.transforms[from][to] = t
}

// Transform converts a spec from one kind to another using a registered transformer.
func (r *TransformRegistry) Transform(from, to Kind, spec map[string]any) (map[string]any, error) {
	r.mu.RLock()
	targets, ok := r.transforms[from]
	if !ok {
		r.mu.RUnlock()
		return nil, fmt.Errorf("no transforms registered from kind %q", from)
	}
	t, ok := targets[to]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("no transform registered from kind %q to %q", from, to)
	}
	return t.Transform(spec)
}

// Has reports whether a transform is registered for the given (from, to) pair.
func (r *TransformRegistry) Has(from, to Kind) bool {
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
