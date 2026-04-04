package connector

import (
	"fmt"
	"sync"
)

// Factory creates a new IntegrationConnector instance with the given configuration.
type Factory func(config map[string]string) (IntegrationConnector, error)

// Info describes a registered connector type.
type Info struct {
	Name     string
	Category Category
}

// ConnectorInfo is a deprecated alias for [Info].
//
// Deprecated: Use [Info] instead.
type ConnectorInfo = Info

// Registry manages connector factories.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]Factory
	infos     map[string]Info
}

// NewRegistry creates an empty connector registry.
func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[string]Factory),
		infos:     make(map[string]Info),
	}
}

// Register adds a connector factory to the registry.
func (r *Registry) Register(name string, category Category, factory Factory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[name] = factory
	r.infos[name] = Info{Name: name, Category: category}
}

// NewConnector creates a new connector instance from the registry.
func (r *Registry) NewConnector(name string, config map[string]string) (IntegrationConnector, error) {
	r.mu.RLock()
	factory, ok := r.factories[name]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown connector: %s", name)
	}
	return factory(config)
}

// List returns information about all registered connectors.
func (r *Registry) List() []Info {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Info, 0, len(r.infos))
	for _, info := range r.infos {
		result = append(result, info)
	}
	return result
}

// Has returns true if a connector with the given name is registered.
func (r *Registry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.factories[name]
	return ok
}
