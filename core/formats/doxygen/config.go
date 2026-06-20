package doxygen

import (
	"fmt"
	"regexp"

	"github.com/neokapi/neokapi/core/config"
)

// Config holds configuration for the Doxygen comment format.
type Config struct {
	// PreserveWhitespace preserves original whitespace in extracted text.
	// When false (default), whitespace is normalized.
	PreserveWhitespace bool

	// disableNonTranslatableContent, when set, keeps non-translatable
	// contextual content (\code…\endcode, \verbatim…\endverbatim, \dot,
	// \msc, \htmlonly, … region bodies) buried in opaque skeleton/Data
	// instead of surfacing it as RoleCode content blocks (visible to
	// ingestion/LLM consumers, skipped by MT). Zero value = surfacing ON
	// (the opt-out default).
	disableNonTranslatableContent bool

	// UseCodeFinder enables regex-based inline code detection. Defaults to false.
	UseCodeFinder bool

	// CodeFinderRules are regex patterns that match inline codes.
	CodeFinderRules []string

	// compiledCodeFinder caches compiled regex patterns.
	compiledCodeFinder []*regexp.Regexp
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "doxygen" }

// ConfigKind returns the Kind for Doxygen format config.
func (c *Config) ConfigKind() config.Kind { return config.FormatConfigKind("doxygen") }

// Reset restores default values.
func (c *Config) Reset() {
	*c = Config{}
}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ExtractNonTranslatableContent reports whether non-translatable contextual
// content (\code/\verbatim/\dot/\msc/\htmlonly/… region bodies) is surfaced as
// RoleCode content blocks. Default true.
func (c *Config) ExtractNonTranslatableContent() bool {
	return !c.disableNonTranslatableContent
}

// SetExtractNonTranslatableContent toggles surfacing of non-translatable
// contextual content as content blocks (used by the parity runner to match the
// Okapi bridge, which keeps such content in skeleton).
func (c *Config) SetExtractNonTranslatableContent(v bool) {
	c.disableNonTranslatableContent = !v
}

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		case "preserveWhitespace":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("preserveWhitespace: expected bool, got %T", val)
			}
			c.PreserveWhitespace = b
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
			c.compiledCodeFinder = nil
		case "codeFinderRules":
			rules, err := parseCodeFinderRules(val)
			if err != nil {
				return fmt.Errorf("codeFinderRules: %w", err)
			}
			c.CodeFinderRules = rules
			c.compiledCodeFinder = nil
		default:
			return fmt.Errorf("doxygen: unknown parameter: %s", key)
		}
	}
	return nil
}

// CodeFinderPatterns returns compiled regex patterns for code finder.
func (c *Config) CodeFinderPatterns() []*regexp.Regexp {
	if c.compiledCodeFinder != nil {
		return c.compiledCodeFinder
	}
	if !c.UseCodeFinder || len(c.CodeFinderRules) == 0 {
		return nil
	}
	for _, pattern := range c.CodeFinderRules {
		re, err := regexp.Compile(pattern)
		if err == nil {
			c.compiledCodeFinder = append(c.compiledCodeFinder, re)
		}
	}
	return c.compiledCodeFinder
}

// parseCodeFinderRules parses code finder rules from bridge-style map or string slice.
func parseCodeFinderRules(val any) ([]string, error) {
	if rules, ok := val.([]string); ok {
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
