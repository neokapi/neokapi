package tools

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
)

// Chars listing property keys stored on Block.Properties.
const (
	PropCharsListingCount = "chars-listing-unique-count" // Number of unique chars in this block
)

// CharsListingConfig holds configuration for the chars listing tool.
type CharsListingConfig struct {
	IncludeSource bool           `json:"includeSource,omitempty" schema:"title=Include Source,description=Include source text in character listing,default=true"`
	IncludeTarget bool           `json:"includeTarget,omitempty" schema:"title=Include Target,description=Include target text in character listing,default=true"`
	TargetLocale  model.LocaleID `json:"targetLocale,omitempty"  schema:"-"`
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
		return errors.New("chars-listing: at least one of IncludeSource or IncludeTarget must be true")
	}
	if c.IncludeTarget && c.TargetLocale.IsEmpty() {
		return errors.New("chars-listing: TargetLocale is required when IncludeTarget is true")
	}
	return nil
}

// CharsListingSchema returns the auto-generated schema for the chars-listing tool.
func CharsListingSchema() *schema.ComponentSchema {
	return schema.FromStruct(&CharsListingConfig{}, schema.ToolMeta{
		ID:          "chars-listing",
		Category:    schema.CategoryAnalysis,
		DisplayName: "Chars Listing",
		Description: "List all distinct characters used in source and/or target",
	})
}

// NewCharsListingFromConfig creates a chars-listing tool from a config map.
func NewCharsListingFromConfig(config map[string]any, targetLang string) (tool.Tool, error) {
	var cfg CharsListingConfig
	if err := schema.ApplyConfig(config, &cfg); err != nil {
		return nil, fmt.Errorf("chars-listing config: %w", err)
	}
	if targetLang != "" {
		cfg.TargetLocale = model.LocaleID(targetLang)
	}
	return NewCharsListingTool(&cfg).Tool(), nil
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
	t.Annotate = func(v tool.BlockView) error {
		if !v.Translatable() {
			return nil
		}

		conf := t.Cfg.(*CharsListingConfig)

		blockChars := make(map[rune]struct{})

		if conf.IncludeSource {
			for _, r := range v.SourceText() {
				charCounts[r]++
				blockChars[r] = struct{}{}
			}
		}

		if conf.IncludeTarget && v.HasTarget(conf.TargetLocale) {
			for _, r := range v.TargetText(conf.TargetLocale) {
				charCounts[r]++
				blockChars[r] = struct{}{}
			}
		}

		v.SetProperty(PropCharsListingCount, strconv.Itoa(len(blockChars)))

		return nil
	}

	result.tool = t
	return result
}
