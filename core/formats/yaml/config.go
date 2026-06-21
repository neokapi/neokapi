package yaml

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/neokapi/neokapi/core/format"
)

// Config holds configuration for the YAML format.
type Config struct {
	// ValidationConfigField carries the reader validation mode (RVM). Zero value
	// is ValidationOff, so YAML extraction stays byte-identical until the CLI
	// turns it on.
	format.ValidationConfigField

	// ExtractNonStrings controls whether non-string scalar values
	// (booleans, numbers, nulls) are extracted as translatable blocks.
	// Default: false (only string scalars are extracted).
	ExtractNonStrings bool

	// UseCodeFinder enables inline code detection within string values.
	// When enabled, patterns like \n in double-quoted strings are
	// recognized as inline codes.
	UseCodeFinder bool

	// CodeFinderRules defines inline code patterns.
	CodeFinderRules []string

	// KeyPathPatterns defines extraction rules based on key paths.
	// When non-empty, only keys matching one of these patterns are extracted.
	// Patterns support glob-style matching: * matches any single key,
	// ** matches any number of keys.
	// Example: ["en.**"] extracts only keys under "en".
	KeyPathPatterns []string

	// Subfilter specifies a sub-filter to apply to scalar values.
	// Currently supported: "html" (process HTML within YAML values).
	Subfilter string

	// compiled regex caches
	compiledCodeFinder []*regexp.Regexp
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "yaml" }

// Reset restores default values.
func (c *Config) Reset() {
	c.ExtractNonStrings = false
	c.UseCodeFinder = false
	c.CodeFinderRules = nil
	c.KeyPathPatterns = nil
	c.Subfilter = ""
	c.compiledCodeFinder = nil
}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		case "extractNonStrings":
			b, ok := val.(bool)
			if !ok {
				return errors.New("yaml: extractNonStrings must be bool")
			}
			c.ExtractNonStrings = b
		case "useCodeFinder":
			b, ok := val.(bool)
			if !ok {
				return errors.New("yaml: useCodeFinder must be bool")
			}
			c.UseCodeFinder = b
		case "keyPathPatterns":
			switch v := val.(type) {
			case []any:
				patterns := make([]string, len(v))
				for i, p := range v {
					s, ok := p.(string)
					if !ok {
						return errors.New("yaml: keyPathPatterns items must be strings")
					}
					patterns[i] = s
				}
				c.KeyPathPatterns = patterns
			case []string:
				c.KeyPathPatterns = v
			default:
				return errors.New("yaml: keyPathPatterns must be a string array")
			}
		case "codeFinderRules":
			rules, err := parseCodeFinderRules(val)
			if err != nil {
				return fmt.Errorf("codeFinderRules: %w", err)
			}
			c.CodeFinderRules = rules
			c.compiledCodeFinder = nil
		case "subfilter":
			s, ok := val.(string)
			if !ok {
				return errors.New("yaml: subfilter must be string")
			}
			c.Subfilter = s
		default:
			return fmt.Errorf("yaml: unknown parameter: %s", key)
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
	// Handle direct string slice
	if rules, ok := val.([]string); ok {
		return rules, nil
	}
	// Handle []any of strings
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
	// Handle bridge-style map with count + rule0, rule1, etc.
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
