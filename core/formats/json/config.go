package json

import (
	"fmt"

	"github.com/gokapi/gokapi/core/format"
)

// Config holds configuration for the JSON format.
type Config struct {
	// ExtractArrayStrings controls whether string values inside arrays
	// are extracted as translatable Blocks. Defaults to true.
	ExtractArrayStrings bool

	// Subfilters maps JSON key path patterns to format names for embedded
	// content. When a string value's key path matches a pattern, it is
	// parsed by the named format reader instead of being emitted as a
	// plain text block.
	//
	// Example: [{Pattern: "*.body", Format: "html"}] processes all
	// "body" keys at any depth through the HTML reader.
	Subfilters []format.SubfilterMapping
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "json" }

// Reset restores default values.
func (c *Config) Reset() {
	c.ExtractArrayStrings = true
	c.Subfilters = nil
}

// Validate checks configuration validity.
func (c *Config) Validate() error {
	for _, sf := range c.Subfilters {
		if sf.Pattern == "" {
			return fmt.Errorf("json: subfilter mapping has empty pattern")
		}
		if sf.Format == "" {
			return fmt.Errorf("json: subfilter mapping for %q has empty format", sf.Pattern)
		}
	}
	return nil
}

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		case "extractArrayStrings":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("extractArrayStrings: expected bool, got %T", val)
			}
			c.ExtractArrayStrings = b
		case "subfilters":
			sfs, err := parseSubfilterMappings(val)
			if err != nil {
				return fmt.Errorf("json: subfilters: %w", err)
			}
			c.Subfilters = sfs
		default:
			return fmt.Errorf("json: unknown parameter: %s", key)
		}
	}
	return nil
}

// parseSubfilterMappings parses subfilter config from a generic map value.
// Accepts []any where each element is map[string]any with "pattern" and "format" keys.
func parseSubfilterMappings(val any) ([]format.SubfilterMapping, error) {
	arr, ok := val.([]any)
	if !ok {
		return nil, fmt.Errorf("expected array, got %T", val)
	}
	var result []format.SubfilterMapping
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("expected object, got %T", item)
		}
		pattern, _ := m["pattern"].(string)
		formatName, _ := m["format"].(string)
		if pattern == "" || formatName == "" {
			return nil, fmt.Errorf("subfilter mapping requires 'pattern' and 'format'")
		}
		result = append(result, format.SubfilterMapping{Pattern: pattern, Format: formatName})
	}
	return result, nil
}
