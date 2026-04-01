package tools

import (
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
)

// Terminology check property keys.
const (
	PropTermCheckPassed = "term-check-passed"
	PropTermCheckErrors = "term-check-errors"
)

// GlossaryEntry defines a source term and its required target translation.
type GlossaryEntry struct {
	Source string // Source language term
	Target string // Required target translation
}

// TermCheckConfig holds configuration for the terminology check tool.
type TermCheckConfig struct {
	Glossary      []GlossaryEntry `json:"glossary,omitempty"      schema:"-"`
	TargetLocale  model.LocaleID  `json:"targetLocale,omitempty"  schema:"-"`
	CaseSensitive bool            `json:"caseSensitive,omitempty" schema:"description=Whether term matching is case-sensitive"`
}

// ToolName returns the tool name this config applies to.
func (c *TermCheckConfig) ToolName() string { return "term-check" }

// Reset restores default values.
func (c *TermCheckConfig) Reset() {
	c.Glossary = nil
	c.TargetLocale = ""
	c.CaseSensitive = false
}

// Validate checks configuration validity.
func (c *TermCheckConfig) Validate() error {
	if c.TargetLocale.IsEmpty() {
		return fmt.Errorf("term-check: TargetLocale is required")
	}
	for i, entry := range c.Glossary {
		if entry.Source == "" {
			return fmt.Errorf("term-check: glossary entry %d has empty source", i)
		}
		if entry.Target == "" {
			return fmt.Errorf("term-check: glossary entry %d has empty target", i)
		}
	}
	return nil
}

// TermCheckSchema returns the auto-generated schema for the term-check tool.
func TermCheckSchema() *schema.ComponentSchema {
	return schema.FromStruct(&TermCheckConfig{}, schema.ToolMeta{
		ID:          "term-check",
		Category:    schema.CategoryValidate,
		DisplayName: "Term Check",
		Description: "Check terminology consistency across content",
		Inputs:      []string{schema.PartTypeBlock},
		Requires:    []string{schema.RequiresTargetLanguage},
	})
}

// NewTermCheckFromConfig creates a term-check tool from a config map.
func NewTermCheckFromConfig(config map[string]any, targetLang string) (tool.Tool, error) {
	var cfg TermCheckConfig
	if err := schema.ApplyConfig(config, &cfg); err != nil {
		return nil, fmt.Errorf("term-check config: %w", err)
	}
	if targetLang != "" {
		cfg.TargetLocale = model.LocaleID(targetLang)
	}
	return NewTermCheckTool(&cfg), nil
}

// NewTermCheckTool creates a tool that verifies terminology usage in translations.
// For each glossary entry, if the source term appears in the source text,
// the tool checks that the required target term appears in the target text.
func NewTermCheckTool(cfg *TermCheckConfig) *tool.BaseTool {
	t := &tool.BaseTool{
		ToolName:        "term-check",
		ToolDescription: "Verifies terminology usage in translations against a glossary",
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

		conf := t.Cfg.(*TermCheckConfig)
		if len(conf.Glossary) == 0 {
			return part, nil
		}

		if !block.HasTarget(conf.TargetLocale) {
			return part, nil
		}

		if block.Properties == nil {
			block.Properties = make(map[string]string)
		}

		sourceText := block.SourceText()
		targetText := block.TargetText(conf.TargetLocale)

		var errors []string
		for _, entry := range conf.Glossary {
			srcContains := containsTerm(sourceText, entry.Source, conf.CaseSensitive)
			if !srcContains {
				continue
			}
			tgtContains := containsTerm(targetText, entry.Target, conf.CaseSensitive)
			if !tgtContains {
				errors = append(errors, fmt.Sprintf("term %q found in source but required translation %q missing in target", entry.Source, entry.Target))
			}
		}

		if len(errors) == 0 {
			block.Properties[PropTermCheckPassed] = "true"
		} else {
			block.Properties[PropTermCheckPassed] = "false"
			block.Properties[PropTermCheckErrors] = strings.Join(errors, "; ")
		}

		return part, nil
	}
	return t
}

func containsTerm(text, term string, caseSensitive bool) bool {
	if caseSensitive {
		return strings.Contains(text, term)
	}
	return strings.Contains(strings.ToLower(text), strings.ToLower(term))
}
