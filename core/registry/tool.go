package registry

import (
	"fmt"
	"sync"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
)

// ToolFactory creates a new Tool instance with default configuration.
type ToolFactory func() tool.Tool

// ToolConfigFactory creates a Tool from a config map and target language.
// Used for project flows where step config overrides tool defaults.
type ToolConfigFactory func(config map[string]any, targetLang string) (tool.Tool, error)

// ConfigPreprocessor transforms a tool's config map before it is passed to
// the tool's ConfigFactory. Used by CLI/desktop to inject credentials or
// resolve references before tool creation. The toolName identifies the tool
// being created; requires lists its runtime requirements (e.g. "credentials").
type ConfigPreprocessor func(toolName string, requires []string, config map[string]any) (map[string]any, error)

// ToolInfo holds metadata about a registered tool.
type ToolInfo struct {
	Name        ToolID   `json:"name"`
	DisplayName string   `json:"display_name,omitempty"`
	Description string   `json:"description,omitempty"`
	Category    string   `json:"category,omitempty"`
	Source      string   `json:"source,omitempty"` // "built-in", plugin name
	HasSchema   bool     `json:"hasSchema"`
	Tags        []string `json:"tags,omitempty"`     // freeform labels: "ai-powered","regex","batch"
	Requires    []string `json:"requires,omitempty"` // runtime requirements: "target-language","credentials","tm"

	// IO contract fields (Framework AD-006): port Consumes/Produces (IOPort).
	Cardinality   schema.LocaleCardinality `json:"cardinality,omitempty"`
	DefaultLocale model.LocaleID           `json:"default_locale,omitempty"`
	Consumes      []schema.IOPort          `json:"consumes,omitempty"`
	Produces      []schema.IOPort          `json:"produces,omitempty"`
	SideEffects   []schema.SideEffect      `json:"side_effects,omitempty"`

	// IsSourceTransform reports whether the tool can rewrite source
	// (tool.CapTransform) — a transformer in AD-006 terms; the placement pass
	// validates where such a tool sits among the ordered steps. Derived from
	// the tool's handler at registration.
	IsSourceTransform bool `json:"is_source_transform,omitempty"`

	// Recoverable marks a transformer that vaults originals for later restore
	// (redaction); the placement pass holds it to the remote-egress rule.
	Recoverable bool `json:"recoverable,omitempty"`

	// CLI metadata
	WritesOutput          bool     `json:"writes_output,omitempty"`
	DefaultParallelBlocks int      `json:"default_parallel_blocks,omitempty"`
	Aliases               []string `json:"aliases,omitempty"`

	// Bridge step metadata (only for Okapi bridge step tools).
	StepMeta *schema.StepMeta `json:"step_meta,omitempty"`

	// Group is set when this tool is a ToolGroup — a tool whose behaviour is
	// provided by one of several self-describing members selected by a
	// discriminator field. Flat consumers (CLI flags, docs, MCP) read the
	// composed Schema as before; group-aware consumers (the config UI) use this
	// to render master-detail (pick a member, configure only that member).
	Group *ToolGroupInfo `json:"group,omitempty"`
}

// ToolGroupInfo describes a tool group's members for group-aware consumers.
type ToolGroupInfo struct {
	// Discriminator is the config key that selects the active member.
	Discriminator string `json:"discriminator"`
	// Default is the member selected when the discriminator is unset.
	Default string `json:"default,omitempty"`
	// Members are the selectable backends, in selector order.
	Members []ToolGroupMemberInfo `json:"members"`
}

// ToolGroupMemberInfo is one member in a ToolGroupInfo.
type ToolGroupMemberInfo struct {
	Name        string `json:"name"`
	Label       string `json:"label,omitempty"`
	Description string `json:"description,omitempty"`
	HasSchema   bool   `json:"hasSchema"`
}

// probeSourceTransform reports whether a default-constructed tool from factory
// is source-transform-capable (tool.CapTransform). It is the DRY source of
// truth — capability comes from the tool's handler, not a hand-maintained flag.
// Guarded: a factory that panics on default construction yields false.
func probeSourceTransform(factory ToolFactory) (result bool) {
	if factory == nil {
		return false
	}
	defer func() { _ = recover() }()
	if c, ok := factory().(tool.Capable); ok {
		return c.Capability() == tool.CapTransform
	}
	return false
}

