package tmx

import (
	"fmt"

	"github.com/neokapi/neokapi/core/config"
)

// Config holds configuration for the TMX format.
type Config struct {
	// Extraction settings

	// ProcessAllTargets reads all target language TUVs from each TU.
	// When false, only the first target TUV is read. Defaults to true.
	ProcessAllTargets bool

	// ExitOnInvalid stops processing when encountering invalid TUs.
	// When false (default), invalid TUs are skipped silently.
	ExitOnInvalid bool

	// Output settings

	// EscapeGT escapes greater-than characters as &gt; in output.
	// When false (default), > is written literally. Defaults to false.
	EscapeGT bool

	// Inline codes settings

	// UseCodeFinder enables regex-based inline code detection. Defaults to false.
	UseCodeFinder bool

	// CodeFinderRules defines inline code patterns.
	CodeFinderRules []string
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "tmx" }

// ConfigKind returns the Kind for TMX format config.
func (c *Config) ConfigKind() config.Kind { return config.FormatConfigKind("tmx") }

// Reset restores default values.
func (c *Config) Reset() {
	*c = Config{
		ProcessAllTargets: true,
	}
}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		// Extraction
		case "processAllTargets":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("processAllTargets: expected bool, got %T", val)
			}
			c.ProcessAllTargets = b
		case "exitOnInvalid":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("exitOnInvalid: expected bool, got %T", val)
			}
			c.ExitOnInvalid = b

		// Output
		case "escapeGT":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("escapeGT: expected bool, got %T", val)
			}
			c.EscapeGT = b

		// Inline codes
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
			return fmt.Errorf("tmx: unknown parameter: %s", key)
		}
	}
	return nil
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
