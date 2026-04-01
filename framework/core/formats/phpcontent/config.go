package phpcontent

import (
	"fmt"

	"github.com/neokapi/neokapi/core/format/schema"
)

// Config holds configuration for the PHP Content format.
type Config struct {
	// UseDirectives controls whether //okapi: skip/text directives are honored.
	UseDirectives bool
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "phpcontent" }

// Reset restores default values.
func (c *Config) Reset() {
	c.UseDirectives = true
}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		case "useDirectives":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("phpcontent: useDirectives must be a boolean")
			}
			c.UseDirectives = b
		default:
			return fmt.Errorf("phpcontent: unknown parameter: %s", key)
		}
	}
	return nil
}

// Schema returns the JSON Schema metadata for the PHP Content format.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "PHP Content",
		Description: "Extracts translatable strings from PHP source files",
		Type:        "object",
		FormatMeta: schema.FormatSchemaMeta{
			ID:         "phpcontent",
			Extensions: []string{".php", ".phpcnt"},
			MimeTypes:  []string{"application/x-php"},
		},
	}
}
