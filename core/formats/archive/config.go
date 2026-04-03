package archive

import (
	"fmt"

	"github.com/neokapi/neokapi/core/format"
)

// Config holds configuration for the archive format.
type Config struct {
	// FilePatterns specifies glob patterns for files to extract text from.
	// If empty, defaults to common text file patterns.
	FilePatterns []string

	// SubfilterMappings maps file extension glob patterns to format names.
	// When a SubfilterResolver is available, matching files are routed through
	// the corresponding sub-format reader/writer instead of line-by-line extraction.
	// Example: {"*.html": "html", "*.json": "json", "*.xml": "xml"}
	SubfilterMappings []format.SubfilterMapping
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "archive" }

// Reset restores default values.
func (c *Config) Reset() {
	c.FilePatterns = nil
}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		case "filePatterns":
			switch v := val.(type) {
			case []string:
				c.FilePatterns = v
			case []any:
				patterns := make([]string, 0, len(v))
				for _, item := range v {
					s, ok := item.(string)
					if !ok {
						return fmt.Errorf("archive: filePatterns items must be strings")
					}
					patterns = append(patterns, s)
				}
				c.FilePatterns = patterns
			default:
				return fmt.Errorf("archive: filePatterns must be a string array")
			}
		case "subfilterMappings":
			sfs, err := parseSubfilterMappings(val)
			if err != nil {
				return fmt.Errorf("archive: subfilterMappings: %w", err)
			}
			c.SubfilterMappings = sfs
		default:
			return fmt.Errorf("archive: unknown parameter: %s", key)
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
			return nil, fmt.Errorf("expected map, got %T", item)
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

// DefaultSubfilterMappings returns the default extension-to-format mappings
// used when a SubfilterResolver is available but no explicit mappings are configured.
func DefaultSubfilterMappings() []format.SubfilterMapping {
	return []format.SubfilterMapping{
		{Pattern: "*.html", Format: "html"},
		{Pattern: "*.htm", Format: "html"},
		{Pattern: "*.xhtml", Format: "html"},
		{Pattern: "*.xml", Format: "xml"},
		{Pattern: "*.json", Format: "json"},
		{Pattern: "*.yaml", Format: "yaml"},
		{Pattern: "*.yml", Format: "yaml"},
		{Pattern: "*.properties", Format: "javaproperties"},
		{Pattern: "*.md", Format: "markdown"},
	}
}

// defaultTextPatterns returns the default glob patterns for text files.
func defaultTextPatterns() []string {
	return []string{
		"*.txt", "*.xml", "*.html", "*.htm", "*.xhtml",
		"*.json", "*.yaml", "*.yml", "*.csv", "*.tsv",
		"*.properties", "*.strings", "*.md", "*.rst",
		"*.po", "*.pot", "*.xlf", "*.xliff", "*.tmx",
		"*.srt", "*.vtt",
	}
}
