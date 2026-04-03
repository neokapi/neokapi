package mif

import (
	"fmt"

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
			`^[A-Z]{1}:`,
			`\u2022`,
			`\\t`,
			`<[naArR ]{1}[+]*>`,
			`<[naArR]{1}=[0-9]+>`,
			`<\$.*?>`,
			`<Default \u00B6 Font>`,
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
		case "codeFinderRules":
			rules, err := parseCodeFinderRules(val)
			if err != nil {
				return fmt.Errorf("codeFinderRules: %w", err)
			}
			c.CodeFinderRules = rules
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
