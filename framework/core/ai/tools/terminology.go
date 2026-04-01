package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/providers/ai"
)

// AITerminologyTool extracts terminology from Blocks using an LLM.
type AITerminologyTool struct {
	tool.BaseTool
	usageAccumulator
	provider provider.LLMProvider
	locale   model.LocaleID
	domain   string // e.g., "medical", "legal", "technology"
}

// AITerminologyConfig holds configuration for the terminology tool.
type AITerminologyConfig struct {
	Locale model.LocaleID `schema:"description=Locale of the source content"`
	Domain string         `schema:"description=Subject domain for terminology extraction (e.g. medical legal technology)"`
}

// NewAITerminologyTool creates a new AI terminology extraction tool.
func NewAITerminologyTool(p provider.LLMProvider, cfg AITerminologyConfig) *AITerminologyTool {
	t := &AITerminologyTool{
		provider: p,
		locale:   cfg.Locale,
		domain:   cfg.Domain,
	}
	t.ToolName = "ai-terminology"
	t.ToolDescription = "Extracts terminology from Blocks using AI/LLM"
	t.HandleBlockFn = t.handleBlock
	return t
}

// TermEntry represents an extracted terminology entry.
type TermEntry struct {
	Term       string `json:"term"`
	Definition string `json:"definition,omitempty"`
	Domain     string `json:"domain,omitempty"`
}

// terminologySchema returns a JSON schema for structured terminology extraction output.
func terminologySchema() provider.JSONSchema {
	return provider.JSONSchema{
		Name:        "terminology_extraction",
		Description: "Extracted terminology entries from text",
		Strict:      true,
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"terms": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"term":       map[string]any{"type": "string"},
							"definition": map[string]any{"type": "string"},
							"domain":     map[string]any{"type": "string"},
						},
						"required":             []string{"term", "definition", "domain"},
						"additionalProperties": false,
					},
				},
			},
			"required":             []string{"terms"},
			"additionalProperties": false,
		},
	}
}

// terminologyResult is the JSON structure returned by structured terminology extraction.
type terminologyResult struct {
	Terms []TermEntry `json:"terms"`
}

func (t *AITerminologyTool) handleBlock(part *model.Part) (*model.Part, error) {
	block, ok := part.Resource.(*model.Block)
	if !ok {
		return part, nil
	}

	sourceText := block.SourceText()
	if strings.TrimSpace(sourceText) == "" {
		return part, nil
	}

	domainHint := ""
	if t.domain != "" {
		domainHint = fmt.Sprintf(" in the %s domain", t.domain)
	}

	prompt := fmt.Sprintf(
		"Extract key terminology%s from the following %s text. "+
			"Return notable terms, or an empty array if none found.\n\nText: %s",
		domainHint, t.locale, sourceText,
	)

	resp, err := t.provider.ChatStructured(context.Background(), []provider.Message{
		{Role: "user", Content: prompt},
	}, terminologySchema())
	if err != nil {
		return nil, fmt.Errorf("ai-terminology: %w", err)
	}
	t.addUsage(resp.Usage)

	var result terminologyResult
	if err := json.Unmarshal([]byte(resp.Content), &result); err != nil {
		result.Terms = nil
	}

	if block.Properties == nil {
		block.Properties = make(map[string]string)
	}

	if len(result.Terms) > 0 {
		termsJSON, _ := json.Marshal(result.Terms)
		block.Properties["terminology"] = string(termsJSON)
	}

	return part, nil
}
