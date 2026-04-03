package xliff2

import (
	"fmt"

	"github.com/neokapi/neokapi/core/config"
)

// Config holds configuration for the XLIFF 2.0 format.
type Config struct {
	// Extraction settings

	// ForceUniqueIds ensures inline tag IDs are unique within units.
	// Defaults to false.
	ForceUniqueIds bool

	// IgnoreTagTypeMatch ignores tag type mismatches between source and target.
	// Defaults to false.
	IgnoreTagTypeMatch bool

	// States settings

	// DiscardInvalidTargets discards targets that fail validation rather than
	// rejecting the entire file. Defaults to false.
	DiscardInvalidTargets bool

	// Output settings

	// WriteOriginalData includes original data in output when available.
	// Defaults to true.
	WriteOriginalData bool

	// Inline codes settings

	// UseCodeFinder enables regex-based inline code detection. Defaults to false.
	UseCodeFinder bool

	// CodeFinderRules defines inline code patterns.
	CodeFinderRules []string
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "xliff2" }

// ConfigKind returns the Kind for XLIFF 2.0 format config.
func (c *Config) ConfigKind() config.Kind { return config.FormatConfigKind("xliff2") }

// Reset restores default values.
func (c *Config) Reset() {
	*c = Config{
		WriteOriginalData: true,
	}
}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		// Extraction
		case "forceUniqueIds":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("forceUniqueIds: expected bool, got %T", val)
			}
			c.ForceUniqueIds = b
		case "ignoreTagTypeMatch":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("ignoreTagTypeMatch: expected bool, got %T", val)
			}
			c.IgnoreTagTypeMatch = b

		// States
		case "discardInvalidTargets":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("discardInvalidTargets: expected bool, got %T", val)
			}
			c.DiscardInvalidTargets = b

		// Output
		case "writeOriginalData":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("writeOriginalData: expected bool, got %T", val)
			}
			c.WriteOriginalData = b

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
			return fmt.Errorf("xliff2: unknown parameter: %s", key)
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
		for i := 0; i < count; i++ {
			key := fmt.Sprintf("rule%d", i)
			if rule, ok := m[key].(string); ok {
				rules = append(rules, rule)
			}
		}
		return rules, nil
	}
	return nil, fmt.Errorf("expected []string or map, got %T", val)
}
