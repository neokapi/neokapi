// Package tools provides pipeline tools for machine translation.
package tools

import (
	"context"
	"fmt"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/providers/mt"
)

// MTTranslateTool translates Blocks using an MT provider.
type MTTranslateTool struct {
	tool.BaseTool
	provider     provider.MTProvider
	sourceLocale model.LocaleID
	targetLocale model.LocaleID
	vocab        *model.VocabularyRegistry
}

// MTTranslateConfig holds configuration for the MT translate tool.
type MTTranslateConfig struct {
	SourceLocale model.LocaleID
	TargetLocale model.LocaleID
}

// NewMTTranslateTool creates a new MT translation tool.
func NewMTTranslateTool(p provider.MTProvider, cfg MTTranslateConfig) *MTTranslateTool {
	vocab := model.NewVocabularyRegistry()
	_ = vocab.LoadDefaults()

	t := &MTTranslateTool{
		provider:     p,
		sourceLocale: cfg.SourceLocale,
		targetLocale: cfg.TargetLocale,
		vocab:        vocab,
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

	// Check if the source fragment has inline spans.
	frag := block.FirstFragment()
	if frag != nil && frag.HasSpans() {
		return t.handleBlockWithSpans(part, block, frag)
	}

	// Plain text translation.
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

// handleBlockWithSpans translates a block that contains inline spans.
// Uses SemanticHTML for MT APIs that handle HTML tags natively.
func (t *MTTranslateTool) handleBlockWithSpans(part *model.Part, block *model.Block, frag *model.Fragment) (*model.Part, error) {
	// Render as semantic HTML — MT APIs preserve HTML tags natively.
	sourceHTML := frag.SemanticHTML(t.vocab)

	resp, err := t.provider.Translate(context.Background(), provider.TranslateRequest{
		Source:       sourceHTML,
		SourceLocale: t.sourceLocale,
		TargetLocale: t.targetLocale,
	})
	if err != nil {
		return nil, fmt.Errorf("%s-translate: %w", t.provider.Name(), err)
	}

	// Reconstruct Fragment from the HTML response.
	targetFrag := model.ParseSemanticHTML(resp.Translation, frag.Spans, t.vocab)
	block.SetTargetFragment(t.targetLocale, targetFrag)
	return part, nil
}
