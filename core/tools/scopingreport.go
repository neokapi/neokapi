package tools

import (
	"fmt"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
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
	TargetLocale model.LocaleID `json:"targetLocale,omitempty" schema:"-"`
}

// ToolName returns the tool name this config applies to.
func (c *ScopingReportConfig) ToolName() string { return "scoping-report" }

// Reset restores default values.
func (c *ScopingReportConfig) Reset() { c.TargetLocale = "" }

// Validate checks configuration validity.
func (c *ScopingReportConfig) Validate() error { return nil }

// ScopingReportSchema returns the auto-generated schema for the scoping-report tool.
func ScopingReportSchema() *schema.ComponentSchema {
	return schema.FromStruct(&ScopingReportConfig{}, schema.ToolMeta{
		ID:          "scoping-report",
		Category:    schema.CategoryAnalysis,
		DisplayName: "Scoping Report",
		Description: "Generate detailed scoping report (word counts, repetitions, file breakdown)",
	})
}

// NewScopingReportFromConfig creates a scoping-report tool from a config map.
func NewScopingReportFromConfig(config map[string]any, targetLang string) (tool.Tool, error) {
	var cfg ScopingReportConfig
	if err := schema.ApplyConfig(config, &cfg); err != nil {
		return nil, fmt.Errorf("scoping-report config: %w", err)
	}
	if targetLang != "" {
		cfg.TargetLocale = model.LocaleID(targetLang)
	}
	return NewScopingReportTool(&cfg), nil
}

// NewScopingReportTool creates a new scoping report tool.
// It classifies each translatable block into a scoping category based on
// properties set by upstream tools (repetition-analysis, diff-leverage).
func NewScopingReportTool(cfg *ScopingReportConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "scoping-report",
		ToolDescription: "Classifies blocks into scoping categories based on repetition and match status",
		Cfg:             cfg,
	}
	t.Annotate = func(v tool.BlockView) error {
		if !v.Translatable() {
			return nil
		}

		category := classifyScopingCategory(v)
		v.SetProperty(PropScopingCategory, category)

		return nil
	}
	return t
}

// classifyScopingCategory determines the scoping category of a block
// based on properties set by upstream analysis tools.
func classifyScopingCategory(v tool.BlockView) string {
	// Check repetition status first.
	if rep, ok := v.Annotations()[string(model.AnnoRepetition)].(*RepetitionAnnotation); ok && rep.Status == "repetition" {
		return ScopingRepetition
	}

	// Check diff-leverage status.
	if status := v.Property(PropDiffLeverageStatus); status != "" {
		switch status {
		case "unchanged":
			return ScopingExactMatch
		case "leveraged":
			return ScopingFuzzyMatch
		}
	}

	return ScopingNew
}
