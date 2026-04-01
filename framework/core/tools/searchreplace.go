package tools

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// ReplacePair defines a single search-and-replace operation.
type ReplacePair struct {
	Search  string // The text or regex pattern to search for
	Replace string // The replacement text
	IsRegex bool   // If true, Search is treated as a regular expression
}

// SearchReplaceConfig holds configuration for the search-and-replace tool.
type SearchReplaceConfig struct {
	Pairs        []ReplacePair  `schema:"description=Search/replace pairs to apply"` // The search/replace pairs to apply
	TargetLocale model.LocaleID `schema:"description=Target locale; if set replacements also apply to target text"` // If set, also apply to target text for this locale
}

// ToolName returns the tool name this config applies to.
func (c *SearchReplaceConfig) ToolName() string { return "search-replace" }

// Reset restores default values.
func (c *SearchReplaceConfig) Reset() {
	c.Pairs = nil
	c.TargetLocale = ""
}

// Validate checks configuration validity.
func (c *SearchReplaceConfig) Validate() error {
	for i, pair := range c.Pairs {
		if pair.Search == "" {
			return fmt.Errorf("search-replace: pair %d has empty search string", i)
		}
		if pair.IsRegex {
			if _, err := regexp.Compile(pair.Search); err != nil {
				return fmt.Errorf("search-replace: pair %d has invalid regex %q: %w", i, pair.Search, err)
			}
		}
	}
	return nil
}

// NewSearchReplaceTool creates a new search-and-replace tool.
// It performs search and replace operations on Block source and target text.
func NewSearchReplaceTool(cfg *SearchReplaceConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "search-replace",
		ToolDescription: "Performs search and replace on block text",
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

		conf := t.Cfg.(*SearchReplaceConfig)
		if len(conf.Pairs) == 0 {
			return part, nil
		}

		// Apply to source text.
		sourceText := block.SourceText()
		newSource, err := applyReplacements(sourceText, conf.Pairs)
		if err != nil {
			return nil, fmt.Errorf("search-replace source: %w", err)
		}
		if newSource != sourceText {
			block.SetSourceText(newSource)
		}

		// Apply to target text if locale is set and target exists.
		if !conf.TargetLocale.IsEmpty() && block.HasTarget(conf.TargetLocale) {
			targetText := block.TargetText(conf.TargetLocale)
			newTarget, err := applyReplacements(targetText, conf.Pairs)
			if err != nil {
				return nil, fmt.Errorf("search-replace target: %w", err)
			}
			if newTarget != targetText {
				block.SetTargetText(conf.TargetLocale, newTarget)
			}
		}

		return part, nil
	}
	return t
}

// applyReplacements applies all replacement pairs to the given text.
func applyReplacements(text string, pairs []ReplacePair) (string, error) {
	result := text
	for _, pair := range pairs {
		if pair.IsRegex {
			re, err := regexp.Compile(pair.Search)
			if err != nil {
				return "", fmt.Errorf("invalid regex %q: %w", pair.Search, err)
			}
			result = re.ReplaceAllString(result, pair.Replace)
		} else {
			result = strings.ReplaceAll(result, pair.Search, pair.Replace)
		}
	}
	return result, nil
}
