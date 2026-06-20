package csv

import (
	"errors"
	"fmt"
	"regexp"
)

// Config holds configuration for the CSV format.
type Config struct {
	// Separator is the field delimiter character. Default is ','.
	Separator rune
	// TextQualifier is the character used to quote field values. Default is '"'.
	TextQualifier rune
	// HasHeader if true, the first row is treated as headers.
	HasHeader bool
	// TranslatableColumns specifies which column indices (0-based) to extract
	// as translatable content. If empty, all columns are translatable.
	TranslatableColumns []int
	// KeyColumns specifies which column indices (0-based) provide the block ID
	// (source ID / record ID). Values from these columns are concatenated with "."
	// to form the block ID. If empty, row-based IDs are used.
	KeyColumns []int
	// CommentColumns specifies which column indices (0-based) contain comments
	// or notes. These are stored as block properties.
	CommentColumns []int
	// TrimValues if true, leading and trailing whitespace is removed from cell values.
	TrimValues bool
	// disableNonTranslatableContent, when set, keeps non-translatable contextual
	// content (preamble rows, non-translatable column cells) in opaque
	// model.Data parts instead of surfacing it as non-translatable content
	// Blocks (visible to ingestion/LLM consumers, skipped by MT). Zero value =
	// surfacing ON (the opt-out default). Independent of TranslatableColumns,
	// which only decides which cells are *translatable*.
	disableNonTranslatableContent bool
	// ValuesStartRow is the 1-based row number where data values begin.
	// Default is 0 (auto: row after header, or row 1 if no header).
	ValuesStartRow int
	// ColumnNamesRow is the 1-based row number that contains column names.
	// Default is 0 (auto: row 1 if HasHeader is true).
	ColumnNamesRow int
	// UseCodeFinder enables regex-based inline code detection.
	UseCodeFinder bool
	// CodeFinderRules defines inline code patterns.
	CodeFinderRules []string

	// compiled regex caches
	compiledCodeFinder []*regexp.Regexp
}

// FormatName returns the format this config applies to.
func (c *Config) FormatName() string {
	if c.Separator == '\t' {
		return "tsv"
	}
	return "csv"
}

// Reset restores default values.
func (c *Config) Reset() {
	*c = Config{
		Separator:     ',',
		TextQualifier: '"',
		HasHeader:     true,
	}
}

// Validate checks configuration validity.
func (c *Config) Validate() error {
	if c.Separator == 0 {
		return errors.New("csv: separator must not be zero")
	}
	return nil
}

// ExtractNonTranslatableContent reports whether non-translatable contextual
// content (preamble rows, non-translatable column cells) is surfaced as
// non-translatable content Blocks. Default true.
func (c *Config) ExtractNonTranslatableContent() bool {
	return !c.disableNonTranslatableContent
}

// SetExtractNonTranslatableContent toggles surfacing of non-translatable
// contextual content as content Blocks (used by the parity runner to match the
// Okapi bridge, which keeps such content in skeleton/Data).
func (c *Config) SetExtractNonTranslatableContent(v bool) {
	c.disableNonTranslatableContent = !v
}

// ApplyMap applies configuration values from a map.
func (c *Config) ApplyMap(values map[string]any) error {
	for key, val := range values {
		switch key {
		case "separator":
			s, ok := val.(string)
			if !ok {
				return fmt.Errorf("separator: expected string, got %T", val)
			}
			runes := []rune(s)
			if len(runes) != 1 {
				return fmt.Errorf("separator: expected single character, got %q", s)
			}
			c.Separator = runes[0]
		case "textQualifier":
			s, ok := val.(string)
			if !ok {
				return fmt.Errorf("textQualifier: expected string, got %T", val)
			}
			runes := []rune(s)
			if len(runes) != 1 {
				return fmt.Errorf("textQualifier: expected single character, got %q", s)
			}
			c.TextQualifier = runes[0]
		case "hasHeader":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("hasHeader: expected bool, got %T", val)
			}
			c.HasHeader = b
		case "translatableColumns":
			cols, err := parseIntSlice(val, key)
			if err != nil {
				return err
			}
			c.TranslatableColumns = cols
		case "keyColumns":
			cols, err := parseIntSlice(val, key)
			if err != nil {
				return err
			}
			c.KeyColumns = cols
		case "commentColumns":
			cols, err := parseIntSlice(val, key)
			if err != nil {
				return err
			}
			c.CommentColumns = cols
		case "trimValues":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("trimValues: expected bool, got %T", val)
			}
			c.TrimValues = b
		case "extractNonTranslatableContent":
			b, ok := val.(bool)
			if !ok {
				return fmt.Errorf("extractNonTranslatableContent: expected bool, got %T", val)
			}
			c.disableNonTranslatableContent = !b
		case "valuesStartRow":
			n, err := parseIntValue(val, key)
			if err != nil {
				return err
			}
			c.ValuesStartRow = n
		case "columnNamesRow":
			n, err := parseIntValue(val, key)
			if err != nil {
				return err
			}
			c.ColumnNamesRow = n
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
			return fmt.Errorf("csv: unknown parameter: %s", key)
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

// parseIntSlice parses an []any of numbers into []int.
func parseIntSlice(val any, key string) ([]int, error) {
	arr, ok := val.([]any)
	if !ok {
		return nil, fmt.Errorf("%s: expected []any, got %T", key, val)
	}
	cols := make([]int, 0, len(arr))
	for i, elem := range arr {
		switch v := elem.(type) {
		case float64:
			cols = append(cols, int(v))
		case int:
			cols = append(cols, v)
		default:
			return nil, fmt.Errorf("%s[%d]: expected number, got %T", key, i, elem)
		}
	}
	return cols, nil
}

// parseIntValue parses a single number value.
func parseIntValue(val any, key string) (int, error) {
	switch v := val.(type) {
	case float64:
		return int(v), nil
	case int:
		return v, nil
	default:
		return 0, fmt.Errorf("%s: expected number, got %T", key, val)
	}
}
