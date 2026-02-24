package xml

import "fmt"

// Config holds configuration for the XML format.
type Config struct {
	// TranslatableElements lists element names whose text content is translatable.
	// If empty, all text content is considered translatable.
	TranslatableElements []string

	// TranslatableAttributes lists attribute names that are translatable.
	TranslatableAttributes []string
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "xml" }

// Reset restores default values.
func (c *Config) Reset() {
	c.TranslatableElements = nil
	c.TranslatableAttributes = nil
}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		case "translatableElements":
			arr, ok := val.([]any)
			if !ok {
				return fmt.Errorf("translatableElements: expected []any, got %T", val)
			}
			strs := make([]string, 0, len(arr))
			for i, elem := range arr {
				s, ok := elem.(string)
				if !ok {
					return fmt.Errorf("translatableElements[%d]: expected string, got %T", i, elem)
				}
				strs = append(strs, s)
			}
			c.TranslatableElements = strs
		case "translatableAttributes":
			arr, ok := val.([]any)
			if !ok {
				return fmt.Errorf("translatableAttributes: expected []any, got %T", val)
			}
			strs := make([]string, 0, len(arr))
			for i, elem := range arr {
				s, ok := elem.(string)
				if !ok {
					return fmt.Errorf("translatableAttributes[%d]: expected string, got %T", i, elem)
				}
				strs = append(strs, s)
			}
			c.TranslatableAttributes = strs
		default:
			return fmt.Errorf("xml: unknown parameter: %s", key)
		}
	}
	return nil
}
