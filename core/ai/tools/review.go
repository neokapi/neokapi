package tools

import (
	"context"
	"fmt"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/providers/ai"
)

// AIReviewTool reviews translations with explanations using an LLM.
type AIReviewTool struct {
	tool.BaseTool
	usageAccumulator
	provider     aiprovider.LLMProvider
	sourceLocale model.LocaleID
	targetLocale model.LocaleID
}

// AIReviewConfig holds configuration for the review tool.
type AIReviewConfig struct {
	SourceLocale model.LocaleID `json:"sourceLocale,omitempty" schema:"-"`
	TargetLocale model.LocaleID `json:"targetLocale,omitempty" schema:"-"`
	Provider     string         `json:"provider,omitempty"     schema:"title=AI Provider,description=AI provider,default=anthropic,group=provider"`
	APIKey       string         `json:"apiKey,omitempty"       schema:"title=API Key,description=API key for the AI provider,group=provider"`
	Model        string         `json:"model,omitempty"        schema:"title=Model,description=AI model name,group=provider"`
}

// AIReviewSchema returns the auto-generated schema for the AI review tool.
func AIReviewSchema() *schema.ComponentSchema {
	s := schema.FromStruct(&AIReviewConfig{}, schema.ToolMeta{
		ID:                    "ai-review",
		Category:              schema.CategoryQuality,
		DisplayName:           "AI Review",
		Description:           "Review translations with scoring using an LLM provider",
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

// NewAIReviewFromConfig creates an AI review tool from a config map.
func NewAIReviewFromConfig(config map[string]any, targetLang string) (tool.Tool, error) {
	var cfg AIReviewConfig
	if err := schema.ApplyConfig(config, &cfg); err != nil {
		return nil, fmt.Errorf("ai-review config: %w", err)
	}
	if targetLang != "" {
		cfg.TargetLocale = model.LocaleID(targetLang)
	}
	p, err := ProviderFromConfig(cfg.Provider, aiprovider.Config{APIKey: cfg.APIKey, Model: cfg.Model})
	if err != nil {
		return nil, err
	}
	return NewAIReviewTool(p, cfg), nil
}

// NewAIReviewTool creates a new AI translation review tool.
func NewAIReviewTool(p aiprovider.LLMProvider, cfg AIReviewConfig) *AIReviewTool {
	t := &AIReviewTool{
		provider:     p,
		sourceLocale: cfg.SourceLocale,
		targetLocale: cfg.TargetLocale,
	}
	t.ToolName = "ai-review"
	t.ToolDescription = "Reviews translations with explanations using AI/LLM"
	t.HandleBlockFn = t.handleBlock
	return t
}

func (t *AIReviewTool) handleBlock(part *model.Part) (*model.Part, error) {
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
		`Review the following translation. Provide a brief assessment of accuracy, fluency, and any suggested improvements. Be concise.

Source (%s): %s
Translation (%s): %s

Format your response as:
Score: <1-10>
Assessment: <brief assessment>
Suggestion: <improved translation if needed, or "none">`,
		t.sourceLocale, sourceText,
		t.targetLocale, targetText,
	)

	resp, err := t.provider.Chat(context.Background(), []aiprovider.Message{
		{Role: "user", Content: prompt},
	})
	if err != nil {
		return nil, fmt.Errorf("ai-review: %w", err)
	}
	t.addUsage(resp.Usage)

	if block.Properties == nil {
		block.Properties = make(map[string]string)
	}
	block.Properties["review"] = resp.Content
	block.Properties["review-provider"] = string(t.provider.Name())

	return part, nil
}
