package rtf

import (
	"fmt"

	"github.com/neokapi/neokapi/core/config"
)

// Config holds configuration for the RTF format.
// The Okapi RTF filter uses DefaultParameters (no configurable options),
// but neokapi adds commonly-needed extraction controls for RTF content.
type Config struct {
	// ExtractHeadersFooters controls whether text in RTF headers and footers
	// is extracted as translatable content. When false, header/footer
	// destinations are skipped.
	// Defaults to false (matching Okapi behavior which skips these destinations).
	ExtractHeadersFooters bool

	// ExtractAnnotations controls whether text in RTF annotation (comment)
	// destinations is extracted as translatable content.
	// Defaults to false.
	ExtractAnnotations bool

	// ExtractBookmarks controls whether bookmark text is extracted.
	// Defaults to false.
	ExtractBookmarks bool

	// UseCodeFinder enables regex-based inline code detection in extracted text.
	// Defaults to false.
	UseCodeFinder bool

	// CodeFinderRules defines regex patterns for detecting inline codes.
	CodeFinderRules []string

	// disableNonTranslatableContent, when set, keeps non-translatable
	// contextual content (headers/footers, \info title/doccomm metadata, and
	// \xe/\tc index/TOC entries) in opaque skeleton instead of surfacing it as
	// role-tagged, non-translatable content Blocks (visible to ingestion,
	// skipped by MT). It also disables carrying \annotation review comments as
	// note metadata and the proper \* ignorable-destination handling. Zero
	// value = surfacing ON (the opt-out default). The parity runner type-asserts
	// SetExtractNonTranslatableContent and forces this OFF so the canonical part
	// stream stays byte-identical to the skeleton-only baseline.
	disableNonTranslatableContent bool
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "rtf" }

// ConfigKind returns the Kind for RTF format config.
func (c *Config) ConfigKind() config.Kind { return config.FormatConfigKind("rtf") }

// Reset restores default values.
func (c *Config) Reset() {
	*c = Config{
		ExtractHeadersFooters: false,
		ExtractAnnotations:    false,
		ExtractBookmarks:      false,
		UseCodeFinder:         false,
	}
}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ExtractNonTranslatableContent reports whether non-translatable contextual
// content (headers/footers, \info title/doccomm, \xe/\tc entries) is surfaced
// as role-tagged content Blocks. Default true.
func (c *Config) ExtractNonTranslatableContent() bool {
	return !c.disableNonTranslatableContent
}

// SetExtractNonTranslatableContent toggles surfacing of non-translatable
// contextual content as content blocks. The parity runner type-asserts this and
// turns it off to keep its canonical stream skeleton-only (byte-identical to the
// pre-feature baseline).
func (c *Config) SetExtractNonTranslatableContent(v bool) {
	c.disableNonTranslatableContent = !v
}

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		case "extractHeadersFooters":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("extractHeadersFooters: expected bool, got %T", val)
			}
			c.ExtractHeadersFooters = b
		case "extractAnnotations":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("extractAnnotations: expected bool, got %T", val)
			}
			c.ExtractAnnotations = b
		case "extractBookmarks":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("extractBookmarks: expected bool, got %T", val)
			}
			c.ExtractBookmarks = b
		case "extractNonTranslatableContent":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("extractNonTranslatableContent: expected bool, got %T", val)
			}
			c.disableNonTranslatableContent = !b
		case "useCodeFinder":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("useCodeFinder: expected bool, got %T", val)
			}
			c.UseCodeFinder = b
		case "codeFinderRules":
			rules, err := parseCodeFinderRules(val)
			if err != nil {
				return fmt.Errorf("codeFinderRules: %w", err)
			}
			c.CodeFinderRules = rules
		default:
			return fmt.Errorf("rtf: unknown parameter: %s", key)
		}
	}
	return nil
}

// parseCodeFinderRules parses code finder rules from a string slice or bridge-style map.
func parseCodeFinderRules(val any) ([]string, error) {
	if rules, ok := val.([]string); ok {
		return rules, nil
	}
	if arr, ok := val.([]any); ok {
		var rules []string
		for _, item := range arr {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("expected string in array, got %T", item)
			}
			rules = append(rules, s)
		}
		return rules, nil
	}
	if m, ok := val.(map[string]any); ok {
		count := 0
		if c, ok := m["count"]; ok {
			switch v := c.(type) {
			case int:
				count = v
			case float64:
				count = int(v)
			}
		}
		var rules []string
		for i := range count {
			key := fmt.Sprintf("rule%d", i)
			if rule, ok := m[key].(string); ok {
				rules = append(rules, rule)
			}
		}
		return rules, nil
	}
	return nil, fmt.Errorf("expected []string or map, got %T", val)
}
