// Package tools provides pipeline tools for machine translation.
package tools

import (
	"context"
	"fmt"

	"github.com/gokapi/gokapi/model"
	"github.com/gokapi/gokapi/tool"
	"github.com/gokapi/gokapi/mt/provider"
)

// MTTranslateTool translates Blocks using an MT provider.
type MTTranslateTool struct {
	tool.BaseTool
	provider     provider.MTProvider
	sourceLocale model.LocaleID
	targetLocale model.LocaleID
}

// MTTranslateConfig holds configuration for the MT translate tool.
type MTTranslateConfig struct {
	SourceLocale model.LocaleID
	TargetLocale model.LocaleID
}

// NewMTTranslateTool creates a new MT translation tool.
func NewMTTranslateTool(p provider.MTProvider, cfg MTTranslateConfig) *MTTranslateTool {
	t := &MTTranslateTool{
		provider:     p,
		sourceLocale: cfg.SourceLocale,
		targetLocale: cfg.TargetLocale,
	}
	t.ToolName = fmt.Sprintf("%s-translate", p.Name())
	t.ToolDescription = fmt.Sprintf("Translates Blocks using %s", p.Name())
	t.HandleBlockFn = t.handleBlock
	return t
}

func (t *MTTranslateTool) handleBlock(part *model.Part) (*model.Part, error) {
	block, ok := part.Resource.(*model.Block)
	if !ok {
		return part, nil
	}

	if !block.Translatable {
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
	})
	if err != nil {
		return nil, fmt.Errorf("%s-translate: %w", t.provider.Name(), err)
	}

	block.SetTargetText(t.targetLocale, resp.Translation)
	return part, nil
}
