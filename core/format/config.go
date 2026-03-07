package format

import "github.com/gokapi/gokapi/core/format/schema"

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

// ConfigVersionProvider is an optional interface that DataFormatConfig
// implementations can implement to declare their config envelope apiVersion.
// This enables the config loading system to detect the expected apiVersion
// for a format and validate or transform incoming configs accordingly.
type ConfigVersionProvider interface {
	// ConfigAPIVersion returns the native apiVersion for this format's config.
	// For example, "gokapi/html-v1" or "gokapi/json-v1".
	ConfigAPIVersion() string
}
