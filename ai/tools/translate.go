package tools

import (
	"context"
	"fmt"

	"github.com/asgeirf/gokapi/ai/provider"
	"github.com/asgeirf/gokapi/core/model"
	"github.com/asgeirf/gokapi/core/tool"
)

// AITranslateTool translates untranslated Blocks using an LLM provider.
type AITranslateTool struct {
	tool.BaseTool
	provider     provider.LLMProvider
	sourceLocale model.LocaleID
	targetLocale model.LocaleID
	glossary     map[string]string
	skipMatched  bool
}

// AITranslateConfig holds configuration for the AI translate tool.
type AITranslateConfig struct {
	SourceLocale model.LocaleID
	TargetLocale model.LocaleID
	Glossary     map[string]string
	SkipMatched  bool
}

// NewAITranslateTool creates a new AI translation tool.
func NewAITranslateTool(p provider.LLMProvider, cfg AITranslateConfig) *AITranslateTool {
	t := &AITranslateTool{
		provider:     p,
		sourceLocale: cfg.SourceLocale,
		targetLocale: cfg.TargetLocale,
		glossary:     cfg.Glossary,
		skipMatched:  cfg.SkipMatched,
	}
	t.ToolName = "ai-translate"
	t.ToolDescription = "Translates Blocks using AI/LLM"
	t.HandleBlockFn = t.handleBlock
	return t
}

func (t *AITranslateTool) handleBlock(part *model.Part) (*model.Part, error) {
	block, ok := part.Resource.(*model.Block)
	if !ok {
		return part, nil
	}

	if !block.Translatable {
		return part, nil
	}

	if t.skipMatched && block.HasTarget(t.targetLocale) {
		return part, nil
	}

	sourceText := block.SourceText()
	if sourceText == "" {
		return part, nil
	}

	resp, err := t.provider.Translate(context.Background(), provider.TranslateRequest{
		Source:       sourceText,
		SourceLocale: t.sourceLocale,
		TargetLocale: t.targetLocale,
		Glossary:     t.glossary,
	})
	if err != nil {
		return nil, fmt.Errorf("ai-translate: %w", err)
	}

	block.SetTargetText(t.targetLocale, resp.Translation)

	if block.Annotations == nil {
		block.Annotations = make(map[string]model.Annotation)
	}
	block.Annotations["alt-translations"] = &model.AltTranslation{
		Target:    model.NewFragment(resp.Translation),
		Locale:    t.targetLocale,
		Origin:    "ai:" + t.provider.Name(),
		Score:     resp.Confidence,
		MatchType: "ai",
	}

	return part, nil
}
