package yaml

import "fmt"

// Config holds configuration for the YAML format.
type Config struct {
	// ExtractNonStrings controls whether non-string scalar values
	// (booleans, numbers, nulls) are extracted as translatable blocks.
	// Default: false (only string scalars are extracted).
	ExtractNonStrings bool

	// UseCodeFinder enables inline code detection within string values.
	// When enabled, patterns like \n in double-quoted strings are
	// recognized as inline codes.
	UseCodeFinder bool

	// KeyPathPatterns defines extraction rules based on key paths.
	// When non-empty, only keys matching one of these patterns are extracted.
	// Patterns support glob-style matching: * matches any single key,
	// ** matches any number of keys.
	// Example: ["en.**"] extracts only keys under "en".
	KeyPathPatterns []string

	// Subfilter specifies a sub-filter to apply to scalar values.
	// Currently supported: "html" (process HTML within YAML values).
	Subfilter string
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "yaml" }

// Reset restores default values.
func (c *Config) Reset() {
	c.ExtractNonStrings = false
	c.UseCodeFinder = false
	c.KeyPathPatterns = nil
	c.Subfilter = ""
}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		case "extractNonStrings":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("yaml: extractNonStrings must be bool")
			}
			c.ExtractNonStrings = b
		case "useCodeFinder":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("yaml: useCodeFinder must be bool")
			}
			c.UseCodeFinder = b
		case "keyPathPatterns":
			switch v := val.(type) {
			case []any:
				patterns := make([]string, len(v))
				for i, p := range v {
					s, ok := p.(string)
					if !ok {
						return fmt.Errorf("yaml: keyPathPatterns items must be strings")
					}
					patterns[i] = s
				}
				c.KeyPathPatterns = patterns
			case []string:
				c.KeyPathPatterns = v
			default:
				return fmt.Errorf("yaml: keyPathPatterns must be a string array")
			}
		case "subfilter":
			s, ok := val.(string)
			if !ok {
				return fmt.Errorf("yaml: subfilter must be string")
			}
			c.Subfilter = s
		default:
			return fmt.Errorf("yaml: unknown parameter: %s", key)
		}
	}
	return nil
}
