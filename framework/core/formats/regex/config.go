package regex

import "fmt"

// Rule defines a regex extraction rule. The Pattern is compiled as a Go
// regexp and applied to each line (or the whole input if Multiline is true).
// Capture group indices specify which groups map to source text, block ID,
// and note content.
type Rule struct {
	// Pattern is the Go regular expression pattern.
	Pattern string

	// SourceGroup is the capture group index for the translatable source text.
	// Must be >= 1.
	SourceGroup int

	// IDGroup is the capture group index for the block ID/name.
	// 0 means auto-generate IDs.
	IDGroup int

	// NoteGroup is the capture group index for a note/comment.
	// 0 means no note extraction.
	NoteGroup int
}

// EscapeNone means no escape processing.
const EscapeNone = "none"

// EscapeBackslash means backslash escape sequences (e.g. \" \\ \n \t).
const EscapeBackslash = "backslash"

// EscapeDoubleChar means a character is escaped by doubling it (e.g. "" for ").
const EscapeDoubleChar = "doublechar"

// Config holds configuration for the Regex extraction format.
type Config struct {
	// Rules defines the regex extraction rules, processed in order.
	// Each match produces a Block; non-matching content becomes Data.
	Rules []Rule

	// EscapeType controls how escape sequences are handled in extracted text.
	// One of "none", "backslash", "doublechar". Default is "none".
	EscapeType string

	// EscapeChar is the escape character for doublechar mode.
	// Default is "\"" (double-quote).
	EscapeChar string
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "regex" }

// Reset restores default values.
func (c *Config) Reset() {
	c.Rules = nil
	c.EscapeType = EscapeNone
	c.EscapeChar = "\""
}

// Validate checks configuration validity.
func (c *Config) Validate() error {
	for i, r := range c.Rules {
		if r.Pattern == "" {
			return fmt.Errorf("regex: rule %d has empty pattern", i)
		}
		if r.SourceGroup < 1 {
			return fmt.Errorf("regex: rule %d sourceGroup must be >= 1", i)
		}
	}
	switch c.EscapeType {
	case "", EscapeNone, EscapeBackslash, EscapeDoubleChar:
		// ok
	default:
		return fmt.Errorf("regex: unknown escapeType: %q", c.EscapeType)
	}
	return nil
}

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		case "rules":
			rulesSlice, ok := val.([]any)
			if !ok {
				return fmt.Errorf("rules: expected []any, got %T", val)
			}
			var rules []Rule
			for i, rv := range rulesSlice {
				rm, ok := rv.(map[string]any)
				if !ok {
					return fmt.Errorf("rules[%d]: expected map, got %T", i, rv)
				}
				r, err := parseRule(rm, i)
				if err != nil {
					return err
				}
				rules = append(rules, r)
			}
			c.Rules = rules
		case "escapeType":
			s, ok := val.(string)
			if !ok {
				return fmt.Errorf("escapeType: expected string, got %T", val)
			}
			c.EscapeType = s
		case "escapeChar":
			s, ok := val.(string)
			if !ok {
				return fmt.Errorf("escapeChar: expected string, got %T", val)
			}
			c.EscapeChar = s
		default:
			return fmt.Errorf("regex: unknown parameter: %s", key)
		}
	}
	return nil
}

func parseRule(m map[string]any, index int) (Rule, error) {
	r := Rule{}
	if v, ok := m["pattern"]; ok {
		s, ok := v.(string)
		if !ok {
			return r, fmt.Errorf("rules[%d].pattern: expected string, got %T", index, v)
		}
		r.Pattern = s
	}
	if v, ok := m["sourceGroup"]; ok {
		n, err := toInt(v)
		if err != nil {
			return r, fmt.Errorf("rules[%d].sourceGroup: %w", index, err)
		}
		r.SourceGroup = n
	}
	if v, ok := m["idGroup"]; ok {
		n, err := toInt(v)
		if err != nil {
			return r, fmt.Errorf("rules[%d].idGroup: %w", index, err)
		}
		r.IDGroup = n
	}
	if v, ok := m["noteGroup"]; ok {
		n, err := toInt(v)
		if err != nil {
			return r, fmt.Errorf("rules[%d].noteGroup: %w", index, err)
		}
		r.NoteGroup = n
	}
	return r, nil
}

func toInt(v any) (int, error) {
	switch n := v.(type) {
	case int:
		return n, nil
	case int64:
		return int(n), nil
	case float64:
		return int(n), nil
	default:
		return 0, fmt.Errorf("expected number, got %T", v)
	}
}
