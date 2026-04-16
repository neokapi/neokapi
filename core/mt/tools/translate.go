// Package tools provides pipeline tools for machine translation.
package tools

import (
	"context"
	"fmt"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	mtprovider "github.com/neokapi/neokapi/providers/mt"
)

// MTTranslateTool translates Blocks using an MT provider.
type MTTranslateTool struct {
	tool.BaseTool
	provider     mtprovider.MTProvider
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
func NewMTTranslateTool(p mtprovider.MTProvider, cfg MTTranslateConfig) *MTTranslateTool {
	vocab := model.NewVocabularyRegistry()
	_ = vocab.LoadDefaults()

	t := &MTTranslateTool{
		provider:     p,
		sourceLocale: cfg.SourceLocale,
		targetLocale: cfg.TargetLocale,
		vocab:        vocab,
	}
	t.ToolName = string(p.Name()) + "-translate"
	t.ToolDescription = "Translates Blocks using " + string(p.Name())
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

	// Route through the HTML-preserving path when the source has any
	// non-text runs (inline codes).
	sourceRuns := block.SourceRuns()
	if hasInlineCodes(sourceRuns) {
		return t.handleBlockWithInlineCodes(part, block, sourceRuns)
	}

	// Plain text translation.
	resp, err := t.provider.Translate(context.Background(), mtprovider.TranslateRequest{
		Source:       sourceText,
		SourceLocale: t.sourceLocale,
		TargetLocale: t.targetLocale,
	})
	if err != nil {
		return nil, fmt.Errorf("%s-translate: %w", string(t.provider.Name()), err)
	}

	block.SetTargetText(t.targetLocale, resp.Translation)
	return part, nil
}

// handleBlockWithInlineCodes translates a block whose source contains
// inline codes. Source and target round-trip through the legacy
// Fragment/SemanticHTML representation — MT APIs preserve HTML tags
// natively, so rendering through semantic tags is the most robust
// format for this pipeline.
func (t *MTTranslateTool) handleBlockWithInlineCodes(part *model.Part, block *model.Block, sourceRuns []model.Run) (*model.Part, error) {
	frag := model.RunsToFragment(sourceRuns)
	sourceHTML := frag.SemanticHTML(t.vocab)

	resp, err := t.provider.Translate(context.Background(), mtprovider.TranslateRequest{
		Source:       sourceHTML,
		SourceLocale: t.sourceLocale,
		TargetLocale: t.targetLocale,
	})
	if err != nil {
		return nil, fmt.Errorf("%s-translate: %w", string(t.provider.Name()), err)
	}

	targetFrag := model.ParseSemanticHTML(resp.Translation, frag.Spans, t.vocab)
	block.SetTargetRuns(t.targetLocale, model.FragmentToRuns(targetFrag))
	return part, nil
}

// hasInlineCodes reports whether a Run sequence contains any non-text
// run (Ph / PcOpen / PcClose / Sub / Plural / Select).
func hasInlineCodes(runs []model.Run) bool {
	for _, r := range runs {
		if r.Text == nil {
			return true
		}
	}
	return false
}
