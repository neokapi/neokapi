package registry

// FormatID is a type-safe identifier for a registered data format.
// Values are lowercase, stable identifiers like "json", "xliff", "po".
// Versioned formats use the syntax "name@version" (e.g., "okapi-html@1.46.0").
type FormatID string

// String returns the string representation.
func (id FormatID) String() string { return string(id) }

// ToolID is a type-safe identifier for a registered tool.
// Values are kebab-case, stable identifiers like "word-count", "translate".
type ToolID string

// String returns the string representation.
func (id ToolID) String() string { return string(id) }

// FlowID is a type-safe identifier for a flow definition.
type FlowID string

// String returns the string representation.
func (id FlowID) String() string { return string(id) }

// PresetID is a type-safe identifier for a format preset.
type PresetID string

// String returns the string representation.
func (id PresetID) String() string { return string(id) }

// PluginID is a type-safe identifier for a plugin.
type PluginID string

// String returns the string representation.
func (id PluginID) String() string { return string(id) }

// SourceBuiltIn is the canonical source value for built-in formats, tools, and flows.
const SourceBuiltIn = "built-in"

// FormatAuto is the sentinel format ID for automatic format detection.
const FormatAuto FormatID = "auto"
