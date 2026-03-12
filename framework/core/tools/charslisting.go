package tools

import (
	"fmt"
	"strconv"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/tool"
)

// Chars listing property keys stored on Block.Properties.
const (
	PropCharsListingCount = "chars-listing-unique-count" // Number of unique chars in this block
)

// CharsListingConfig holds configuration for the chars listing tool.
type CharsListingConfig struct {
	IncludeSource bool           // Include source text (default: true)
	IncludeTarget bool           // Include target text (default: true)
	TargetLocale  model.LocaleID // Target locale (required when IncludeTarget)
}

// ToolName returns the tool name this config applies to.
func (c *CharsListingConfig) ToolName() string { return "chars-listing" }

// Reset restores default values.
func (c *CharsListingConfig) Reset() {
	c.IncludeSource = true
	c.IncludeTarget = true
	c.TargetLocale = ""
}

// Validate checks configuration validity.
func (c *CharsListingConfig) Validate() error {
	if !c.IncludeSource && !c.IncludeTarget {
		return fmt.Errorf("chars-listing: at least one of IncludeSource or IncludeTarget must be true")
	}
	if c.IncludeTarget && c.TargetLocale.IsEmpty() {
		return fmt.Errorf("chars-listing: TargetLocale is required when IncludeTarget is true")
	}
	return nil
}

// CharsListingResult wraps a BaseTool and provides access to accumulated character counts.
type CharsListingResult struct {
	tool  *tool.BaseTool
	chars map[rune]int
}

// CharCounts returns the accumulated character frequency map.
func (r *CharsListingResult) CharCounts() map[rune]int { return r.chars }

// Tool returns the underlying BaseTool for pipeline registration.
func (r *CharsListingResult) Tool() *tool.BaseTool { return r.tool }

// NewCharsListingTool creates a character listing tool.
// It scans source and/or target text, accumulates a map of all unique characters
// and their frequencies, and stores per-block unique character counts in Block.Properties.
func NewCharsListingTool(cfg *CharsListingConfig) *CharsListingResult {
	charCounts := make(map[rune]int)

	result := &CharsListingResult{
		chars: charCounts,
	}

	t := &tool.BaseTool{
		ToolName:        "chars-listing",
		ToolDescription: "Lists all unique characters used in content for font subsetting",
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

		conf := t.Cfg.(*CharsListingConfig)

		if block.Properties == nil {
			block.Properties = make(map[string]string)
		}

		blockChars := make(map[rune]struct{})

		if conf.IncludeSource {
			for _, r := range block.SourceText() {
				charCounts[r]++
				blockChars[r] = struct{}{}
			}
		}

		if conf.IncludeTarget && block.HasTarget(conf.TargetLocale) {
			for _, r := range block.TargetText(conf.TargetLocale) {
				charCounts[r]++
				blockChars[r] = struct{}{}
			}
		}

		block.Properties[PropCharsListingCount] = strconv.Itoa(len(blockChars))

		return part, nil
	}

	result.tool = t
	return result
}
