package tools

import (
	"fmt"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/tool"
)

// Translation comparison property keys stored on Block.Properties.
const (
	PropComparisonStatus = "comparison-status" // "identical", "different", "missing-locale1", "missing-locale2", "missing-both"
	PropComparisonDiff   = "comparison-diff"   // Simple diff description
)

// TranslationComparisonConfig holds configuration for the translation comparison tool.
type TranslationComparisonConfig struct {
	Locale1 model.LocaleID // First target locale to compare (required)
	Locale2 model.LocaleID // Second target locale to compare (required)
}

// ToolName returns the tool name this config applies to.
func (c *TranslationComparisonConfig) ToolName() string { return "translation-comparison" }

// Reset restores default values.
func (c *TranslationComparisonConfig) Reset() {
	c.Locale1 = ""
	c.Locale2 = ""
}

// Validate checks configuration validity.
func (c *TranslationComparisonConfig) Validate() error {
	if c.Locale1.IsEmpty() {
		return fmt.Errorf("translation-comparison: Locale1 is required")
	}
	if c.Locale2.IsEmpty() {
		return fmt.Errorf("translation-comparison: Locale2 is required")
	}
	return nil
}

// NewTranslationComparisonTool creates a tool that compares translations across
// two target locales for the same source text and reports differences.
// Results are stored in Block.Properties using PropComparisonStatus and PropComparisonDiff.
func NewTranslationComparisonTool(cfg *TranslationComparisonConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "translation-comparison",
		ToolDescription: "Compares translations across two target locales and reports differences",
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

		conf := t.Cfg.(*TranslationComparisonConfig)

		if block.Properties == nil {
			block.Properties = make(map[string]string)
		}

		has1 := block.HasTarget(conf.Locale1)
		has2 := block.HasTarget(conf.Locale2)

		switch {
		case !has1 && !has2:
			block.Properties[PropComparisonStatus] = "missing-both"
			block.Properties[PropComparisonDiff] = fmt.Sprintf(
				"Both %s and %s translations are missing", conf.Locale1, conf.Locale2)

		case !has1:
			block.Properties[PropComparisonStatus] = "missing-locale1"
			block.Properties[PropComparisonDiff] = fmt.Sprintf(
				"Translation for %s is missing", conf.Locale1)

		case !has2:
			block.Properties[PropComparisonStatus] = "missing-locale2"
			block.Properties[PropComparisonDiff] = fmt.Sprintf(
				"Translation for %s is missing", conf.Locale2)

		default:
			text1 := block.TargetText(conf.Locale1)
			text2 := block.TargetText(conf.Locale2)

			if text1 == text2 {
				block.Properties[PropComparisonStatus] = "identical"
				block.Properties[PropComparisonDiff] = fmt.Sprintf(
					"Translations for %s and %s are identical", conf.Locale1, conf.Locale2)
			} else {
				block.Properties[PropComparisonStatus] = "different"
				block.Properties[PropComparisonDiff] = fmt.Sprintf(
					"%s: %q vs %s: %q", conf.Locale1, text1, conf.Locale2, text2)
			}
		}

		return part, nil
	}
	return t
}
