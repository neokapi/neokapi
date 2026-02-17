package tools

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gokapi/gokapi/model"
	"github.com/gokapi/gokapi/tool"
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
	Locale model.LocaleID // Target locale for counting target characters
}

// ToolName returns the tool name this config applies to.
func (c *CharCountConfig) ToolName() string { return "char-count" }

// Reset restores default values.
func (c *CharCountConfig) Reset() {
	c.Locale = ""
}

// Validate checks configuration validity.
func (c *CharCountConfig) Validate() error {
	return nil
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
	t.HandleBlockFn = func(part *model.Part) (*model.Part, error) {
		block, ok := part.Resource.(*model.Block)
		if !ok {
			return part, nil
		}
		if !block.Translatable {
			return part, nil
		}

		conf := t.Cfg.(*CharCountConfig)

		if block.Properties == nil {
			block.Properties = make(map[string]string)
		}

		// Count source characters.
		sourceText := block.SourceText()
		block.Properties[PropCharCountSource] = strconv.Itoa(countChars(sourceText))
		block.Properties[PropCharCountSourceNospace] = strconv.Itoa(countCharsNoSpace(sourceText))

		// Count target characters if locale is set and target exists.
		if !conf.Locale.IsEmpty() && block.HasTarget(conf.Locale) {
			targetText := block.TargetText(conf.Locale)
			block.Properties[PropCharCountTarget] = strconv.Itoa(countChars(targetText))
			block.Properties[PropCharCountTargetNospace] = strconv.Itoa(countCharsNoSpace(targetText))
		}

		return part, nil
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
