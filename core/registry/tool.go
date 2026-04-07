package registry

import (
	"fmt"
	"sync"

	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
)

// ToolFactory creates a new Tool instance with default configuration.
type ToolFactory func() tool.Tool

// ToolConfigFactory creates a Tool from a config map and target language.
// Used for project flows where step config overrides tool defaults.
type ToolConfigFactory func(config map[string]any, targetLang string) (tool.Tool, error)

// ToolInfo holds metadata about a registered tool.
type ToolInfo struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name,omitempty"`
	Description string   `json:"description,omitempty"`
	Category    string   `json:"category,omitempty"`
	Source      string   `json:"source,omitempty"` // "built-in", plugin name
	HasSchema   bool     `json:"hasSchema"`
	Inputs      []string `json:"inputs,omitempty"`   // part types accepted: "block","data","media","layer","group"
	Outputs     []string `json:"outputs,omitempty"`  // part types produced/modified
	Tags        []string `json:"tags,omitempty"`     // freeform labels: "ai-powered","regex","batch"
	Requires    []string `json:"requires,omitempty"` // runtime requirements: "target-language","credentials","tm"

	// IO contract fields (AD-043)
	Cardinality   schema.LocaleCardinality `json:"cardinality,omitempty"`
	DefaultLocale string                   `json:"default_locale,omitempty"`
	Produces      []schema.AnnotationType  `json:"produces,omitempty"`
	SideEffects   []schema.SideEffect      `json:"side_effects,omitempty"`
}

// ToolRegistration bundles a factory with optional schema and metadata.
type ToolRegistration struct {
	Factory       ToolFactory
	ConfigFactory ToolConfigFactory // optional: creates tool from step config
	Schema        *schema.ComponentSchema
	Info          ToolInfo
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
		info.DisplayName = s.Title
		info.Description = s.Description
		if s.ToolMeta != nil {
			info.Category = s.ToolMeta.Category
			info.Inputs = s.ToolMeta.Inputs
			info.Outputs = s.ToolMeta.Outputs
			info.Tags = s.ToolMeta.Tags
			info.Requires = s.ToolMeta.Requires
			info.Cardinality = s.ToolMeta.Cardinality
			info.DefaultLocale = s.ToolMeta.DefaultLocale
			info.Produces = s.ToolMeta.Produces
			info.SideEffects = s.ToolMeta.SideEffects
		}
	}
	r.tools[name] = &ToolRegistration{
		Factory: factory,
		Schema:  s,
		Info:    info,
	}
}

// RegisterMetadata registers a tool's schema and metadata without a factory.
// Used for plugin tools that are executed remotely via a bridge — they appear
// in listings and have schemas for config UI, but cannot be instantiated locally.
func (r *ToolRegistry) RegisterMetadata(name string, s *schema.ComponentSchema, source string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	info := ToolInfo{
		Name:      name,
		Source:    source,
		HasSchema: s != nil,
	}
	if s != nil {
		info.DisplayName = s.Title
		info.Description = s.Description
		if s.ToolMeta != nil {
			info.Category = s.ToolMeta.Category
			info.Inputs = s.ToolMeta.Inputs
			info.Outputs = s.ToolMeta.Outputs
			info.Tags = s.ToolMeta.Tags
			info.Requires = s.ToolMeta.Requires
			info.Cardinality = s.ToolMeta.Cardinality
			info.DefaultLocale = s.ToolMeta.DefaultLocale
			info.Produces = s.ToolMeta.Produces
			info.SideEffects = s.ToolMeta.SideEffects
		}
	}
	r.tools[name] = &ToolRegistration{
		Schema: s,
		Info:   info,
	}
}

// SetConfigFactory registers a config-based factory for an already-registered tool.
// This is used by CLI/desktop to attach NewToolFromConfig functions to tools
// that were registered via RegisterAll with zero-arg factories.
func (r *ToolRegistry) SetConfigFactory(name string, factory ToolConfigFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if reg, ok := r.tools[name]; ok {
		reg.ConfigFactory = factory
	}
}

// NewTool creates a new Tool instance for the given name with default config.
func (r *ToolRegistry) NewTool(name string) (tool.Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	reg, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
	if reg.Factory == nil {
		return nil, fmt.Errorf("tool %s is a plugin tool and cannot be instantiated locally", name)
	}
	return reg.Factory(), nil
}

// GetToolInfo returns the metadata for a named tool, or nil if not found.
func (r *ToolRegistry) GetToolInfo(name string) *ToolInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	reg, ok := r.tools[name]
	if !ok {
		return nil
	}
	info := reg.Info
	return &info
}

// NewToolWithConfig creates a Tool from a step config map and target language.
// Falls back to the zero-arg Factory if no ConfigFactory is registered.
func (r *ToolRegistry) NewToolWithConfig(name string, config map[string]any, targetLang string) (tool.Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	reg, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
	if reg.ConfigFactory != nil {
		return reg.ConfigFactory(config, targetLang)
	}
	if reg.Factory != nil {
		return reg.Factory(), nil
	}
	return nil, fmt.Errorf("tool %s has no factory", name)
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
