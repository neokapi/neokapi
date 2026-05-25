package tools

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/tool"
)

// Tag protection property keys.
const (
	PropTagProtectCount = "tag-protect-count"
)

// TagProtectConfig holds configuration for the tag protection tool.
type TagProtectConfig struct {
	Patterns []string `schema:"title=Protection Patterns,description=Regex patterns for tags and placeholders to protect,widget=regex"` // Regex patterns for tags to protect
}

// ToolName returns the tool name this config applies to.
func (c *TagProtectConfig) ToolName() string { return "tag-protect" }

// Reset restores default values.
func (c *TagProtectConfig) Reset() {
	c.Patterns = nil
}

// Validate checks configuration validity.
func (c *TagProtectConfig) Validate() error {
	for i, pat := range c.Patterns {
		if pat == "" {
			return fmt.Errorf("tag-protect: pattern %d is empty", i)
		}
		if _, err := regexp.Compile(pat); err != nil {
			return fmt.Errorf("tag-protect: pattern %d is invalid: %w", i, err)
		}
	}
	return nil
}

// defaultTagPatterns matches common markup tags (HTML/XML), printf placeholders,
// and ICU message format placeholders.
var defaultTagPatterns = []string{
	`<[^>]+>`,            // HTML/XML tags
	`\{[^}]+\}`,          // curly brace placeholders (ICU, Java, .NET)
	`%[sdfoexXbBhHtT%]+`, // printf-style placeholders
	`\$\{[^}]+\}`,        // ${...} template expressions
}

// NewTagProtectTool creates a tool that identifies and marks tags/placeholders
// in source text. Protected patterns are stored as annotations for downstream
// tools (e.g., MT connectors) to preserve.
func NewTagProtectTool(cfg *TagProtectConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "tag-protect",
		ToolDescription: "Identifies and marks tags and placeholders for protection",
		Cfg:             cfg,
	}
	t.Annotate = func(v tool.BlockView) error {
		if !v.Translatable() {
			return nil
		}

		conf := t.Cfg.(*TagProtectConfig)
		patterns := conf.Patterns
		if len(patterns) == 0 {
			patterns = defaultTagPatterns
		}

		protected := findProtectedTags(v.SourceText(), patterns)

		v.SetProperty(PropTagProtectCount, strconv.Itoa(len(protected)))

		if len(protected) > 0 {
			v.Annotate("protected-tags", &ProtectedTags{Tags: protected})
		}

		return nil
	}
	return t
}

// ProtectedTags is an annotation listing tags/placeholders that should be preserved.
type ProtectedTags struct {
	Tags []ProtectedTag
}

// AnnotationType returns the annotation type identifier.
func (pt *ProtectedTags) AnnotationType() string { return "protected-tags" }

// ProtectedTag represents a single protected tag/placeholder found in text.
type ProtectedTag struct {
	Text   string // The matched tag text
	Offset int    // Byte offset in source text
}

func findProtectedTags(text string, patterns []string) []ProtectedTag {
	var tags []ProtectedTag
	seen := make(map[string]bool) // deduplicate by offset+text

	for _, pat := range patterns {
		re, err := regexp.Compile(pat)
		if err != nil {
			continue
		}
		matches := re.FindAllStringIndex(text, -1)
		for _, m := range matches {
			matched := text[m[0]:m[1]]
			key := fmt.Sprintf("%d:%s", m[0], matched)
			if seen[key] {
				continue
			}
			seen[key] = true
			tags = append(tags, ProtectedTag{
				Text:   matched,
				Offset: m[0],
			})
		}
	}

	// Sort by offset (already naturally ordered per-pattern, but merge order may differ).
	for i := 1; i < len(tags); i++ {
		for j := i; j > 0 && tags[j].Offset < tags[j-1].Offset; j-- {
			tags[j], tags[j-1] = tags[j-1], tags[j]
		}
	}

	return tags
}

// ReplaceProtectedTags replaces protected tags in text with numbered placeholders.
// Returns the modified text and a mapping from placeholder to original tag.
func ReplaceProtectedTags(text string, tags []ProtectedTag) (string, map[string]string) {
	if len(tags) == 0 {
		return text, nil
	}

	mapping := make(map[string]string)
	result := text

	// Replace in reverse order to preserve offsets.
	for i := len(tags) - 1; i >= 0; i-- {
		tag := tags[i]
		placeholder := fmt.Sprintf("{%d}", i+1)
		mapping[placeholder] = tag.Text
		result = strings.Replace(result, tag.Text, placeholder, 1)
	}

	return result, mapping
}

// RestoreProtectedTags restores the original tags from their placeholders.
func RestoreProtectedTags(text string, mapping map[string]string) string {
	result := text
	for placeholder, original := range mapping {
		result = strings.ReplaceAll(result, placeholder, original)
	}
	return result
}
