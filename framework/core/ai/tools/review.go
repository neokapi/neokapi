package tools

import (
	"context"
	"fmt"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/providers/ai"
)

// AIReviewTool reviews translations with explanations using an LLM.
type AIReviewTool struct {
	tool.BaseTool
	usageAccumulator
	provider     provider.LLMProvider
	sourceLocale model.LocaleID
	targetLocale model.LocaleID
}

// AIReviewConfig holds configuration for the review tool.
type AIReviewConfig struct {
	SourceLocale model.LocaleID `schema:"description=Source locale of the content"`
	TargetLocale model.LocaleID `schema:"description=Target locale for processing"`
}

// NewAIReviewTool creates a new AI translation review tool.
func NewAIReviewTool(p provider.LLMProvider, cfg AIReviewConfig) *AIReviewTool {
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

	resp, err := t.provider.Chat(context.Background(), []provider.Message{
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
	block.Properties["review-provider"] = t.provider.Name()

	return part, nil
}