// copyToolMeta copies all ToolMeta fields into a ToolInfo.
func copyToolMeta(info *ToolInfo, m *schema.ToolMeta) {
	info.Category = m.Category
	info.Tags = m.Tags
	info.Requires = m.Requires
	info.Cardinality = m.Cardinality
	info.DefaultLocale = m.DefaultLocale
	info.Consumes = m.Consumes
	info.Produces = m.Produces
	info.SideEffects = m.SideEffects
	info.Recoverable = m.Recoverable
	info.WritesOutput = m.WritesOutput
	info.DefaultParallelBlocks = m.DefaultParallelBlocks
	info.Aliases = m.Aliases
}

// ToolRegistration bundles a factory with optional schema and metadata.
type ToolRegistration struct {
	Factory       ToolFactory
	ConfigFactory ToolConfigFactory // optional: creates tool from step config
	Schema        *schema.ComponentSchema
	Info          ToolInfo
	// ContractResolver, when set, refines the static IO contract (Consumes/
	// Produces/Requires) for a specific node config — e.g. a port that is
	// required only when an option is enabled (redact's entity overlay when
	// entity detection is on). It must not mutate base in place; return a copy
	// with the adjusted fields. nil means the static contract always applies.
	ContractResolver func(config map[string]any, base ToolInfo) ToolInfo

	// MemberSchemas holds each group member's own parameter schema, keyed by
	// member name, for tools registered via RegisterGroup. Empty for ordinary
	// tools. The composed (flat) schema lives in Schema; these are the per-member
	// schemas the UI renders master-detail.
	MemberSchemas map[string]*schema.ComponentSchema
}

// ToolRegistry manages available Tools.
type ToolRegistry struct {
	mu           sync.RWMutex
	tools        map[ToolID]*ToolRegistration
	preprocessor ConfigPreprocessor // optional: runs before ConfigFactory
}

// NewToolRegistry creates a new ToolRegistry.
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{tools: make(map[ToolID]*ToolRegistration)}
}

// Register registers a Tool factory (backward compatible).
func (r *ToolRegistry) Register(name ToolID, factory ToolFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[name] = &ToolRegistration{
		Factory: factory,
		Info:    ToolInfo{Name: name, Source: SourceBuiltIn, IsSourceTransform: probeSourceTransform(factory)},
	}
}

// RegisterWithSchema registers a Tool factory with a parameter schema.
func (r *ToolRegistry) RegisterWithSchema(name ToolID, factory ToolFactory, s *schema.ComponentSchema) {
	r.mu.Lock()
	defer r.mu.Unlock()
	info := ToolInfo{
		Name:              name,
		Source:            SourceBuiltIn,
		HasSchema:         s != nil,
		IsSourceTransform: probeSourceTransform(factory),
	}
	if s != nil {
		info.DisplayName = s.Title
		info.Description = s.Description
		if s.ToolMeta != nil {
			copyToolMeta(&info, s.ToolMeta)
		}
		info.StepMeta = s.StepMeta
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
func (r *ToolRegistry) RegisterMetadata(name ToolID, s *schema.ComponentSchema, source string) {
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
			copyToolMeta(&info, s.ToolMeta)
		}
		info.StepMeta = s.StepMeta
	}
	r.tools[name] = &ToolRegistration{
		Schema: s,
		Info:   info,
	}
}

// SetConfigFactory registers a config-based factory for an already-registered tool.
// This is used by CLI/desktop to attach NewToolFromConfig functions to tools
// that were registered via RegisterAll with zero-arg factories.
func (r *ToolRegistry) SetConfigFactory(name ToolID, factory ToolConfigFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if reg, ok := r.tools[name]; ok {
		reg.ConfigFactory = factory
	}
}

// SetContractResolver registers a contract resolver for an already-registered
// tool (see ToolRegistration.ContractResolver). The resolver refines the tool's
// IO contract from a node's config — used by data-flow validation so a port that
// is required only under certain options (e.g. redact's entity overlay when
// entity detection is enabled) is enforced exactly when it applies.
func (r *ToolRegistry) SetContractResolver(name ToolID, fn func(config map[string]any, base ToolInfo) ToolInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if reg, ok := r.tools[name]; ok {
		reg.ContractResolver = fn
	}
}

