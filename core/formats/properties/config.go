package properties

import (
	"fmt"
	"regexp"
)

// Config holds configuration for the Java Properties format.
type Config struct {
	// Separator is the key-value separator character. Default is '='.
	Separator string

	// UseJavaEscapes enables additional Java escape decoding: \: \= \# \!
	// in property values are decoded to their literal characters.
	UseJavaEscapes bool

	// UseKeyCondition enables key-based extraction filtering.
	// When true, entries are filtered based on KeyCondition.
	UseKeyCondition bool

	// ExtractOnlyMatchingKey controls the filter direction.
	// When true (default), only keys matching KeyCondition are extracted.
	// When false, matching keys are excluded.
	ExtractOnlyMatchingKey bool

	// KeyCondition is a regex pattern for key-based extraction filtering.
	KeyCondition string

	// ExtraComments enables recognition of semicolon (;) and double-slash (//)
	// comment styles in addition to standard # and ! markers.
	ExtraComments bool

	// CommentsAreNotes controls whether comments are treated as translator
	// notes attached to the following entry. Default is true.
	CommentsAreNotes bool

	// EscapeExtendedChars controls whether non-ASCII characters are escaped
	// using \uXXXX notation in output. Default is true.
	EscapeExtendedChars bool

	// UseCodeFinder enables regex-based inline code detection.
	UseCodeFinder bool

	// CodeFinderRules defines inline code patterns.
	CodeFinderRules []string

	// compiled regex caches
	compiledKeyCondition *regexp.Regexp
	compiledCodeFinder   []*regexp.Regexp
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "properties" }

// Reset restores default values.
func (c *Config) Reset() {
	*c = Config{
		Separator:              "=",
		ExtractOnlyMatchingKey: true,
		KeyCondition:           ".*text.*",
		CommentsAreNotes:       true,
		EscapeExtendedChars:    true,
	}
}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		case "separator":
			s, ok := val.(string)
			if !ok {
				return fmt.Errorf("separator: expected string, got %T", val)
			}
			c.Separator = s
		case "useJavaEscapes":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("useJavaEscapes: expected bool, got %T", val)
			}
			c.UseJavaEscapes = b
		case "useKeyCondition":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("useKeyCondition: expected bool, got %T", val)
			}
			c.UseKeyCondition = b
			c.compiledKeyCondition = nil
		case "extractOnlyMatchingKey":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("extractOnlyMatchingKey: expected bool, got %T", val)
			}
			c.ExtractOnlyMatchingKey = b
		case "keyCondition":
			s, ok := val.(string)
			if !ok {
				return fmt.Errorf("keyCondition: expected string, got %T", val)
			}
			c.KeyCondition = s
			c.compiledKeyCondition = nil
		case "extraComments":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("extraComments: expected bool, got %T", val)
			}
			c.ExtraComments = b
		case "commentsAreNotes":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("commentsAreNotes: expected bool, got %T", val)
			}
			c.CommentsAreNotes = b
		case "escapeExtendedChars":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("escapeExtendedChars: expected bool, got %T", val)
			}
			c.EscapeExtendedChars = b
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
			return fmt.Errorf("properties: unknown parameter: %s", key)
		}
	}
	return nil
}

// getKeyCondition returns the compiled key condition regex.
func (c *Config) getKeyCondition() *regexp.Regexp {
	if c.compiledKeyCondition == nil && c.KeyCondition != "" {
		c.compiledKeyCondition = regexp.MustCompile(c.KeyCondition)
	}
	return c.compiledKeyCondition
}

// shouldExtractKey returns true if a key should be extracted based on
// key condition settings.
func (c *Config) shouldExtractKey(key string) bool {
	if !c.UseKeyCondition {
		return true
	}
	re := c.getKeyCondition()
	if re == nil {
		return true
	}
	matches := re.MatchString(key)
	if c.ExtractOnlyMatchingKey {
		return matches
	}
	return !matches
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
