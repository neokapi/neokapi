package wiki

import (
	"fmt"

	"github.com/neokapi/neokapi/core/format"
)

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
	Variant Variant `json:"variant"`

	// PreserveWhitespace preserves original whitespace in wiki markup
	// instead of normalizing it during extraction.
	PreserveWhitespace bool `json:"preserveWhitespace"`
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "wiki" }

// Reset restores default values.
//
// The default Variant is DokuWiki because the bridge filter id `okf_wiki`
// targets DokuWiki-only — per the upstream WikiFilter docs ("Currently
// the only supported markup style is Dokuwiki"). MediaWiki support
// remains available by setting Variant: VariantMediaWiki explicitly.
// See issue #496.
func (c *Config) Reset() {
	c.Variant = VariantDokuWiki
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
	return format.ApplyMapViaJSON(c, values)
}
