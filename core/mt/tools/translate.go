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
	// Translate: writes the target locale; source stays read-only.
	t.Translate = t.translate
	return t
}

// translate writes the MT target for one block. Source is read-only (the
// TargetView exposes no source setter). When the source carries inline codes
// it round-trips through RunsSemanticHTML — MT APIs preserve HTML tags
// natively, so semantic tags are the most robust transport for the codes.
func (t *MTTranslateTool) translate(v tool.TargetView) error {
	if !v.Translatable() {
		return nil
	}

	sourceText := v.SourceText()
	if sourceText == "" {
		return nil
	}

	sourceRuns := v.SourceRuns()
	if hasInlineCodes(sourceRuns) {
		resp, err := t.provider.Translate(context.Background(), mtprovider.TranslateRequest{
			Source:       model.RunsSemanticHTML(sourceRuns, t.vocab),
			SourceLocale: t.sourceLocale,
			TargetLocale: t.targetLocale,
		})
		if err != nil {
			return fmt.Errorf("%s-translate: %w", string(t.provider.Name()), err)
		}
		v.SetTargetRuns(t.targetLocale, model.ParseRunsSemanticHTML(resp.Translation, sourceRuns, t.vocab))
		return nil
	}

	resp, err := t.provider.Translate(context.Background(), mtprovider.TranslateRequest{
		Source:       sourceText,
		SourceLocale: t.sourceLocale,
		TargetLocale: t.targetLocale,
	})
	if err != nil {
		return fmt.Errorf("%s-translate: %w", string(t.provider.Name()), err)
	}

	v.SetTargetText(t.targetLocale, resp.Translation)
	return nil
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
		return t.translate(tool.NewTargetView(block))
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

	if err := t.translate(tool.NewTargetView(block)); err != nil {
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
