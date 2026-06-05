package dtd

import (
	"fmt"
	"regexp"
)

// Config holds configuration for the DTD format.
type Config struct {
	// UseCodeFinder enables inline-code extraction in entity values
	// using CodeFinderRules. Mirrors okapi's DTDFilter useCodeFinder.
	UseCodeFinder bool

	// CodeFinderRules are regex patterns for inline-code extraction —
	// matched substrings become Ph runs (Spans) instead of plain text,
	// so they survive pseudo-translation as opaque codes.
	CodeFinderRules []string

	compiledCodeFinder []*regexp.Regexp
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "dtd" }

// Reset restores default values.
func (c *Config) Reset() {
	c.UseCodeFinder = false
	c.CodeFinderRules = nil
	c.compiledCodeFinder = nil
}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
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
			return fmt.Errorf("dtd: unknown parameter: %s", key)
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
	if arr, ok := val.([]any); ok {
		rules := make([]string, 0, len(arr))
		for _, item := range arr {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("expected string, got %T", item)
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