// ResolveToolInfo returns the tool's metadata with its IO contract refined for
// the given node config via the registered ContractResolver. Without a resolver
// it equals ToolInfo. Returns nil if the tool is unknown.
func (r *ToolRegistry) ResolveToolInfo(name ToolID, config map[string]any) *ToolInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	reg, ok := r.tools[name]
	if !ok {
		return nil
	}
	info := reg.Info
	if reg.ContractResolver != nil {
		info = reg.ContractResolver(config, info)
	}
	return &info
}

// SetConfigPreprocessor registers a function that transforms tool config maps
// before they are passed to the tool's ConfigFactory. This enables credential
// resolution, environment variable expansion, and similar config enrichment
// without tools needing to know about these concerns.
func (r *ToolRegistry) SetConfigPreprocessor(fn ConfigPreprocessor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.preprocessor = fn
}

// NewTool creates a new Tool instance for the given name with default config.
func (r *ToolRegistry) NewTool(name ToolID) (tool.Tool, error) {
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

// ToolInfo returns the metadata for a named tool, or nil if not found.
func (r *ToolRegistry) ToolInfo(name ToolID) *ToolInfo {
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
// If a ConfigPreprocessor is set, it runs first to enrich the config (e.g.
// credential resolution). Falls back to the zero-arg Factory if no
// ConfigFactory is registered.
func (r *ToolRegistry) NewToolWithConfig(name ToolID, config map[string]any, targetLang string) (tool.Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	reg, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("unknown tool: %s", name)
	}

	// Run preprocessor if set (e.g. credential resolution). The requirements
	// are config-resolved: a tool whose contract drops a requirement under a
	// given config (ai-entity-extract with engine "ner" needs no credentials)
	// must not have the preprocessor demand it anyway.
	if r.preprocessor != nil && reg.ConfigFactory != nil {
		requires := reg.Info.Requires
		if reg.ContractResolver != nil {
			requires = reg.ContractResolver(config, reg.Info).Requires
		}
		var err error
		config, err = r.preprocessor(string(name), requires, config)
		if err != nil {
			return nil, fmt.Errorf("tool %s config: %w", name, err)
		}
	}

	if reg.ConfigFactory != nil {
		return reg.ConfigFactory(config, targetLang)
	}
	if reg.Factory != nil {
		return reg.Factory(), nil
	}
	return nil, fmt.Errorf("tool %s has no factory", name)
}

// Schema returns the schema for a tool, or nil if none is registered.
func (r *ToolRegistry) Schema(name ToolID) *schema.ComponentSchema {
	r.mu.RLock()
	defer r.mu.RUnlock()
	reg, ok := r.tools[name]
	if !ok {
		return nil
	}
	return reg.Schema
}

// Names returns the names of all registered tools.
func (r *ToolRegistry) Names() []ToolID {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]ToolID, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}

// Has returns true if a tool is registered for the given name.
func (r *ToolRegistry) Has(name ToolID) bool {
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

// CLIToolEntry holds the information needed to generate a CLI command for a tool.
type CLIToolEntry struct {
	Info   ToolInfo
	Schema *schema.ComponentSchema
}

// CLITools returns tools that should be exposed as CLI commands.
// A tool is CLI-visible if it has a schema and a ConfigFactory (built-in tools
// with NewToolFromConfig) or is a plugin tool with a Factory and schema.
// Internal pipeline tools that lack a ConfigFactory are excluded.
func (r *ToolRegistry) CLITools() []CLIToolEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entries := make([]CLIToolEntry, 0, len(r.tools))
	for _, reg := range r.tools {
		if reg.Schema == nil {
			continue
		}
		// Built-in tools need ConfigFactory to be CLI-visible.
		// Plugin tools (bridge step tools) have Factory from RegisterWithSchema.
		if reg.ConfigFactory == nil && reg.Info.Source == SourceBuiltIn {
			continue
		}
		if reg.ConfigFactory == nil && reg.Factory == nil {
			continue
		}
		entries = append(entries, CLIToolEntry{
			Info:   reg.Info,
			Schema: reg.Schema,
		})
	}
	return entries
}
