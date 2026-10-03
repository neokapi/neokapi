// Package tools provides pipeline tools for machine translation.
package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
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
	// configFP fingerprints the output-affecting config (provider, locales) so the
	// session overlay cache re-translates after a provider/locale change instead
	// of serving a stale cached target. See tool.OverlayConfigFingerprint.
	configFP string
	// Governing context, resolved once at construction and stamped onto every
	// target's Origin so an MT target records what governed it. See mtOrigin.
	profileID      string
	profileVersion string
	contextFP      string
	// reused counts the blocks this run served from the block store instead of
	// sending to the MT API. See AITranslateTool.ReusedTargets.
	reused atomic.Int64
}

// ReusedTargets is how many blocks this tool served from the block store rather
// than translating.
func (t *MTTranslateTool) ReusedTargets() int { return int(t.reused.Load()) }

// ReusesStoredTarget implements tool.StoredTargetReuser: the stored target
// answers this block when it was made from the same source wording by this
// engine for this language pair, under the governing context in force. Every
// store lookup this tool makes goes through here, and so does the plan's.
func (t *MTTranslateTool) ReusesStoredTarget(_ context.Context, b *model.Block, stored blockstore.TargetOverlay) bool {
	if b == nil {
		return false
	}
	return stored.ReusableFor(b.SourceText(), t.configFP, t.contextFP)
}

// MTTranslateConfig holds configuration for the MT translate tool.
//
// Locale fields are supplied programmatically by the runner. The credential
// field carries schema/json tags so it surfaces as a CLI flag and flow config;
// it is populated by the shared credential resolver (see
// cli/credentials/resolve.go) or inline in a recipe step. The provider itself
// is fixed by the id routed via --provider, so there is no Provider field.
type MTTranslateConfig struct {
	SourceLocale model.LocaleID `json:"sourceLocale,omitempty"     schema:"-"`
	TargetLocale model.LocaleID `json:"targetLocale,omitempty"     schema:"-"`

	// APIKey is the primary credential for keyed MT providers. Resolved from
	// the keychain by the CLI credential preprocessor, or set inline.
	APIKey string `json:"apiKey,omitempty"          schema:"title=API Key,description=API key for the MT provider,group=provider,widget=password"`
	// BaseURL is the endpoint the provider is called at, for a self-hosted or
	// private-cloud deployment.
	//
	// It is HOST-OWNED, not recipe-settable. `schema:"-"` keeps it off the CLI
	// and the form, but `schema` tags have no bearing on what a step's config:
	// map can reach — core/schema.ApplyConfig is a json round-trip, so the json
	// tag alone decides. What actually keeps a recipe out of it is
	// host/credentials.ResolveCredentials, which clears this key on the way in
	// and re-sets it only from a resolved credential's own base_url. The json
	// tag stays so that host-resolved value can bind.
	//
	// The endpoint and the key sent to it are one decision: they are configured
	// together with `kapi credentials add --base-url`, and neither is read from
	// a committable recipe.
	BaseURL string `json:"baseURL,omitempty"         schema:"-"`

	// ToolName, when set, fixes the reported tool name
	// regardless of the backing provider. The registry sets this so a tool
	// registered as "translate" but constructed with the offline demo
	// provider as a default still reports its registered name. When empty, the
	// name is derived from the provider id (<provider>-translate).
	ToolName string `json:"-" schema:"-"`

	// Profile and TermRules carry the governing context the flow's bindings
	// inject into the unified translate tool. A classic MT engine takes neither
	// voice guidance nor terminology, so they are never sent to the provider —
	// but they are the context governing the collection, so they are recorded on
	// the target's Origin (Profile/ProfileVersion/ContextFingerprint). That keeps
	// an MT target's governance stamp comparable to an AI or recycled one: all
	// fall stale together when the profile or terms move.
	Profile   *coreprofile.VoiceProfile `json:"-" schema:"-"`
	TermRules []coreprofile.TermRule    `json:"term_rules,omitempty" schema:"-"`
}

