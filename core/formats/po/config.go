package po

import (
	"fmt"
	"github.com/neokapi/neokapi/core/format"
	"regexp"
)

// Config holds configuration for the PO format.
type Config struct {
	// PreserveUntranslated if true, emits blocks for entries with empty msgstr.
	PreserveUntranslated bool

	// BilingualMode if true (default), treats msgid as source text and
	// msgstr as target text. If false, treats msgid as an ID and
	// extracts the msgstr as the source text (monolingual mode).
	BilingualMode bool

	// WrapContent if true (default), wraps long content lines in output.
	WrapContent bool

	// AllowEmptyOutputTarget if true, allows empty target in output
	// when no translation is available.
	AllowEmptyOutputTarget bool

	// UseCodeFinder enables regex-based inline code detection.
	UseCodeFinder bool

	// CodeFinderRules defines inline code patterns.
	CodeFinderRules []string

	// ProtectApproved mirrors Okapi's `protectApproved` parameter —
	// when true, fully-translated, non-fuzzy entries are emitted as
	// non-translatable. The native reader accepts this flag for
	// configuration parity but does not yet act on it.
	ProtectApproved bool

	// MakeID mirrors Okapi's `makeID` parameter — generate stable
	// identifiers from the source text. Accepted for parity; the
	// native reader currently uses the msgid value as the Block name.
	MakeID bool

	// IncludeMsgContextInNote mirrors Okapi's
	// `include_msg_context_in_note` parameter — append the msgctxt value
	// to the note annotation. Accepted for parity; the native reader
	// stores msgctxt under the block's `context` property regardless.
	IncludeMsgContextInNote bool

	// OutputGeneric mirrors Okapi's `outputGeneric` parameter — write
	// the document in Okapi's generic format. Accepted for parity;
	// the native writer always emits the canonical PO format.
	OutputGeneric bool

	// compiled regex caches
	compiledCodeFinder []*regexp.Regexp
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "po" }

// Reset restores default values.
func (c *Config) Reset() {
	*c = Config{
		PreserveUntranslated: true,
		BilingualMode:        true,
		WrapContent:          true,
	}
}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		case "preserve_untranslated":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("preserve_untranslated: expected bool, got %T", val)
			}
			c.PreserveUntranslated = b
		case "bilingual_mode":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("bilingual_mode: expected bool, got %T", val)
			}
			c.BilingualMode = b
		case "wrap_content":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("wrap_content: expected bool, got %T", val)
			}
			c.WrapContent = b
		case "allow_empty_output_target":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("allow_empty_output_target: expected bool, got %T", val)
			}
			c.AllowEmptyOutputTarget = b
		case "use_code_finder":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("use_code_finder: expected bool, got %T", val)
			}
			c.UseCodeFinder = b
			c.compiledCodeFinder = nil
		case "code_finder_rules":
			rules, err := parseCodeFinderRules(val)
			if err != nil {
				return fmt.Errorf("code_finder_rules: %w", err)
			}
			c.CodeFinderRules = rules
			c.compiledCodeFinder = nil
		case "protect_approved":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("protect_approved: expected bool, got %T", val)
			}
			c.ProtectApproved = b
		case "make_id":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("make_id: expected bool, got %T", val)
			}
			c.MakeID = b
		case "include_msg_context_in_note":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("include_msg_context_in_note: expected bool, got %T", val)
			}
			c.IncludeMsgContextInNote = b
		case "output_generic":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("output_generic: expected bool, got %T", val)
			}
			c.OutputGeneric = b
		default:
			return format.UnknownParameterError(c, "po", key)
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
