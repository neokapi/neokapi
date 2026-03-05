package xml

import (
	"fmt"

	"github.com/gokapi/gokapi/core/format"
)

// Config holds configuration for the XML format.
type Config struct {
	// TranslatableElements lists element names whose text content is translatable.
	// If empty, all text content is considered translatable.
	TranslatableElements []string

	// TranslatableAttributes lists attribute names that are translatable.
	TranslatableAttributes []string

	// Subfilters maps XML element path patterns to format names for embedded
	// content. When an element's text content path matches a pattern, it is
	// parsed by the named format reader instead of being emitted as a plain
	// text block. Patterns use dot-separated element paths with glob support.
	Subfilters []format.SubfilterMapping
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "xml" }

// Reset restores default values.
func (c *Config) Reset() {
	c.TranslatableElements = nil
	c.TranslatableAttributes = nil
}

// Validate checks configuration validity.
func (c *Config) Validate() error {
	for _, sf := range c.Subfilters {
		if sf.Pattern == "" {
			return fmt.Errorf("xml: subfilter mapping has empty pattern")
		}
		if sf.Format == "" {
			return fmt.Errorf("xml: subfilter mapping for %q has empty format", sf.Pattern)
		}
	}
	return nil
}

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
		case "subfilters":
			sfs, err := parseSubfilterMappings(val)
			if err != nil {
				return fmt.Errorf("xml: subfilters: %w", err)
			}
			c.Subfilters = sfs
		default:
			return fmt.Errorf("xml: unknown parameter: %s", key)
		}
	}
	return nil
}

// parseSubfilterMappings parses subfilter config from a generic map value.
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
