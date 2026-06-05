package mosestext

import (
	"errors"
	"fmt"
	"regexp"
)

// Config holds configuration for the Moses Text format.
type Config struct {
	// UseCodeFinder enables inline-code detection within each line. When
	// enabled, each pattern in CodeFinderRules carves placeholder runs
	// out of the source text so XML-style markup like `<mrk mtype="seg">`
	// or entity references like `&lt;` survive translation as opaque
	// inline codes instead of being translated character-by-character.
	UseCodeFinder bool

	// CodeFinderRules is the list of regular expressions that identify
	// inline codes within a line. Patterns are tried in order and may
	// overlap; matches are sorted by start offset.
	CodeFinderRules []string

	compiledCodeFinder []*regexp.Regexp
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "mosestext" }

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
				return errors.New("mosestext: useCodeFinder must be bool")
			}
			c.UseCodeFinder = b
		case "codeFinderRules":
			rules, err := parseCodeFinderRules(val)
			if err != nil {
				return fmt.Errorf("mosestext: codeFinderRules: %w", err)
			}
			c.CodeFinderRules = rules
			c.compiledCodeFinder = nil
		default:
			return fmt.Errorf("mosestext: unknown parameter: %s", key)
		}
	}
	return nil
}

// CodeFinderPatterns returns compiled regex patterns for the code
// finder. Compilation is lazy and cached.
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

// parseCodeFinderRules accepts either a string slice, an []any of
// strings, or a bridge-style map (`count` + `rule0`, `rule1`, …) — the
// same shapes the yaml/csv configs accept.
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
	return nil, fmt.Errorf("expected []string, []any, or map[string]any, got %T", val)
}
