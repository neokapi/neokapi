package doxygen

import (
	"fmt"

	"github.com/neokapi/neokapi/core/format/schema"
)

// Config holds configuration for the Doxygen comment format.
type Config struct{}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "doxygen" }

// Reset restores default values.
func (c *Config) Reset() {}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key := range values {
		return fmt.Errorf("doxygen: unknown parameter: %s", key)
	}
	return nil
}

// Schema returns the JSON Schema metadata for the Doxygen format.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "Doxygen Comments",
		Description: "Extracts translatable text from Doxygen/Javadoc comments in source code",
		Type:        "object",
		FormatMeta: schema.FormatSchemaMeta{
			ID:         "doxygen",
			Extensions: []string{".c", ".cpp", ".h", ".java", ".m", ".py"},
			MimeTypes:  []string{"text/x-doxygen-txt"},
		},
	}
}
