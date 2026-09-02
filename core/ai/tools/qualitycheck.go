package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/ai/prompt"
	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// AICheckTool checks translation quality using an LLM provider.
type AICheckTool struct {
	tool.BaseTool
	usageAccumulator
	provider     aiprovider.LLMProvider
	sourceLocale model.LocaleID
	targetLocale model.LocaleID
	checks       []string // e.g., "terminology", "fluency", "accuracy", "consistency"
}

// AICheckConfig holds configuration for the LLM-judged check tool.
type AICheckConfig struct {
	SourceLocale model.LocaleID `json:"sourceLocale,omitempty" schema:"-"`
	TargetLocale model.LocaleID `json:"targetLocale,omitempty" schema:"-"`
	Provider     string         `json:"provider,omitempty"     schema:"title=AI Provider,description=AI provider,default=anthropic,group=provider"`
	APIKey       string         `json:"apiKey,omitempty"       schema:"title=API Key,description=API key for the AI provider,group=provider"`
	Model        string         `json:"model,omitempty"        schema:"title=Model,description=AI model name,group=provider"`
	Checks       []string       `json:"checks,omitempty"       schema:"title=Quality Checks,description=Quality checks to perform (e.g. terminology fluency accuracy consistency)"`
}

// NewAICheckFromConfig creates an LLM-judged check tool from a config map.
func NewAICheckFromConfig(config map[string]any, targetLang string) (tool.Tool, error) {
	var cfg AICheckConfig
	if err := schema.ApplyConfig(config, &cfg); err != nil {
		return nil, fmt.Errorf("qa config: %w", err)
	}
	if targetLang != "" {
		cfg.TargetLocale = model.LocaleID(targetLang)
	}
	p, err := ProviderFromConfig(cfg.Provider, aiprovider.Config{APIKey: cfg.APIKey, Model: cfg.Model})
	if err != nil {
		return nil, err
	}
	return NewAICheckTool(p, cfg), nil
}

// NewAICheckTool creates a new AI quality check tool.
func NewAICheckTool(p aiprovider.LLMProvider, cfg AICheckConfig) *AICheckTool {
	if len(cfg.Checks) == 0 {
		cfg.Checks = []string{"terminology", "fluency", "accuracy"}
	}
	t := &AICheckTool{
		provider:     p,
		sourceLocale: cfg.SourceLocale,
		targetLocale: cfg.TargetLocale,
		checks:       cfg.Checks,
	}
	t.ToolName = "qa"
	t.ToolDescription = "Checks translation quality using AI/LLM"
	t.Annotate = t.annotate
	return t
}

// aiCheckSchema returns a JSON schema for structured check output.
func aiCheckSchema() aiprovider.JSONSchema {
	return aiprovider.JSONSchema{
		Name:        "qa_check",
		Description: "Quality check results for a translation",
		Strict:      true,
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"issues": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"type":        map[string]any{"type": "string"},
							"severity":    map[string]any{"type": "string", "enum": []string{"error", "warning", "info"}},
							"description": map[string]any{"type": "string"},
							"suggestion":  map[string]any{"type": "string"},
						},
						"required":             []string{"type", "severity", "description", "suggestion"},
						"additionalProperties": false,
					},
				},
			},
			"required":             []string{"issues"},
			"additionalProperties": false,
		},
	}
}

// aiCheckResult is the JSON structure returned by the structured check.
type aiCheckResult struct {
	Issues []aiprovider.CheckIssue `json:"issues"`
}

func (t *AICheckTool) annotate(v tool.BlockView) error {
	if !v.HasTarget(t.targetLocale) {
		return nil
	}

	sourceText := v.SourceText()
	targetText := v.TargetText(t.targetLocale)

	turns := prompt.QualityCheck{
		SourceLocale: t.sourceLocale,
		TargetLocale: t.targetLocale,
		Checks:       t.checks,
	}.Turns(sourceText, targetText)

	ctx := prompt.WithID(v.Context(), prompt.IDQualityCheck)
	resp, err := t.provider.ChatStructured(ctx, aiprovider.MessagesFromTurns(turns), aiCheckSchema())
	if err != nil {
		return fmt.Errorf("qa: %w", err)
	}
	t.addUsage(resp.Usage)

	var result aiCheckResult
	if err := json.Unmarshal([]byte(resp.Content), &result); err != nil {
		result.Issues = []aiprovider.CheckIssue{{
			Type:        "parse-error",
			Severity:    "info",
			Description: resp.Content,
		}}
	}

	// Map the model's structured output onto the unified core/check model so
	// the LLM judge feeds the same findings/score pipeline as the deterministic
	// checkers. aiprovider.CheckIssue stays the structured-output wire type; the
	// findings are what every consumer reads.
	findings := make([]check.Finding, 0, len(result.Issues))
	for _, iss := range result.Issues {
		findings = append(findings, check.Finding{
			Category:   iss.Type,
			Severity:   aiCheckSeverity(iss.Severity),
			Message:    iss.Description,
			Suggestion: iss.Suggestion,
		})
	}
	check.Annotate(v, "qa", findings)
	v.SetProperty("qa-provider", string(t.provider.Name()))
	v.SetProperty("qa-checks", strings.Join(t.checks, ","))

	return nil
}

// aiCheckSeverity maps the LLM structured-output severity ("error"/"warning"/
// "info") onto the unified core/check severity scale.
func aiCheckSeverity(s string) check.Severity {
	switch s {
	case "error":
		return check.SeverityMajor
	case "warning":
		return check.SeverityMinor
	default: // "info" and any unrecognized value carry no penalty.
		return check.SeverityNeutral
	}
}
