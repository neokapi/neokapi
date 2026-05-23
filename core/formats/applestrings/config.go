package applestrings

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

// Config holds configuration for the Apple Strings format (legacy
// .strings + .stringsdict localization files).
//
// Both file types have a fixed, well-defined structure — a key/value text
// table (.strings) and a plist-XML plural/format dictionary (.stringsdict).
// The translatable surface is narrow, so the format exposes only a couple of
// behavioural toggles; the bulk of its behaviour is dictated by the format
// itself rather than by user configuration.
type Config struct {
	// ExtractComments controls whether a /* ... */ or // ... comment that
	// immediately precedes a .strings entry is surfaced as a translator note
	// on the emitted Block. Defaults to true. When false, comments still
	// round-trip verbatim in the document; they are simply not exposed as
	// Block annotations.
	ExtractComments bool

	// ProtectPlaceholders controls whether printf-style format specifiers
	// (%@, %lld, %1$@, …) in .strings values and %#@var@ / printf tokens in
	// .stringsdict format strings are lifted into inline placeholder codes so
	// they survive pseudo-translation and AI/MT translation as opaque tokens.
	// Defaults to true.
	ProtectPlaceholders bool
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "applestrings" }

// ConfigKind returns the Kind for the applestrings format config.
func (c *Config) ConfigKind() config.Kind { return config.FormatConfigKind("applestrings") }

// Reset restores default values.
func (c *Config) Reset() {
	*c = Config{
		ExtractComments:     true,
		ProtectPlaceholders: true,
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
		case "protectPlaceholders":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("protectPlaceholders: expected bool, got %T", val)
			}
			c.ProtectPlaceholders = b
		default:
			return fmt.Errorf("applestrings: unknown parameter: %s", key)
		}
	}
	return nil
}
