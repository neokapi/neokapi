package wiki

import "fmt"

// Variant identifies the wiki markup dialect.
type Variant string

const (
	// VariantMediaWiki selects MediaWiki markup syntax.
	VariantMediaWiki Variant = "mediawiki"
	// VariantDokuWiki selects DokuWiki markup syntax.
	VariantDokuWiki Variant = "dokuwiki"
)

// Config holds configuration for the wiki format reader/writer.
type Config struct {
	// Variant selects the wiki markup dialect (mediawiki or dokuwiki).
	Variant Variant

	// PreserveWhitespace preserves original whitespace in wiki markup
	// instead of normalizing it during extraction.
	PreserveWhitespace bool
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "wiki" }

// Reset restores default values.
func (c *Config) Reset() {
	c.Variant = VariantMediaWiki
	c.PreserveWhitespace = false
}

// Validate checks configuration validity.
func (c *Config) Validate() error {
	switch c.Variant {
	case VariantMediaWiki, VariantDokuWiki:
		return nil
	default:
		return fmt.Errorf("wiki: unknown variant: %s", c.Variant)
	}
}

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		case "variant":
			s, ok := val.(string)
			if !ok {
				return fmt.Errorf("variant: expected string, got %T", val)
			}
			c.Variant = Variant(s)
		case "preserveWhitespace":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("preserveWhitespace: expected bool, got %T", val)
			}
			c.PreserveWhitespace = b
		default:
			return fmt.Errorf("wiki: unknown parameter: %s", key)
		}
	}
	return nil
}
