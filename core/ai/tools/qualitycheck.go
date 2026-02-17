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
		`Analyze the following translation for quality issues. Check for: %s.

Source (%s): %s
Translation (%s): %s

Respond in JSON format with an array of issues:
[{"type": "<check-type>", "severity": "<error|warning|info>", "description": "<issue>", "suggestion": "<fix>"}]
If no issues found, return an empty array: []`,
		strings.Join(t.checks, ", "),
		t.sourceLocale, sourceText,
		t.targetLocale, targetText,
	)

	resp, err := t.provider.Chat(context.Background(), []provider.Message{
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return nil, fmt.Errorf("ai-qa: %w", err)
	}

	var issues []provider.QAIssue
	content := strings.TrimSpace(resp.Content)
	if err := json.Unmarshal([]byte(content), &issues); err != nil {
		// If the response isn't valid JSON, wrap it as a single info issue
		issues = []provider.QAIssue{{
			Type:        "parse-error",
			Severity:    "info",
			Description: content,
		}}
	}

	if block.Properties == nil {
		block.Properties = make(map[string]string)
	}

	issuesJSON, _ := json.Marshal(issues)
	block.Properties["qa-issues"] = string(issuesJSON)
	block.Properties["qa-provider"] = t.provider.Name()
	block.Properties["qa-checks"] = strings.Join(t.checks, ",")

	return part, nil
}
