package registry

import (
	"fmt"
	"sync"

	"github.com/gokapi/gokapi/core/tool"
)

// ToolFactory creates a new Tool instance.
type ToolFactory func() tool.Tool

// ToolRegistry manages available Tools.
type ToolRegistry struct {
	mu    sync.RWMutex
	tools map[string]ToolFactory
}

// NewToolRegistry creates a new ToolRegistry.
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{tools: make(map[string]ToolFactory)}
}

// Register registers a Tool factory.
func (r *ToolRegistry) Register(name string, factory ToolFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[name] = factory
}

// NewTool creates a new Tool instance for the given name.
func (r *ToolRegistry) NewTool(name string) (tool.Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	factory, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
	return factory(), nil
}

// Names returns the names of all registered tools.
func (r *ToolRegistry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}

// Has returns true if a tool is registered for the given name.
func (r *ToolRegistry) Has(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.tools[name]
	return ok
}
