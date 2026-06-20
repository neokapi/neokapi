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

	// disableNonTranslatableContent, when set, keeps skipped PHP string
	// literals (the _skip_/_bskip_ targets, or strings outside a _btext_
	// region when ExtractOutsideDirectives is false) in opaque skeleton +
	// Data instead of surfacing them as RoleCode content blocks (visible to
	// ingestion/LLM consumers, skipped by MT). Zero value = surfacing ON
	// (the opt-out default), so the flag defaults ON however the Config is
	// constructed. Orthogonal to the directive flags, which only decide
	// which strings are *translatable*.
	disableNonTranslatableContent bool
}

// ExtractNonTranslatableContent reports whether skipped PHP string literals
// are surfaced as non-translatable RoleCode content blocks. Default true.
func (c *Config) ExtractNonTranslatableContent() bool {
	return !c.disableNonTranslatableContent
}

// SetExtractNonTranslatableContent toggles surfacing of skipped string
// literals as content blocks (used by the parity runner to match the Okapi
// bridge, which keeps such content in skeleton).
func (c *Config) SetExtractNonTranslatableContent(v bool) {
	c.disableNonTranslatableContent = !v
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
		case "extractNonTranslatableContent":
			b, ok := val.(bool)
			if !ok {
				return errors.New("phpcontent: extractNonTranslatableContent must be a boolean")
			}
			c.disableNonTranslatableContent = !b
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
					"extractNonTranslatableContent",
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
			"extractNonTranslatableContent": schema.Prop(coreschema.PropertySchema{
				Type:        "boolean",
				Title:       "Extract non-translatable content",
				Default:     true,
				Description: "If true (default), skipped PHP string literals are surfaced as non-translatable content blocks (visible to ingestion/LLM consumers, skipped by machine translation) instead of being hidden in skeleton. Disable to keep them in skeleton.",
			}),
		},
	}
}
