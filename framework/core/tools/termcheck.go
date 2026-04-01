package tools

import (
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/model"
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
	Glossary      []GlossaryEntry `schema:"description=Glossary entries mapping source terms to required target translations"`
	TargetLocale  model.LocaleID  `schema:"description=Target locale for processing"`
	CaseSensitive bool            `schema:"description=Whether term matching is case-sensitive"`
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
