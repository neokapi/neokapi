package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/providers/ai"
)

// AIQACheckTool checks translation quality using an LLM provider.
type AIQACheckTool struct {
	tool.BaseTool
	usageAccumulator
	provider     aiprovider.LLMProvider
	sourceLocale model.LocaleID
	targetLocale model.LocaleID
	checks       []string // e.g., "terminology", "fluency", "accuracy", "consistency"
}

// AIQAConfig holds configuration for the QA check tool.
type AIQAConfig struct {
	SourceLocale model.LocaleID `json:"sourceLocale,omitempty" schema:"-"`
	TargetLocale model.LocaleID `json:"targetLocale,omitempty" schema:"-"`
	Provider     string         `json:"provider,omitempty"     schema:"title=AI Provider,description=AI provider,default=anthropic,group=provider"`
	APIKey       string         `json:"apiKey,omitempty"       schema:"title=API Key,description=API key for the AI provider,group=provider"`
	Model        string         `json:"model,omitempty"        schema:"title=Model,description=AI model name,group=provider"`
	Checks       []string       `json:"checks,omitempty"       schema:"title=Quality Checks,description=Quality checks to perform (e.g. terminology fluency accuracy consistency)"`
}

// AIQASchema returns the auto-generated schema for the AI QA tool.
func AIQASchema() *schema.ComponentSchema {
	s := schema.FromStruct(&AIQAConfig{}, schema.ToolMeta{
		ID:                    "ai-qa",
		Category:              schema.CategoryQuality,
		DisplayName:           "AI QA Check",
		Description:           "Check translation quality using an LLM provider",
		Inputs:                []string{schema.PartTypeBlock},
		Tags:                  []string{"ai-powered"},
		WritesOutput:          true,
		DefaultParallelBlocks: 5,
		Requires:              []string{schema.RequiresTargetLanguage, schema.RequiresCredentials},
		Cardinality:           schema.Bilingual,
		Produces:              []schema.AnnotationType{schema.AnnotationQAIssues},
		SideEffects:           []schema.SideEffect{schema.SideEffectAPICall},
	})
	injectProviderOptions(s)
	return s
}

// NewAIQAFromConfig creates an AI QA tool from a config map.
func NewAIQAFromConfig(config map[string]any, targetLang string) (tool.Tool, error) {
	var cfg AIQAConfig
	if err := schema.ApplyConfig(config, &cfg); err != nil {
		return nil, fmt.Errorf("ai-qa config: %w", err)
	}
	if targetLang != "" {
		cfg.TargetLocale = model.LocaleID(targetLang)
	}
	p, err := ProviderFromConfig(cfg.Provider, aiprovider.Config{APIKey: cfg.APIKey, Model: cfg.Model})
	if err != nil {
		return nil, err
	}
	return NewAIQACheckTool(p, cfg), nil
}

// NewAIQACheckTool creates a new AI quality check tool.
func NewAIQACheckTool(p aiprovider.LLMProvider, cfg AIQAConfig) *AIQACheckTool {
	if len(cfg.Checks) == 0 {
		cfg.Checks = []string{"terminology", "fluency", "accuracy"}
	}
	t := &AIQACheckTool{
		provider:     p,
		sourceLocale: cfg.SourceLocale,
		targetLocale: cfg.TargetLocale,
		checks:       cfg.Checks,
	}
	t.ToolName = "ai-qa"
	t.ToolDescription = "Checks translation quality using AI/LLM"
	t.Annotate = t.annotate
	return t
}

// qaSchema returns a JSON schema for structured QA check output.
func qaSchema() aiprovider.JSONSchema {
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

// qaResult is the JSON structure returned by structured QA check.
type qaResult struct {
	Issues []aiprovider.QAIssue `json:"issues"`
}

func (t *AIQACheckTool) annotate(v tool.BlockView) error {
	if !v.HasTarget(t.targetLocale) {
		return nil
	}

	sourceText := v.SourceText()
	targetText := v.TargetText(t.targetLocale)

	prompt := fmt.Sprintf(
		"Analyze the following translation for quality issues. Check for: %s.\n\n"+
			"Source (%s): %s\nTranslation (%s): %s\n\n"+
			"Return all issues found, or an empty array if none.",
		strings.Join(t.checks, ", "),
		t.sourceLocale, sourceText,
		t.targetLocale, targetText,
	)

	resp, err := t.provider.ChatStructured(context.Background(), []aiprovider.Message{
		{Role: "user", Content: prompt},
	}, qaSchema())
	if err != nil {
		return fmt.Errorf("ai-qa: %w", err)
	}
	t.addUsage(resp.Usage)

	var result qaResult
	if err := json.Unmarshal([]byte(resp.Content), &result); err != nil {
		result.Issues = []aiprovider.QAIssue{{
			Type:        "parse-error",
			Severity:    "info",
			Description: resp.Content,
		}}
	}

	issuesJSON, _ := json.Marshal(result.Issues)
	v.SetProperty("qa-issues", string(issuesJSON))
	v.SetProperty("qa-provider", string(t.provider.Name()))
	v.SetProperty("qa-checks", strings.Join(t.checks, ","))

	return nil
}
