package arb

import (
	"fmt"

	"github.com/neokapi/neokapi/core/config"
	"github.com/neokapi/neokapi/core/format"
)

// Compile-time assertions that Config satisfies the framework config
// interfaces, including the optional schema and config-kind providers used by
// CLI introspection and config decoding.
var (
	_ format.DataFormatConfig   = (*Config)(nil)
	_ format.SchemaProvider     = (*Config)(nil)
	_ format.ConfigKindProvider = (*Config)(nil)
)

// Config holds configuration for the Flutter Application Resource Bundle (.arb)
// format.
//
// ARB has a fixed, well-defined JSON structure (a flat map of message keys to
// ICU MessageFormat strings, with sibling "@<id>" attribute objects and
// "@@<global>" metadata). The format therefore exposes only a small number of
// toggles; the bulk of its behaviour is dictated by the structure rather than
// by user configuration.
type Config struct {
	// DescriptionNotes controls whether the human-facing context in a resource's
	// sibling "@<id>" attributes object is surfaced as developer notes on the
	// emitted block: the resource-level "description" plus each placeholder's
	// "example"/"description" hint. Defaults to true. The attributes object is
	// always preserved byte-faithfully on round-trip regardless of this setting.
	DescriptionNotes bool
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "arb" }

// ConfigKind returns the Kind for the arb format config.
func (c *Config) ConfigKind() config.Kind { return config.FormatConfigKind("arb") }

// Reset restores default values.
func (c *Config) Reset() {
	*c = Config{
		DescriptionNotes: true,
	}
}

// Validate checks configuration validity.
func (c *Config) Validate() error {
	return nil
}

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		case "descriptionNotes":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("descriptionNotes: expected bool, got %T", val)
			}
			c.DescriptionNotes = b
		default:
			return fmt.Errorf("arb: unknown parameter: %s", key)
		}
	}
	return nil
}
