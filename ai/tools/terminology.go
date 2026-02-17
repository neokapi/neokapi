package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gokapi/gokapi/ai/provider"
	"github.com/gokapi/gokapi/model"
	"github.com/gokapi/gokapi/tool"
)

// AITerminologyTool extracts terminology from Blocks using an LLM.
type AITerminologyTool struct {
	tool.BaseTool
	provider provider.LLMProvider
	locale   model.LocaleID
	domain   string // e.g., "medical", "legal", "technology"
}

// AITerminologyConfig holds configuration for the terminology tool.
type AITerminologyConfig struct {
	Locale model.LocaleID
	Domain string
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
		`Extract key terminology%s from the following %s text.

Text: %s

Respond in JSON format with an array of term entries:
[{"term": "<term>", "definition": "<brief definition>", "domain": "<domain>"}]
If no notable terms found, return an empty array: []`,
		domainHint, t.locale, sourceText,
	)

	resp, err := t.provider.Chat(context.Background(), []provider.Message{
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return nil, fmt.Errorf("ai-terminology: %w", err)
	}

	var terms []TermEntry
	content := strings.TrimSpace(resp.Content)
	if err := json.Unmarshal([]byte(content), &terms); err != nil {
		terms = nil
	}

	if block.Properties == nil {
		block.Properties = make(map[string]string)
	}

	if len(terms) > 0 {
		termsJSON, _ := json.Marshal(terms)
		block.Properties["terminology"] = string(termsJSON)
	}

	return part, nil
}
