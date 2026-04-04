package phpcontent

import (
	"errors"
	"fmt"

	"github.com/neokapi/neokapi/core/format/schema"
	coreschema "github.com/neokapi/neokapi/core/schema"
)

// Config holds configuration for the PHP Content format.
type Config struct {
	// UseDirectives controls whether //okapi: skip/text directives are honored.
	UseDirectives bool

	// ExtractOutsideDirectives controls whether text outside the scope of
	// directives is extracted. Only relevant when UseDirectives is true.
	// Defaults to true.
	ExtractOutsideDirectives bool
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "phpcontent" }

// Reset restores default values.
func (c *Config) Reset() {
	c.UseDirectives = true
	c.ExtractOutsideDirectives = true
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
				return errors.New("phpcontent: useDirectives must be a boolean")
			}
			c.UseDirectives = b
		case "extractOutsideDirectives":
			b, ok := val.(bool)
			if !ok {
				return errors.New("phpcontent: extractOutsideDirectives must be a boolean")
			}
			c.ExtractOutsideDirectives = b
		default:
			return fmt.Errorf("phpcontent: unknown parameter: %s", key)
		}
	}
	return nil
}

// Schema returns the JSON Schema metadata for the PHP Content format.
func (c *Config) Schema() *schema.FormatSchema {
	return &schema.FormatSchema{
		Title:       "PHP Content Filter",
		Description: "Extracts translatable strings from PHP source files",
		Type:        "object",
		FormatMeta: schema.FormatMeta{
			ID:         "phpcontent",
			Extensions: []string{".php", ".phpcnt"},
			MimeTypes:  []string{"application/x-php"},
		},
		Groups: []schema.ParameterGroup{
			{
				ID:    "extraction",
				Label: "Extraction settings",
				Fields: []string{
					"useDirectives", "extractOutsideDirectives",
				},
			},
		},
		Properties: map[string]schema.PropertySchema{
			"useDirectives": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Use localization directives",
				Default:     true,
				Description: "Honor //okapi: skip/text directives in PHP source to control extraction scope.",
			}),
			"extractOutsideDirectives": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Extract outside the scope of the directives",
				Default:     true,
				Description: "Extract translatable strings found outside directive-controlled regions.",
				Visible:     &coreschema.ConditionExpr{Field: "useDirectives", Eq: true},
			}),
		},
	}
}
