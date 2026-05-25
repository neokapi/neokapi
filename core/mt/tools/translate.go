// Package tools provides pipeline tools for machine translation.
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	mtprovider "github.com/neokapi/neokapi/providers/mt"
)

// Compile-time assertion: this tool implements SessionTool.
var _ tool.SessionTool = (*MTTranslateTool)(nil)

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
	t.WritesTarget = true
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
// inline codes. Source and target round-trip through RunsSemanticHTML —
// MT APIs preserve HTML tags natively, so rendering through semantic
// tags is the most robust format for this pipeline.
func (t *MTTranslateTool) handleBlockWithInlineCodes(part *model.Part, block *model.Block, sourceRuns []model.Run) (*model.Part, error) {
	sourceHTML := model.RunsSemanticHTML(sourceRuns, t.vocab)

	resp, err := t.provider.Translate(context.Background(), mtprovider.TranslateRequest{
		Source:       sourceHTML,
		SourceLocale: t.sourceLocale,
		TargetLocale: t.targetLocale,
	})
	if err != nil {
		return nil, fmt.Errorf("%s-translate: %w", string(t.provider.Name()), err)
	}

	targetRuns := model.ParseRunsSemanticHTML(resp.Translation, sourceRuns, t.vocab)
	block.SetTargetRuns(t.targetLocale, targetRuns)
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

// SessionProcess consults `targets/<locale>` overlays before hitting
// the MT API — same incremental-work story as ai-translate. MT is
// often cheaper than LLMs but still rate-limited and billed per
// request; skipping cached targets avoids both.
func (t *MTTranslateTool) SessionProcess(
	ctx context.Context,
	sess blockstore.Session,
	in <-chan *model.Part,
	out chan<- *model.Part,
) error {
	overlayKind := "targets/" + string(t.targetLocale)
	caps := sess.Capabilities()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-in:
			if !ok {
				return nil
			}
			if err := t.sessionHandleBlock(sess, caps.RandomAccess, overlayKind, part); err != nil {
				return err
			}
			select {
			case out <- part:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

// mtTargetCache is the payload stored in `targets/<locale>` overlays
// written by MT translate. Shape-compatible with ai-translate's
// overlay so sessions can interop.
type mtTargetCache struct {
	Text     string `json:"text"`
	Provider string `json:"provider,omitempty"`
}

func (t *MTTranslateTool) sessionHandleBlock(
	sess blockstore.Session,
	randomAccess bool,
	overlayKind string,
	part *model.Part,
) error {
	block, ok := part.Resource.(*model.Block)
	if !ok || block == nil || !block.Translatable {
		return nil
	}
	hash := block.ID
	if hash == "" {
		_, err := t.handleBlock(part)
		return err
	}

	if randomAccess {
		if sc, err := sess.GetOverlay(overlayKind, hash); err == nil && len(sc.Payload) > 0 {
			var cached mtTargetCache
			if err := json.Unmarshal(sc.Payload, &cached); err == nil && cached.Text != "" {
				block.SetTargetText(t.targetLocale, cached.Text)
				return nil
			}
		}
	}

	if _, err := t.handleBlock(part); err != nil {
		return err
	}

	if target := block.TargetText(t.targetLocale); target != "" {
		payload, err := json.Marshal(mtTargetCache{Text: target, Provider: string(t.provider.Name())})
		if err != nil {
			return fmt.Errorf("mt-translate: encode overlay: %w", err)
		}
		if err := sess.PutOverlay(blockstore.Overlay{
			Kind:      overlayKind,
			BlockHash: hash,
			Payload:   payload,
		}); err != nil && !errors.Is(err, blockstore.ErrReadOnly) {
			return fmt.Errorf("mt-translate: write overlay: %w", err)
		}
	}
	return nil
}
