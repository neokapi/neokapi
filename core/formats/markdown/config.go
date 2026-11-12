package markdown

import (
	"fmt"
	"github.com/neokapi/neokapi/core/format"
	"regexp"
)

// Config holds configuration for the Markdown format.
type Config struct {
	// TranslateCodeBlocks controls whether fenced/indented code blocks are
	// translatable (emitted as Blocks). Default false = emitted as Data.
	TranslateCodeBlocks bool

	// TranslateFrontMatter controls whether YAML front matter values are
	// translatable. Default false = emitted as Data.
	TranslateFrontMatter bool

	// FrontMatterKeys restricts which front matter keys are extracted when
	// TranslateFrontMatter is on. Empty means every scalar value (the
	// historical behavior); set it to the prose-bearing keys (e.g.
	// ["title", "description"]) so numbers, slugs, and tag lists stay
	// skeleton.
	FrontMatterKeys []string

	// TranslateImageAlt controls whether image alt text is translatable
	// (included in the block's inline content). Default true (nonSkipImageAlt=false).
	nonSkipImageAlt bool

	// TranslateURLs controls whether link/image URLs are translatable.
	// Default false.
	TranslateURLs bool

	// TranslateBlockQuotes controls whether blockquote content is translatable.
	// Default true (nonTranslatableBlockQuotes=false).
	nonTranslatableBlockQuotes bool

	// TranslateHTMLBlocks controls whether HTML blocks are translatable.
	// Default false = emitted as Data.
	TranslateHTMLBlocks bool

	// UnescapeBackslashCharacters controls whether backslash-escaped
	// punctuation in source documents is parsed. Default false.
	UnescapeBackslashCharacters bool

	// Subfilter specifies a subfilter format to apply to HTML content
	// within Markdown (e.g., "html").
	Subfilter string

	// disableNonTranslatableContent, when set, keeps non-translatable contextual
	// content (code blocks) in opaque skeleton instead of surfacing it as
	// RoleCode content blocks (visible to ingestion, skipped by MT). Zero value
	// = surfacing ON (the opt-out default). Orthogonal to TranslateCodeBlocks,
	// which only decides whether code is *translatable*.
	disableNonTranslatableContent bool

	// UseCodeFinder enables regex-based inline code detection.
	UseCodeFinder bool

	// CodeFinderRules defines inline code patterns.
	CodeFinderRules []string

	// compiled regex caches
	compiledCodeFinder []*regexp.Regexp
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string { return "markdown" }

// Reset restores default values.
func (c *Config) Reset() {
	*c = Config{}
}

// Validate checks configuration validity.
func (c *Config) Validate() error { return nil }

// TranslateImageAlt returns true if image alt text should be translatable.
func (c *Config) TranslateImageAlt() bool {
	return !c.nonSkipImageAlt
}

// TranslateBlockQuotes returns true if blockquote content should be translatable.
func (c *Config) TranslateBlockQuotes() bool {
	return !c.nonTranslatableBlockQuotes
}

// ExtractNonTranslatableContent reports whether non-translatable contextual
// content (code blocks) is surfaced as RoleCode content blocks. Default true.
func (c *Config) ExtractNonTranslatableContent() bool {
	return !c.disableNonTranslatableContent
}

// SetExtractNonTranslatableContent toggles surfacing of non-translatable
// contextual content as content blocks (used by the parity runner to match the
// Okapi bridge, which keeps such content in skeleton).
func (c *Config) SetExtractNonTranslatableContent(v bool) {
	c.disableNonTranslatableContent = !v
}

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		case "translate_code_blocks":
			if v, ok := val.(bool); ok {
				c.TranslateCodeBlocks = v
			}
		case "extract_non_translatable_content":
			if v, ok := val.(bool); ok {
				c.disableNonTranslatableContent = !v
			}
		case "translate_front_matter":
			if v, ok := val.(bool); ok {
				c.TranslateFrontMatter = v
			}
		case "front_matter_keys":
			keys, err := toStringSlice(val)
			if err != nil {
				return fmt.Errorf("front_matter_keys: %w", err)
			}
			c.FrontMatterKeys = keys
		case "translate_image_alt":
			if v, ok := val.(bool); ok {
				c.nonSkipImageAlt = !v
			}
		case "translate_urls":
			if v, ok := val.(bool); ok {
				c.TranslateURLs = v
			}
		case "translate_block_quotes":
			if v, ok := val.(bool); ok {
				c.nonTranslatableBlockQuotes = !v
			}
		case "translate_html_blocks":
			if v, ok := val.(bool); ok {
				c.TranslateHTMLBlocks = v
			}
		case "unescape_backslash_characters":
			if v, ok := val.(bool); ok {
				c.UnescapeBackslashCharacters = v
			}
		case "subfilter":
			s, ok := val.(string)
			if !ok {
				return fmt.Errorf("subfilter: expected string, got %T", val)
			}
			c.Subfilter = s
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
		default:
			return format.UnknownParameterError(c, "markdown", key)
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

// toStringSlice coerces a YAML/JSON-decoded list into []string.
func toStringSlice(val any) ([]string, error) {
	if s, ok := val.([]string); ok {
		return s, nil
	}
	if arr, ok := val.([]any); ok {
		out := make([]string, 0, len(arr))
		for _, item := range arr {
			s, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("expected string, got %T", item)
			}
			out = append(out, s)
		}
		return out, nil
	}
	return nil, fmt.Errorf("expected list of strings, got %T", val)
}
