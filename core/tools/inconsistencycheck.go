package tools

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
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
	TargetLocale             model.LocaleID `json:"targetLocale,omitempty"             schema:"-"`
	CaseSensitive            bool           `json:"caseSensitive,omitempty"            schema:"title=Case Sensitive,description=Whether comparison is case-sensitive,default=true"`
	CheckTargetInconsistency bool           `json:"checkTargetInconsistency,omitempty" schema:"title=Check Target Inconsistency,description=Flag when the same source has different translations,default=true"`
	CheckSourceInconsistency bool           `json:"checkSourceInconsistency,omitempty" schema:"title=Check Source Inconsistency,description=Flag when different sources share the same translation"`
	CheckPerFile             bool           `json:"checkPerFile,omitempty"             schema:"title=Per-File Checking,description=Process each input file individually instead of comparing segments across all documents"`
	DisplayOption            string         `json:"displayOption,omitempty"            schema:"title=Inline Code Display,description=Controls how inline codes are represented: original codes or generic markers or plain text,enum=original|generic|plain,default=generic"`
}

// ToolName returns the tool name this config applies to.
func (c *InconsistencyCheckConfig) ToolName() string { return "inconsistency-check" }

// Reset restores default values.
func (c *InconsistencyCheckConfig) Reset() {
	c.TargetLocale = ""
	c.CaseSensitive = true
	c.CheckTargetInconsistency = true
	c.CheckSourceInconsistency = false
	c.CheckPerFile = false
	c.DisplayOption = "generic"
}

// Validate checks configuration validity.
func (c *InconsistencyCheckConfig) Validate() error {
	if c.TargetLocale.IsEmpty() {
		return errors.New("inconsistency-check: TargetLocale is required")
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
		CheckPerFile:             false,
		DisplayOption:            "generic",
	}
}

// InconsistencyCheckSchema returns the auto-generated schema for the inconsistency-check tool.
func InconsistencyCheckSchema() *schema.ComponentSchema {
	return schema.FromStruct(NewInconsistencyCheckConfig(""), schema.ToolMeta{
		ID:          "inconsistency-check",
		Category:    schema.CategoryQuality,
		DisplayName: "Inconsistency Check",
		Description: "Detect inconsistent translations of identical source strings",
		Requires:    []string{schema.RequiresTargetLanguage},
	})
}

// NewInconsistencyCheckFromConfig creates an inconsistency-check tool from a config map.
func NewInconsistencyCheckFromConfig(config map[string]any, targetLang string) (tool.Tool, error) {
	cfg := NewInconsistencyCheckConfig(model.LocaleID(targetLang))
	if err := schema.ApplyConfig(config, cfg); err != nil {
		return nil, fmt.Errorf("inconsistency-check config: %w", err)
	}
	if targetLang != "" {
		cfg.TargetLocale = model.LocaleID(targetLang)
	}
	return NewInconsistencyCheckTool(cfg), nil
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
	// When checking per file, reset state maps at the start of each document layer.
	t.HandleLayerStartFn = func(part *model.Part) (*model.Part, error) {
		conf := t.Cfg.(*InconsistencyCheckConfig)
		if conf.CheckPerFile {
			for k := range sourceToTargets {
				delete(sourceToTargets, k)
			}
			for k := range targetToSources {
				delete(targetToSources, k)
			}
		}
		return part, nil
	}
	t.Annotate = func(v tool.BlockView) error {
		if !v.Translatable() {
			return nil
		}

		conf := t.Cfg.(*InconsistencyCheckConfig)

		if !v.HasTarget(conf.TargetLocale) {
			return nil
		}

		sourceText := strings.TrimSpace(v.SourceText())
		targetText := strings.TrimSpace(v.TargetText(conf.TargetLocale))

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
			v.SetProperty(PropInconsistencyStatus, "inconsistent")
			v.SetProperty(PropInconsistencyType, "target-inconsistency")
			alternatives := alternativesExcluding(sourceToTargets[normSource], normTarget)
			data, _ := json.Marshal(alternatives)
			v.SetProperty(PropInconsistencyDetails, string(data))
			check.Annotate(v, "inconsistency-check", []check.Finding{{
				Category: "inconsistency",
				Severity: check.SeverityMajor,
				Message:  "Source has more than one translation; also seen as: " + strings.Join(alternatives, ", "),
			}})
			return nil
		}

		// Check for source inconsistency: different sources, same target.
		if conf.CheckSourceInconsistency && len(targetToSources[normTarget]) > 1 {
			v.SetProperty(PropInconsistencyStatus, "inconsistent")
			v.SetProperty(PropInconsistencyType, "source-inconsistency")
			alternatives := alternativesExcluding(targetToSources[normTarget], normSource)
			data, _ := json.Marshal(alternatives)
			v.SetProperty(PropInconsistencyDetails, string(data))
			check.Annotate(v, "inconsistency-check", []check.Finding{{
				Category: "inconsistency",
				Severity: check.SeverityMajor,
				Message:  "Different sources share this translation; also from: " + strings.Join(alternatives, ", "),
			}})
			return nil
		}

		v.SetProperty(PropInconsistencyStatus, "consistent")
		return nil
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
