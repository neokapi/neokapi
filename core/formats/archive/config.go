package archive

import "fmt"

// Config holds configuration for the archive format.
type Config struct {
	// FilePatterns specifies glob patterns for files to extract text from.
	// If empty, defaults to common text file patterns.
	FilePatterns []string
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
		default:
			return fmt.Errorf("archive: unknown parameter: %s", key)
		}
	}
	return nil
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
