package format

import (
	"github.com/neokapi/neokapi/core/config"
	"github.com/neokapi/neokapi/core/format/schema"
)

// DataFormatConfig holds configuration for a data format.
type DataFormatConfig interface {
	// FormatName returns the format this config applies to.
	FormatName() string

	// Reset restores default values.
	Reset()

	// Validate checks configuration validity.
	Validate() error

	// ApplyMap applies configuration values from a map.
	// Unknown keys or type mismatches return an error.
	ApplyMap(values map[string]any) error
}

// SchemaProvider is an optional interface that DataFormatConfig implementations
// can implement to provide JSON Schema metadata for their parameters.
// Formats that implement this interface enable CLI introspection (formats info,
// formats schema) and schema-based validation without requiring bridge plugins.
type SchemaProvider interface {
	Schema() *schema.FilterSchema
}

// ConfigKindProvider is an optional interface that DataFormatConfig
// implementations can implement to declare their config envelope kind.
// This enables the config loading system to detect the expected kind
// for a format and validate or transform incoming configs accordingly.
type ConfigKindProvider interface {
	// ConfigKind returns the Kind for this format's config envelope.
	// For example, config.FormatConfigKind("html") for native HTML.
	ConfigKind() config.Kind
}
