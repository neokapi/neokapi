package mif

import (
	"fmt"
	"regexp"

	"github.com/neokapi/neokapi/core/config"
)

// Config holds configuration for the MIF format.
// Options mirror the Okapi Framework MIF filter parameters.
type Config struct {
	// ExtractBodyPages controls whether body page content is extracted.
	// Defaults to true.
	ExtractBodyPages bool

	// ExtractMasterPages controls whether master page content is extracted.
	// Defaults to true.
	ExtractMasterPages bool

	// ExtractReferencePages controls whether reference page content is extracted.
	// Defaults to true.
	ExtractReferencePages bool

	// ExtractHiddenPages controls whether hidden page content is extracted.
	// Defaults to true.
	ExtractHiddenPages bool

	// ExtractVariables controls whether variable definitions are extracted.
	// Defaults to true.
	ExtractVariables bool

	// ExtractIndexMarkers controls whether index marker text is extracted.
	// Defaults to true.
	ExtractIndexMarkers bool

	// ExtractLinks controls whether hyperlink text is extracted.
	// Defaults to false.
	ExtractLinks bool

	// ExtractReferenceFormats controls whether cross-reference format strings
	// are extracted for translation.
	// Defaults to false.
	ExtractReferenceFormats bool

	// ExtractPgfNumFormatsInline controls whether paragraph numbering format
	// strings are extracted as inline codes.
	// Defaults to false.
	ExtractPgfNumFormatsInline bool

	// ExtractHardReturnsAsText controls whether hard returns within paragraphs
	// are preserved as text (newlines). When false, they are treated as
	// paragraph breaks.
	// Defaults to true.
	ExtractHardReturnsAsText bool

	// UseCodeFinder enables regex-based inline code detection in extracted text.
	// Defaults to true.
	UseCodeFinder bool

	// CodeFinderRules defines regex patterns for detecting inline codes.
	CodeFinderRules []string

	// compiledCodeFinder caches the regex.Compile result so repeated
	// reader calls don't re-parse the rules. Reset() clears it.
	compiledCodeFinder []*regexp.Regexp
}

// GetCodeFinderPatterns returns the compiled regex patterns when
// UseCodeFinder is on, lazily building (and caching) them on first
// call. Patterns that fail to compile are skipped silently — matching
// the behaviour of other format readers (po/markdown).
func (c *Config) GetCodeFinderPatterns() []*regexp.Regexp {
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

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "mif" }

// ConfigKind returns the Kind for MIF format config.
func (c *Config) ConfigKind() config.Kind { return config.FormatConfigKind("mif") }

// Reset restores default values matching Okapi's MIF filter defaults.
func (c *Config) Reset() {
	*c = Config{
		ExtractBodyPages:           true,
		ExtractMasterPages:         true,
		ExtractReferencePages:      true,
		ExtractHiddenPages:         true,
		ExtractVariables:           true,
		ExtractIndexMarkers:        true,
		ExtractLinks:               false,
		ExtractReferenceFormats:    false,
		ExtractPgfNumFormatsInline: false,
		ExtractHardReturnsAsText:   true,
		UseCodeFinder:              true,
		CodeFinderRules: []string{
			// Note: okapi's MIF Parameters.java adds `^[A-Z]{1}:` to its
			// codeFinder rule list, but okapi also removes leading and
			// trailing codes back into the skeleton via
			// TextUnitSimplification + CodeSimplifier.simplifyAll(tf, true,
			// true). Empirically the bridge-side TextUnit handed to Go for
			// fixtures like Test01.mif's `<String P:Body>` cell content
			// arrives as a single text run carrying the full literal
			// "P:Body"; the rule has no observable effect on the
			// translatable content okapi exposes. Including it on the
			// native side splits a leading `P:` into a placeholder, which
			// then survives the pseudo-translate transform unchanged
			// (yielding `P:Body` -> `P:_pseudo(Body)_`) while okapi's
			// reference round-trip pseudo-translates the whole literal
			// (`P:Body` -> `_pseudo(P:Body)_`). The leading-prefix rule is
			// applied contextually inside reader.go for PgfNumFormat
			// blocks instead, where it IS observably needed.
			//
			// Bullet (U+2022) and pilcrow (U+00B6) are written as
			// `\x{NNNN}` codepoint escapes \u2014 Go's regexp engine does
			// NOT interpret Java/Perl-style `\uNNNN` (it requires
			// `\x{NNNN}` braces for non-ASCII codepoints), so the prior
			// `\u2022` and `\u00B6` strings compiled to never-matching
			// patterns. This bug silently broke pseudo-translate
			// protection for `<Default \u00B6 Font>` and bullet codes inside
			// VariableDef / paragraph text.
			`\x{2022}`,
			`\\t`,
			`<[naArR ]{1}[+]*>`,
			`<[naArR]{1}=[0-9]+>`,
			`<\$.*?>`,
			`<Default \x{00B6} Font>`,
		},
	}
}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		case "extractBodyPages":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("extractBodyPages: expected bool, got %T", val)
			}
			c.ExtractBodyPages = b
		case "extractMasterPages":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("extractMasterPages: expected bool, got %T", val)
			}
			c.ExtractMasterPages = b
		case "extractReferencePages":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("extractReferencePages: expected bool, got %T", val)
			}
			c.ExtractReferencePages = b
		case "extractHiddenPages":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("extractHiddenPages: expected bool, got %T", val)
			}
			c.ExtractHiddenPages = b
		case "extractVariables":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("extractVariables: expected bool, got %T", val)
			}
			c.ExtractVariables = b
		case "extractIndexMarkers":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("extractIndexMarkers: expected bool, got %T", val)
			}
			c.ExtractIndexMarkers = b
		case "extractLinks":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("extractLinks: expected bool, got %T", val)
			}
			c.ExtractLinks = b
		case "extractReferenceFormats":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("extractReferenceFormats: expected bool, got %T", val)
			}
			c.ExtractReferenceFormats = b
		case "extractPgfNumFormatsInline":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("extractPgfNumFormatsInline: expected bool, got %T", val)
			}
			c.ExtractPgfNumFormatsInline = b
		case "extractHardReturnsAsText":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("extractHardReturnsAsText: expected bool, got %T", val)
			}
			c.ExtractHardReturnsAsText = b
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
			return fmt.Errorf("mif: unknown parameter: %s", key)
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
