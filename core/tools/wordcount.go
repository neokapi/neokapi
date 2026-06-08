package tools

import (
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
)

// Word count property keys stored on Block.Properties.
const (
	PropWordCountSource = "word-count-source"
	// PropWordCountTargetPrefix is used for per-locale target word counts.
	// The full key is PropWordCountTargetPrefix + locale, e.g. "word-count-target:fr".
	PropWordCountTargetPrefix = "word-count-target:"
)

// WordCountConfig holds configuration for the word count tool.
type WordCountConfig struct {
	CountSource bool `json:"countSource,omitempty" schema:"title=Count Source,description=Count words in source text,default=true"`
	CountTarget bool `json:"countTarget,omitempty" schema:"title=Count Target,description=Count words in target text,default=true"`
	CountInline bool `json:"countInline,omitempty" schema:"title=Count Inline Codes,description=Include inline code content in word counts"`
}

// ToolName returns the tool name this config applies to.
func (c *WordCountConfig) ToolName() string { return "word-count" }

// Reset restores default values.
func (c *WordCountConfig) Reset() {
	c.CountSource = true
	c.CountTarget = true
	c.CountInline = false
}

// Validate checks configuration validity.
func (c *WordCountConfig) Validate() error {
	return nil
}

// WordCountSchema returns the auto-generated schema for the word-count tool.
func WordCountSchema() *schema.ComponentSchema {
	return schema.FromStruct(&WordCountConfig{}, schema.ToolMeta{
		ID:          "word-count",
		Category:    schema.CategoryAnalysis,
		DisplayName: "Word Count",
		Description: "Count words in source and target text",
	})
}

// NewWordCountFromConfig creates a word-count tool from a config map.
func NewWordCountFromConfig(config map[string]any, targetLang string) (tool.Tool, error) {
	var cfg WordCountConfig
	if err := schema.ApplyConfig(config, &cfg); err != nil {
		return nil, fmt.Errorf("word-count config: %w", err)
	}
	return NewWordCountTool(&cfg), nil
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
	// Annotate: word-count reads source/target and writes only properties.
	t.Annotate = func(v tool.BlockView) error {
		if !v.Translatable() {
			return nil
		}

		conf := t.Cfg.(*WordCountConfig)

		// Default to counting both when neither scope is explicitly set.
		countSource := conf.CountSource || (!conf.CountSource && !conf.CountTarget)
		countTarget := conf.CountTarget || (!conf.CountSource && !conf.CountTarget)

		// Count source words.
		wc := &WordCountFacet{}
		if countSource {
			wc.Source = v.WordCount()
		}

		// Count target words for every target locale present.
		if countTarget {
			for _, locale := range v.TargetLocales() {
				if wc.Targets == nil {
					wc.Targets = make(map[model.LocaleID]int)
				}
				wc.Targets[locale] = countWords(v.TargetText(locale))
			}
		}
		v.Annotate(string(model.AnnoWordCount), wc)

		return nil
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
