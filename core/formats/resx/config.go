package resx

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

// Config holds configuration for the .NET RESX / .resw format.
//
// RESX files have a fixed, well-defined XML schema (Microsoft ResX 2.0). The
// translatable surface is narrow — only string <data> entries — so the format
// exposes a small number of toggles; the bulk of its behaviour is dictated by
// the schema rather than by user configuration.
type Config struct {
	// ExtractComments controls whether a <data> entry's sibling <comment>
	// element is surfaced as a translator note on the emitted Block. Defaults
	// to true. When false, comments still round-trip verbatim in the document;
	// they are simply not exposed as Block annotations.
	ExtractComments bool

	// SkipNameDataReferences controls whether string <data> entries whose
	// @name begins with ">" (designer "name reference" entries such as
	// ">>control.Name", which carry the WinForms field name, not UI text) are
	// excluded from extraction. Defaults to true — these are never UI strings.
	SkipNameDataReferences bool
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "resx" }

// ConfigKind returns the Kind for the resx format config.
func (c *Config) ConfigKind() config.Kind { return config.FormatConfigKind("resx") }

// Reset restores default values.
func (c *Config) Reset() {
	*c = Config{
		ExtractComments:        true,
		SkipNameDataReferences: true,
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
		case "extractComments":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("extractComments: expected bool, got %T", val)
			}
			c.ExtractComments = b
		case "skipNameDataReferences":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("skipNameDataReferences: expected bool, got %T", val)
			}
			c.SkipNameDataReferences = b
		default:
			return fmt.Errorf("resx: unknown parameter: %s", key)
		}
	}
	return nil
}
