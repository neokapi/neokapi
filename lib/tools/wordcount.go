package tools

import (
	"strconv"
	"strings"

	"github.com/asgeirf/gokapi/core/model"
	"github.com/asgeirf/gokapi/core/tool"
)

// Word count property keys stored on Block.Properties.
const (
	PropWordCountSource = "word-count-source"
	PropWordCountTarget = "word-count-target"
	// PropWordCountTargetPrefix is used for per-locale target word counts.
	// The full key is PropWordCountTargetPrefix + locale, e.g. "word-count-target:fr".
	PropWordCountTargetPrefix = "word-count-target:"
)

// WordCountConfig holds configuration for the word count tool.
type WordCountConfig struct {
	Locale model.LocaleID // Target locale for counting target words
}

// ToolName returns the tool name this config applies to.
func (c *WordCountConfig) ToolName() string { return "word-count" }

// Reset restores default values.
func (c *WordCountConfig) Reset() {
	c.Locale = ""
}

// Validate checks configuration validity.
func (c *WordCountConfig) Validate() error {
	return nil
}

// NewWordCountTool creates a new word count tool.
// It counts words in Block source and target text and stores the
// counts in Block.Properties.
func NewWordCountTool(cfg *WordCountConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "word-count",
		ToolDescription: "Counts words in source and target text of blocks",
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

		conf := t.Cfg.(*WordCountConfig)

		if block.Properties == nil {
			block.Properties = make(map[string]string)
		}

		// Count source words.
		sourceText := block.SourceText()
		sourceCount := countWords(sourceText)
		block.Properties[PropWordCountSource] = strconv.Itoa(sourceCount)

		// Count target words.
		if !conf.Locale.IsEmpty() {
			// Single-locale mode: count for the specified locale.
			if block.HasTarget(conf.Locale) {
				targetText := block.TargetText(conf.Locale)
				targetCount := countWords(targetText)
				block.Properties[PropWordCountTarget] = strconv.Itoa(targetCount)
			}
		} else {
			// All-locale mode: count every target locale present.
			for locale := range block.Targets {
				targetText := block.TargetText(locale)
				targetCount := countWords(targetText)
				block.Properties[PropWordCountTargetPrefix+string(locale)] = strconv.Itoa(targetCount)
			}
		}

		return part, nil
	}
	return t
}

// countWords counts the number of words in a text string.
// Words are sequences of non-whitespace characters separated by whitespace.
func countWords(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	return len(strings.Fields(text))
}
