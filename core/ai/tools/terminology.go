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

// AITerminologyTool extracts terminology from Blocks using an LLM.
type AITerminologyTool struct {
	tool.BaseTool
	usageAccumulator
	provider aiprovider.LLMProvider
	locale   model.LocaleID
	domain   string // e.g., "medical", "legal", "technology"
}

// AITerminologyConfig holds configuration for the terminology tool.
type AITerminologyConfig struct {
	Locale   model.LocaleID `json:"locale,omitempty"   schema:"-"`
	Domain   string         `json:"domain,omitempty"   schema:"title=Domain,description=Subject domain for terminology extraction (e.g. medical legal technology)"`
	Provider string         `json:"provider,omitempty" schema:"title=AI Provider,description=AI provider,default=anthropic,group=provider"`
	APIKey   string         `json:"apiKey,omitempty"   schema:"title=API Key,description=API key for the AI provider,group=provider"`
	Model    string         `json:"model,omitempty"    schema:"title=Model,description=AI model name,group=provider"`
}

// AITerminologySchema returns the auto-generated schema for the tool.
func AITerminologySchema() *schema.ComponentSchema {
	s := schema.FromStruct(&AITerminologyConfig{}, schema.ToolMeta{
		ID:                    "ai-terminology",
		Category:              schema.CategoryAnalysis,
		DisplayName:           "AI Terminology Extraction",
		Description:           "Extract candidate terminology from content using an LLM provider",
		Inputs:                []string{schema.PartTypeBlock},
		Tags:                  []string{"ai-powered", "terminology"},
		DefaultParallelBlocks: 5,
		Requires:              []string{schema.RequiresCredentials},
		Cardinality:           schema.Monolingual,
		Produces:              []schema.AnnotationType{schema.AnnotationTerms},
		SideEffects:           []schema.SideEffect{schema.SideEffectAPICall},
	})
	injectProviderOptions(s)
	return s
}

// NewAITerminologyFromConfig creates an AI terminology tool from a config map.
func NewAITerminologyFromConfig(config map[string]any, _ string) (tool.Tool, error) {
	var cfg AITerminologyConfig
	if err := schema.ApplyConfig(config, &cfg); err != nil {
		return nil, fmt.Errorf("ai-terminology config: %w", err)
	}
	p, err := ProviderFromConfig(cfg.Provider, aiprovider.Config{APIKey: cfg.APIKey, Model: cfg.Model})
	if err != nil {
		return nil, err
	}
	return NewAITerminologyTool(p, cfg), nil
}

// NewAITerminologyTool creates a new AI terminology extraction tool.
func NewAITerminologyTool(p aiprovider.LLMProvider, cfg AITerminologyConfig) *AITerminologyTool {
	t := &AITerminologyTool{
		provider: p,
		locale:   cfg.Locale,
		domain:   cfg.Domain,
	}
	t.ToolName = "ai-terminology"
	t.ToolDescription = "Extracts terminology from Blocks using AI/LLM"
	t.Annotate = t.annotate
	return t
}

// TermEntry represents an extracted terminology entry.
type TermEntry struct {
	Term       string `json:"term"`
	Definition string `json:"definition,omitempty"`
	Domain     string `json:"domain,omitempty"`
}

// terminologySchema returns a JSON schema for structured terminology extraction output.
func terminologySchema() aiprovider.JSONSchema {
	return aiprovider.JSONSchema{
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

func (t *AITerminologyTool) annotate(v tool.BlockView) error {
	sourceText := v.SourceText()
	if strings.TrimSpace(sourceText) == "" {
		return nil
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

	resp, err := t.provider.ChatStructured(context.Background(), []aiprovider.Message{
		{Role: "user", Content: prompt},
	}, terminologySchema())
	if err != nil {
		return fmt.Errorf("ai-terminology: %w", err)
	}
	t.addUsage(resp.Usage)

	var result terminologyResult
	if err := json.Unmarshal([]byte(resp.Content), &result); err != nil {
		result.Terms = nil
	}

	if len(result.Terms) > 0 {
		termsJSON, _ := json.Marshal(result.Terms)
		v.SetProperty("terminology", string(termsJSON))
	}

	return nil
}
