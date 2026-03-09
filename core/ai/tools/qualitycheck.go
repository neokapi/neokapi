package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gokapi/gokapi/core/ai/provider"
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/tool"
)

// AIQACheckTool checks translation quality using an LLM provider.
type AIQACheckTool struct {
	tool.BaseTool
	provider     provider.LLMProvider
	sourceLocale model.LocaleID
	targetLocale model.LocaleID
	checks       []string // e.g., "terminology", "fluency", "accuracy", "consistency"
}

// AIQAConfig holds configuration for the QA check tool.
type AIQAConfig struct {
	SourceLocale model.LocaleID
	TargetLocale model.LocaleID
	Checks       []string
}

// NewAIQACheckTool creates a new AI quality check tool.
func NewAIQACheckTool(p provider.LLMProvider, cfg AIQAConfig) *AIQACheckTool {
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
	t.HandleBlockFn = t.handleBlock
	return t
}

// qaSchema returns a JSON schema for structured QA check output.
func qaSchema() provider.JSONSchema {
	return provider.JSONSchema{
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
	Issues []provider.QAIssue `json:"issues"`
}

func (t *AIQACheckTool) handleBlock(part *model.Part) (*model.Part, error) {
	block, ok := part.Resource.(*model.Block)
	if !ok {
		return part, nil
	}

	if !block.HasTarget(t.targetLocale) {
		return part, nil
	}

	sourceText := block.SourceText()
	targetText := block.TargetText(t.targetLocale)

	prompt := fmt.Sprintf(
		"Analyze the following translation for quality issues. Check for: %s.\n\n"+
			"Source (%s): %s\nTranslation (%s): %s\n\n"+
			"Return all issues found, or an empty array if none.",
		strings.Join(t.checks, ", "),
		t.sourceLocale, sourceText,
		t.targetLocale, targetText,
	)

	resp, err := t.provider.ChatStructured(context.Background(), []provider.Message{
		{Role: "user", Content: prompt},
	}, qaSchema())
	if err != nil {
		return nil, fmt.Errorf("ai-qa: %w", err)
	}

	var result qaResult
	if err := json.Unmarshal([]byte(resp.Content), &result); err != nil {
		result.Issues = []provider.QAIssue{{
			Type:        "parse-error",
			Severity:    "info",
			Description: resp.Content,
		}}
	}

	if block.Properties == nil {
		block.Properties = make(map[string]string)
	}

	issuesJSON, _ := json.Marshal(result.Issues)
	block.Properties["qa-issues"] = string(issuesJSON)
	block.Properties["qa-provider"] = t.provider.Name()
	block.Properties["qa-checks"] = strings.Join(t.checks, ",")

	return part, nil
}