// NewMTTranslateTool creates a new MT translation tool.
func NewMTTranslateTool(p mtprovider.MTProvider, cfg MTTranslateConfig) *MTTranslateTool {
	t := &MTTranslateTool{
		provider:     p,
		sourceLocale: cfg.SourceLocale,
		targetLocale: cfg.TargetLocale,
		vocab:        model.DefaultVocabulary(),
	}
	name := cfg.ToolName
	if name == "" {
		name = string(p.Name()) + "-translate"
	}
	t.ToolName = name
	t.ToolDescription = "Translates Blocks using " + string(p.Name())
	t.configFP = tool.OverlayConfigFingerprint("mt", string(p.Name()), string(cfg.SourceLocale), string(cfg.TargetLocale))
	t.profileID, t.profileVersion, t.contextFP = coreprofile.GovernanceContext(cfg.Profile, cfg.TermRules)
	// Translate: writes the target locale; source stays read-only.
	t.Produce = t.translate
	return t
}

// translate writes the MT target for one block. Source is read-only (the
// VariantView exposes no source setter). When the source carries inline codes
// it round-trips through RunsSemanticHTML — MT APIs preserve HTML tags
// natively, so semantic tags are the most robust transport for the codes. A
// source holding plurals or selects is translated a sequence at a time
// (tool.TranslateStructures), so every branch is translated and none is lost.
func (t *MTTranslateTool) translate(v tool.VariantView) error {
	if !v.Translatable() {
		return nil
	}

	sourceText := v.SourceText()
	if sourceText == "" {
		return nil
	}

	sourceRuns := v.SourceRuns()
	if model.HasStructuredRuns(sourceRuns) || model.RunsHaveInlineCodes(sourceRuns) {
		runs, err := tool.TranslateStructures(sourceRuns, func(seq []model.Run) ([]model.Run, error) {
			return t.translateSequence(v.Context(), seq)
		})
		if err != nil {
			return err
		}
		v.SetTargetRuns(t.targetLocale, runs)
		v.StampTargetProvenance(t.targetLocale, model.TargetStatusDraft, t.mtOrigin())
		return nil
	}

	resp, err := t.provider.Translate(v.Context(), mtprovider.TranslateRequest{
		Source:       sourceText,
		SourceLocale: t.sourceLocale,
		TargetLocale: t.targetLocale,
	})
	if err != nil {
		return fmt.Errorf("%s-translate: %w", string(t.provider.Name()), err)
	}

	v.SetTargetText(t.targetLocale, resp.Translation)
	v.StampTargetProvenance(t.targetLocale, model.TargetStatusDraft, t.mtOrigin())
	return nil
}

// translateSequence translates one run sequence that holds no plural or
// select: its text alone, or, when it holds inline codes, through semantic
// HTML.
func (t *MTTranslateTool) translateSequence(ctx context.Context, seq []model.Run) ([]model.Run, error) {
	if !model.RunsHaveInlineCodes(seq) {
		resp, err := t.provider.Translate(ctx, mtprovider.TranslateRequest{
			Source:       model.RunsText(seq),
			SourceLocale: t.sourceLocale,
			TargetLocale: t.targetLocale,
		})
		if err != nil {
			return nil, fmt.Errorf("%s-translate: %w", string(t.provider.Name()), err)
		}
		return []model.Run{model.TextR(resp.Translation)}, nil
	}
	resp, err := t.provider.Translate(ctx, mtprovider.TranslateRequest{
		Source:       model.RunsSemanticHTML(seq, t.vocab),
		SourceLocale: t.sourceLocale,
		TargetLocale: t.targetLocale,
	})
	if err != nil {
		return nil, fmt.Errorf("%s-translate: %w", string(t.provider.Name()), err)
	}
	// Whether a provider honours <code> is the provider's business, and a
	// translated command is worse than an untranslated sentence, so the
	// protected spans are put back rather than asked for.
	translated := model.ParseRunsSemanticHTML(resp.Translation, seq, t.vocab)
	return model.RestoreNonTranslatable(translated, seq), nil
}

