package tools

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
)

// Character count property keys stored on Block.Properties.
const (
	PropCharCountSource        = "char-count-source"
	PropCharCountSourceNospace = "char-count-source-nospace"
	PropCharCountTarget        = "char-count-target"
	PropCharCountTargetNospace = "char-count-target-nospace"
)

// CharCountConfig holds configuration for the character count tool.
type CharCountConfig struct {
	Locale model.LocaleID `json:"locale,omitempty" schema:"-"`

	CountSource bool `json:"countSource,omitempty" schema:"title=Count Source,description=Count characters in source text,default=true"`
	CountTarget bool `json:"countTarget,omitempty" schema:"title=Count Target,description=Count characters in target text,default=true"`
	CountInline bool `json:"countInline,omitempty" schema:"title=Count Inline Codes,description=Include inline code content in character counts"`
}

// ToolName returns the tool name this config applies to.
func (c *CharCountConfig) ToolName() string { return "char-count" }

// Reset restores default values.
func (c *CharCountConfig) Reset() {
	c.Locale = ""
	c.CountSource = true
	c.CountTarget = true
	c.CountInline = false
}

// Validate checks configuration validity.
func (c *CharCountConfig) Validate() error {
	return nil
}

// CharCountSchema returns the auto-generated schema for the char-count tool.
func CharCountSchema() *schema.ComponentSchema {
	return schema.FromStruct(&CharCountConfig{}, schema.ToolMeta{
		ID:          "char-count",
		Category:    schema.CategoryAnalysis,
		DisplayName: "Char Count",
		Description: "Count characters in source and target text",
		Inputs:      []string{schema.PartTypeBlock},
	})
}

// NewCharCountFromConfig creates a char-count tool from a config map.
func NewCharCountFromConfig(config map[string]any, targetLang string) (tool.Tool, error) {
	var cfg CharCountConfig
	if err := schema.ApplyConfig(config, &cfg); err != nil {
		return nil, fmt.Errorf("char-count config: %w", err)
	}
	if targetLang != "" {
		cfg.Locale = model.LocaleID(targetLang)
	}
	return NewCharCountTool(&cfg), nil
}

// NewCharCountTool creates a new character count tool.
// It counts characters (with and without spaces) in Blocks and
// stores the counts in Block.Properties.
func NewCharCountTool(cfg *CharCountConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "char-count",
		ToolDescription: "Counts characters in source and target text of blocks",
		Cfg:             cfg,
	}
	t.Annotate = func(v tool.BlockView) error {
		if !v.Translatable() {
			return nil
		}

		conf := t.Cfg.(*CharCountConfig)

		// Default to counting both when neither scope is explicitly set.
		countSource := conf.CountSource || (!conf.CountSource && !conf.CountTarget)
		countTarget := conf.CountTarget || (!conf.CountSource && !conf.CountTarget)

		// Count source characters.
		if countSource {
			sourceText := v.SourceText()
			v.SetProperty(PropCharCountSource, strconv.Itoa(countChars(sourceText)))
			v.SetProperty(PropCharCountSourceNospace, strconv.Itoa(countCharsNoSpace(sourceText)))
		}

		// Count target characters if locale is set and target exists.
		if countTarget && !conf.Locale.IsEmpty() && v.HasTarget(conf.Locale) {
			targetText := v.TargetText(conf.Locale)
			v.SetProperty(PropCharCountTarget, strconv.Itoa(countChars(targetText)))
			v.SetProperty(PropCharCountTargetNospace, strconv.Itoa(countCharsNoSpace(targetText)))
		}

		return nil
	}
	return t
}

// countChars returns the number of Unicode characters (runes) in the text.
func countChars(text string) int {
	return utf8.RuneCountInString(text)
}

// countCharsNoSpace returns the number of non-space Unicode characters in the text.
func countCharsNoSpace(text string) int {
	noSpaces := strings.ReplaceAll(text, " ", "")
	return utf8.RuneCountInString(noSpaces)
}
