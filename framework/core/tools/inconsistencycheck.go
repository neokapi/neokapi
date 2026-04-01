package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// Inconsistency check property keys stored on Block.Properties.
const (
	PropInconsistencyStatus  = "inconsistency-status"  // "consistent", "inconsistent"
	PropInconsistencyType    = "inconsistency-type"    // "target-inconsistency", "source-inconsistency"
	PropInconsistencyDetails = "inconsistency-details" // JSON: alternative translations/sources seen
)

// InconsistencyCheckConfig holds configuration for the inconsistency check tool.
type InconsistencyCheckConfig struct {
	TargetLocale             model.LocaleID `schema:"description=Target locale for processing"` // Required
	CaseSensitive            bool           `schema:"description=Whether comparison is case-sensitive,default=true"` // Whether comparison is case-sensitive (default: true)
	CheckTargetInconsistency bool           `schema:"description=Flag when the same source has different translations,default=true"` // Same source, different target (default: true)
	CheckSourceInconsistency bool           `schema:"description=Flag when different sources share the same translation"` // Different source, same target (default: false)
}

// ToolName returns the tool name this config applies to.
func (c *InconsistencyCheckConfig) ToolName() string { return "inconsistency-check" }

// Reset restores default values.
func (c *InconsistencyCheckConfig) Reset() {
	c.TargetLocale = ""
	c.CaseSensitive = true
	c.CheckTargetInconsistency = true
	c.CheckSourceInconsistency = false
}

// Validate checks configuration validity.
func (c *InconsistencyCheckConfig) Validate() error {
	if c.TargetLocale.IsEmpty() {
		return fmt.Errorf("inconsistency-check: TargetLocale is required")
	}
	return nil
}

// NewInconsistencyCheckConfig creates an InconsistencyCheckConfig with default settings.
func NewInconsistencyCheckConfig(targetLocale model.LocaleID) *InconsistencyCheckConfig {
	return &InconsistencyCheckConfig{
		TargetLocale:             targetLocale,
		CaseSensitive:            true,
		CheckTargetInconsistency: true,
		CheckSourceInconsistency: false,
	}
}

// NewInconsistencyCheckTool creates a stateful tool that tracks source-target mappings
// across all blocks and flags inconsistencies where the same source has different
// translations or different sources share the same translation.
func NewInconsistencyCheckTool(cfg *InconsistencyCheckConfig) *tool.BaseTool {
	// Stateful maps captured by the closure.
	sourceToTargets := make(map[string]map[string]bool)
	targetToSources := make(map[string]map[string]bool)

	t := &tool.BaseTool{
		ToolName:        "inconsistency-check",
		ToolDescription: "Checks for translation inconsistencies across blocks",
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

		conf := t.Cfg.(*InconsistencyCheckConfig)

		if !block.HasTarget(conf.TargetLocale) {
			return part, nil
		}

		if block.Properties == nil {
			block.Properties = make(map[string]string)
		}

		sourceText := strings.TrimSpace(block.SourceText())
		targetText := strings.TrimSpace(block.TargetText(conf.TargetLocale))

		// Normalize for comparison.
		normSource := sourceText
		normTarget := targetText
		if !conf.CaseSensitive {
			normSource = strings.ToLower(normSource)
			normTarget = strings.ToLower(normTarget)
		}

		// Record source -> target mapping.
		if sourceToTargets[normSource] == nil {
			sourceToTargets[normSource] = make(map[string]bool)
		}
		sourceToTargets[normSource][normTarget] = true

		// Record target -> source mapping (only if needed).
		if conf.CheckSourceInconsistency {
			if targetToSources[normTarget] == nil {
				targetToSources[normTarget] = make(map[string]bool)
			}
			targetToSources[normTarget][normSource] = true
		}

		// Check for target inconsistency: same source, different targets.
		if conf.CheckTargetInconsistency && len(sourceToTargets[normSource]) > 1 {
			block.Properties[PropInconsistencyStatus] = "inconsistent"
			block.Properties[PropInconsistencyType] = "target-inconsistency"
			alternatives := alternativesExcluding(sourceToTargets[normSource], normTarget)
			data, _ := json.Marshal(alternatives)
			block.Properties[PropInconsistencyDetails] = string(data)
			return part, nil
		}

		// Check for source inconsistency: different sources, same target.
		if conf.CheckSourceInconsistency && len(targetToSources[normTarget]) > 1 {
			block.Properties[PropInconsistencyStatus] = "inconsistent"
			block.Properties[PropInconsistencyType] = "source-inconsistency"
			alternatives := alternativesExcluding(targetToSources[normTarget], normSource)
			data, _ := json.Marshal(alternatives)
			block.Properties[PropInconsistencyDetails] = string(data)
			return part, nil
		}

		block.Properties[PropInconsistencyStatus] = "consistent"
		return part, nil
	}
	return t
}

// alternativesExcluding returns all keys in the set except the excluded one.
func alternativesExcluding(set map[string]bool, exclude string) []string {
	var result []string
	for k := range set {
		if k != exclude {
			result = append(result, k)
		}
	}
	return result
}
