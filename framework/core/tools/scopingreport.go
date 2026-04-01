package tools

import (
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// Scoping category property key stored on Block.Properties.
const (
	PropScopingCategory = "scoping-category" // "new", "repetition", "exact-match", "fuzzy-match"
)

// Scoping category values.
const (
	ScopingNew        = "new"
	ScopingRepetition = "repetition"
	ScopingExactMatch = "exact-match"
	ScopingFuzzyMatch = "fuzzy-match"
)

// ScopingReportConfig holds configuration for the scoping report tool.
type ScopingReportConfig struct {
	TargetLocale model.LocaleID `schema:"description=Target locale for processing; if set includes target word counts"` // Optional — if set, includes target word counts
}

// ToolName returns the tool name this config applies to.
func (c *ScopingReportConfig) ToolName() string { return "scoping-report" }

// Reset restores default values.
func (c *ScopingReportConfig) Reset() { c.TargetLocale = "" }

// Validate checks configuration validity.
func (c *ScopingReportConfig) Validate() error { return nil }

// NewScopingReportTool creates a new scoping report tool.
// It classifies each translatable block into a scoping category based on
// properties set by upstream tools (repetition-analysis, diff-leverage).
func NewScopingReportTool(cfg *ScopingReportConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "scoping-report",
		ToolDescription: "Classifies blocks into scoping categories based on repetition and match status",
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

		if block.Properties == nil {
			block.Properties = make(map[string]string)
		}

		category := classifyScopingCategory(block)
		block.Properties[PropScopingCategory] = category

		return part, nil
	}
	return t
}

// classifyScopingCategory determines the scoping category of a block
// based on properties set by upstream analysis tools.
func classifyScopingCategory(block *model.Block) string {
	// Check repetition status first.
	if status, ok := block.Properties[PropRepetitionStatus]; ok && status == "repetition" {
		return ScopingRepetition
	}

	// Check diff-leverage status.
	if status, ok := block.Properties[PropDiffLeverageStatus]; ok {
		switch status {
		case "unchanged":
			return ScopingExactMatch
		case "leveraged":
			return ScopingFuzzyMatch
		}
	}

	return ScopingNew
}