// mtOrigin describes a target produced by this MT tool: how it was made (the
// provider) and which context governed it. The engine does not consume the
// context, but a later reader cannot recover it after the fact — the profile is
// edited in place and the terminology carries no version — so it is stamped at
// production time, the same governance half the AI and recycle producers record.
func (t *MTTranslateTool) mtOrigin() model.Origin {
	return model.Origin{
		Kind:               model.OriginMT,
		Engine:             string(t.provider.Name()),
		Profile:            t.profileID,
		ProfileVersion:     t.profileVersion,
		ContextFingerprint: t.contextFP,
	}
}

// SessionProcess consults `targets/<locale>` overlays before hitting
// the MT API — same incremental-work story as translate. MT is
// often cheaper than LLMs but still rate-limited and billed per
// request; skipping cached targets avoids both.
func (t *MTTranslateTool) SessionProcess(
	ctx context.Context,
	sess blockstore.Session,
	in <-chan *model.Part,
	out chan<- *model.Part,
) error {
	overlayKind := blockstore.TargetOverlayKind(t.targetLocale)
	caps := sess.Capabilities()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-in:
			if !ok {
				return nil
			}
			if err := t.sessionHandleBlock(ctx, sess, caps.RandomAccess, overlayKind, part); err != nil {
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

func (t *MTTranslateTool) sessionHandleBlock(
	ctx context.Context,
	sess blockstore.Session,
	randomAccess bool,
	overlayKind string,
	part *model.Part,
) error {
	block, ok := part.Resource.(*model.Block)
	if !ok || block == nil || !block.Translatable {
		return nil
	}
	if block.ID == "" {
		return tool.WriteAs(ctx, block, t.ToolName, t.translate)
	}
	// Key overlays globally-unique per source file (falls back to the raw id for
	// ad-hoc single-document runs) so multi-file projects don't collide.
	hash := blockstore.OverlayKey(ctx, block.ID, block.SourceText())

	if randomAccess {
		if sc, err := sess.GetOverlay(overlayKind, hash); err == nil && len(sc.Payload) > 0 {
			var cached blockstore.TargetOverlay
			if err := json.Unmarshal(sc.Payload, &cached); err == nil && t.ReusesStoredTarget(ctx, block, cached) {
				err := tool.WriteAs(ctx, block, t.ToolName, func(v tool.VariantView) error {
					if len(cached.Runs) > 0 {
						v.SetTargetRuns(t.targetLocale, cached.Runs)
					} else {
						v.SetTargetText(t.targetLocale, cached.TargetText())
					}
					v.StampTargetProvenance(t.targetLocale, model.TargetStatusDraft, t.mtOrigin())
					return nil
				})
				if err != nil {
					return err
				}
				t.reused.Add(1)
				return nil
			}
		}
	}

	if err := tool.WriteAs(ctx, block, t.ToolName, t.translate); err != nil {
		return err
	}

	if target := block.TargetText(t.targetLocale); target != "" {
		payload, err := json.Marshal(blockstore.TargetOverlay{
			Runs:     block.TargetRuns(t.targetLocale),
			Text:     target,
			Status:   string(model.TargetStatusDraft),
			Provider: string(t.provider.Name()),
			Config:   t.configFP,
			Source:   blockstore.SourceStamp(block.SourceText()),
			Origin:   blockstore.OverlayOrigin(t.mtOrigin()),
		})
		if err != nil {
			return fmt.Errorf("translate: encode overlay: %w", err)
		}
		if err := sess.PutOverlay(blockstore.Overlay{
			Kind:      overlayKind,
			BlockHash: hash,
			Payload:   payload,
		}); err != nil && !errors.Is(err, blockstore.ErrReadOnly) {
			return fmt.Errorf("translate: write overlay: %w", err)
		}
	}
	return nil
}
