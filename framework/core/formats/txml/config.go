package txml

import (
	"fmt"

	"github.com/neokapi/neokapi/core/config"
)

// Config holds configuration for the Trados XML (TXML) format.
// Options mirror the Okapi Framework TXML filter parameters.
type Config struct {
	// AllowEmptyOutputTarget controls whether empty target segments are
	// permitted in the output. When true, segments with no translation
	// produce an empty <target/> element. When false, segments without
	// translation omit the <target> element entirely.
	// Defaults to true (matching Okapi behavior).
	AllowEmptyOutputTarget bool
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "txml" }

// ConfigKind returns the Kind for TXML format config.
func (c *Config) ConfigKind() config.Kind { return config.FormatConfigKind("txml") }

// Reset restores default values matching Okapi's TXML filter defaults.
func (c *Config) Reset() {
	*c = Config{
		AllowEmptyOutputTarget: true,
	}
}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		case "allowEmptyOutputTarget":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("allowEmptyOutputTarget: expected bool, got %T", val)
			}
			c.AllowEmptyOutputTarget = b
		default:
			return fmt.Errorf("txml: unknown parameter: %s", key)
		}
	}
	return nil
}
