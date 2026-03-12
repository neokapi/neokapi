package markdown

import "fmt"

// Config holds configuration for the Markdown format.
type Config struct {
	// TranslateCodeBlocks controls whether fenced/indented code blocks are
	// translatable (emitted as Blocks). Default false = emitted as Data.
	TranslateCodeBlocks bool

	// TranslateFrontMatter controls whether YAML front matter values are
	// translatable. Default false = emitted as Data.
	TranslateFrontMatter bool

	// TranslateImageAlt controls whether image alt text is translatable
	// (included in the block's inline content). Default true (nonSkipImageAlt=false).
	nonSkipImageAlt bool

	// TranslateURLs controls whether link/image URLs are translatable.
	// Default false.
	TranslateURLs bool

	// TranslateBlockQuotes controls whether blockquote content is translatable.
	// Default true (nonTranslatableBlockQuotes=false).
	nonTranslatableBlockQuotes bool

	// TranslateHTMLBlocks controls whether HTML blocks are translatable.
	// Default false = emitted as Data.
	TranslateHTMLBlocks bool
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "markdown" }

// Reset restores default values.
func (c *Config) Reset() {
	*c = Config{}
}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// TranslateImageAlt returns true if image alt text should be translatable.
func (c *Config) TranslateImageAlt() bool {
	return !c.nonSkipImageAlt
}

// TranslateBlockQuotes returns true if blockquote content should be translatable.
func (c *Config) TranslateBlockQuotes() bool {
	return !c.nonTranslatableBlockQuotes
}

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		case "translateCodeBlocks":
			if v, ok := val.(bool); ok {
				c.TranslateCodeBlocks = v
			}
		case "translateFrontMatter":
			if v, ok := val.(bool); ok {
				c.TranslateFrontMatter = v
			}
		case "translateImageAlt":
			if v, ok := val.(bool); ok {
				c.nonSkipImageAlt = !v
			}
		case "translateURLs":
			if v, ok := val.(bool); ok {
				c.TranslateURLs = v
			}
		case "translateBlockQuotes":
			if v, ok := val.(bool); ok {
				c.nonTranslatableBlockQuotes = !v
			}
		case "translateHTMLBlocks":
			if v, ok := val.(bool); ok {
				c.TranslateHTMLBlocks = v
			}
		default:
			return fmt.Errorf("markdown: unknown parameter: %s", key)
		}
	}
	return nil
}
