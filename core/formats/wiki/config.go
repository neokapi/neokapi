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

	// disableNonTranslatableContent, when set, keeps non-translatable
	// contextual content (DokuWiki `<code>`/`<file>`/`<html>`/`<php>` block
	// bodies and indented code blocks) in opaque skeleton/Data instead of
	// surfacing it as RoleCode content blocks (visible to ingestion/LLM
	// consumers, skipped by machine translation). Zero value = surfacing ON
	// (the opt-out default). It is unexported and carries no JSON tag so it
	// is never round-tripped through ApplyMapViaJSON; ApplyMap handles the
	// public `extractNonTranslatableContent` key explicitly.
	disableNonTranslatableContent bool
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
	c.disableNonTranslatableContent = false
}

// ExtractNonTranslatableContent reports whether non-translatable contextual
// content (DokuWiki tagged code blocks and indented code blocks) is surfaced
// as RoleCode content blocks. Default true.
func (c *Config) ExtractNonTranslatableContent() bool {
	return !c.disableNonTranslatableContent
}

// SetExtractNonTranslatableContent toggles surfacing of non-translatable
// contextual content as content blocks (used by the parity runner to match the
// okf_wiki bridge, which keeps such content in skeleton).
func (c *Config) SetExtractNonTranslatableContent(v bool) {
	c.disableNonTranslatableContent = !v
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
//
// The `extractNonTranslatableContent` key drives the unexported
// disableNonTranslatableContent field via the inverted idiom (default ON), so
// it is handled explicitly and stripped before the remaining keys are applied
// via JSON so ApplyMapViaJSON's DisallowUnknownFields does not reject it.
func (c *Config) ApplyMap(values map[string]any) error {
	if v, ok := values["extractNonTranslatableContent"]; ok {
		b, ok := v.(bool)
		if !ok {
			return fmt.Errorf("extractNonTranslatableContent: expected bool, got %T", v)
		}
		c.disableNonTranslatableContent = !b
		rest := make(map[string]any, len(values))
		for k, val := range values {
			if k == "extractNonTranslatableContent" {
				continue
			}
			rest[k] = val
		}
		values = rest
	}
	return format.ApplyMapViaJSON(c, values)
}
