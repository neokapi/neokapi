package registry

import (
	"fmt"
	"sync"

	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
)

// ToolFactory creates a new Tool instance.
type ToolFactory func() tool.Tool

// ToolInfo holds metadata about a registered tool.
type ToolInfo struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Category    string   `json:"category,omitempty"`
	Source      string   `json:"source,omitempty"` // "built-in", plugin name
	HasSchema   bool     `json:"hasSchema"`
	Inputs      []string `json:"inputs,omitempty"`   // part types accepted: "block","data","media","layer","group"
	Outputs     []string `json:"outputs,omitempty"`  // part types produced/modified
	Tags        []string `json:"tags,omitempty"`     // freeform labels: "ai-powered","regex","batch"
	Requires    []string `json:"requires,omitempty"` // runtime requirements: "target-language","credentials","tm"
}

// ToolRegistration bundles a factory with optional schema and metadata.
type ToolRegistration struct {
	Factory ToolFactory
	Schema  *schema.ComponentSchema
	Info    ToolInfo
}

// ToolRegistry manages available Tools.
type ToolRegistry struct {
	mu    sync.RWMutex
	tools map[string]*ToolRegistration
}

// NewToolRegistry creates a new ToolRegistry.
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{tools: make(map[string]*ToolRegistration)}
}

// Register registers a Tool factory (backward compatible).
func (r *ToolRegistry) Register(name string, factory ToolFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[name] = &ToolRegistration{
		Factory: factory,
		Info:    ToolInfo{Name: name, Source: "built-in"},
	}
}

// RegisterWithSchema registers a Tool factory with a parameter schema.
func (r *ToolRegistry) RegisterWithSchema(name string, factory ToolFactory, s *schema.ComponentSchema) {
	r.mu.Lock()
	defer r.mu.Unlock()
	info := ToolInfo{
		Name:      name,
		Source:    "built-in",
		HasSchema: s != nil,
	}
	if s != nil {
		info.Description = s.Description
		info.Category = s.Meta.Category
		info.Inputs = s.Meta.Inputs
		info.Outputs = s.Meta.Outputs
		info.Tags = s.Meta.Tags
		info.Requires = s.Meta.Requires
	}
	r.tools[name] = &ToolRegistration{
		Factory: factory,
		Schema:  s,
		Info:    info,
	}
}

// NewTool creates a new Tool instance for the given name.
func (r *ToolRegistry) NewTool(name string) (tool.Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	reg, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
	return reg.Factory(), nil
}

// GetSchema returns the schema for a tool, or nil if none is registered.
func (r *ToolRegistry) GetSchema(name string) *schema.ComponentSchema {
	r.mu.RLock()
	defer r.mu.RUnlock()
	reg, ok := r.tools[name]
	if !ok {
		return nil
	}
	return reg.Schema
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

// ListWithSchemas returns info about all registered tools, including schema status.
func (r *ToolRegistry) ListWithSchemas() []ToolInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	infos := make([]ToolInfo, 0, len(r.tools))
	for _, reg := range r.tools {
		infos = append(infos, reg.Info)
	}
	return infos
}
