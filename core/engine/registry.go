// Package engine provides a generic registry of named factories for a
// plugin-backed capability — a speech recognizer (core/asr), an OCR engine
// (core/vision), and any future one with the same shape. A framework-only build
// registers no factory, so the capability is simply absent; a host wires the
// factory that discovers and drives the plugin.
package engine

import (
	"fmt"
	"sync"
)

// Registry is a thread-safe registry of named factories producing engines of
// type T. The first engine registered becomes the default; registering a
// duplicate name overwrites it.
type Registry[T any] struct {
	// label prefixes error messages (e.g. "asr").
	label string
	// noEngine is returned by Open when nothing is registered; it names the
	// capability and how to obtain it (e.g. install the plugin).
	noEngine error

	mu          sync.RWMutex
	factories   map[string]func() (T, error)
	defaultName string
}

// NewRegistry creates a Registry. label prefixes the "engine %q not registered"
// error; noEngine is returned by Open when nothing is registered.
func NewRegistry[T any](label string, noEngine error) *Registry[T] {
	return &Registry[T]{
		label:     label,
		noEngine:  noEngine,
		factories: map[string]func() (T, error){},
	}
}

// Register registers a named engine factory. The first one registered becomes
// the default. A nil factory is ignored.
func (r *Registry[T]) Register(name string, f func() (T, error)) {
	if f == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[name] = f
	if r.defaultName == "" {
		r.defaultName = name
	}
}

// Available reports whether the named engine ("" = default) is registered.
func (r *Registry[T]) Available(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if name == "" {
		name = r.defaultName
	}
	if name == "" {
		return false
	}
	_, ok := r.factories[name]
	return ok
}

// Open opens the named engine ("" = default), returning the registry's
// noEngine error when none is registered. The caller owns the returned engine.
func (r *Registry[T]) Open(name string) (T, error) {
	r.mu.RLock()
	if name == "" {
		name = r.defaultName
	}
	f, ok := r.factories[name]
	r.mu.RUnlock()
	if !ok {
		var zero T
		if name == "" {
			return zero, r.noEngine
		}
		return zero, fmt.Errorf("%s: engine %q not registered: %w", r.label, name, r.noEngine)
	}
	return f()
}

// ResetForTest clears the registry so a test's fake engine does not leak across
// cases.
func (r *Registry[T]) ResetForTest() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories = map[string]func() (T, error){}
	r.defaultName = ""
}
