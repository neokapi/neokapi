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
	// configFP fingerprints the output-affecting config (provider, locales) so the
	// session overlay cache re-translates after a provider/locale change instead
	// of serving a stale cached target. See tool.OverlayConfigFingerprint.
	configFP string
}

// MTTranslateConfig holds configuration for the MT translate tool.
//
// Locale fields are supplied programmatically by the runner. The credential
// fields carry schema/json tags so they surface as CLI flags and flow config;
// they are populated by the shared credential resolver (see
// cli/credentials/resolve.go) or inline in a recipe step. The provider itself
// is fixed by the registered tool name (e.g. routed to deepl via --provider), so
// there is no Provider field.
type MTTranslateConfig struct {
	SourceLocale model.LocaleID `json:"sourceLocale,omitempty"     schema:"-"`
	TargetLocale model.LocaleID `json:"targetLocale,omitempty"     schema:"-"`

	// APIKey is the primary credential (deepl, google, modernmt). Resolved from
	// the keychain by the CLI credential preprocessor, or set inline.
	APIKey string `json:"apiKey,omitempty"          schema:"title=API Key,description=API key for the MT provider,group=provider,widget=password"`
	// SubscriptionKey is the Azure credential for the microsoft provider.
	SubscriptionKey string `json:"subscriptionKey,omitempty" schema:"title=Subscription Key,description=Azure subscription key (microsoft),group=provider,widget=password"`
	// Region is the Azure region for the microsoft provider.
	Region string `json:"region,omitempty"          schema:"title=Region,description=Azure region (microsoft),group=provider"`
	// Email is the optional MyMemory account email for higher rate limits.
	Email string `json:"email,omitempty"           schema:"title=Email,description=Account email for higher rate limits (mymemory),group=provider"`
	// ProjectID is the optional Google Cloud project id.
	ProjectID string `json:"projectId,omitempty"       schema:"title=Project ID,description=Google Cloud project ID (google),group=provider"`
	// BaseURL overrides the provider API endpoint (primarily for tests).
	BaseURL string `json:"baseURL,omitempty"         schema:"-"`

	// ToolName, when set, fixes the reported tool name
	// regardless of the backing provider. The registry sets this so a tool
	// registered as "translate" but constructed with the offline demo
	// provider as a default still reports its registered name. When empty, the
	// name is derived from the provider id (<provider>-translate).
	ToolName string `json:"-" schema:"-"`
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
	name := cfg.ToolName
	if name == "" {
		name = string(p.Name()) + "-translate"
	}
	t.ToolName = name
	t.ToolDescription = "Translates Blocks using " + string(p.Name())
	t.configFP = tool.OverlayConfigFingerprint("mt", string(p.Name()), string(cfg.SourceLocale), string(cfg.TargetLocale))
	// Translate: writes the target locale; source stays read-only.
	t.Produce = t.translate
	return t
}

// translate writes the MT target for one block. Source is read-only (the
// VariantView exposes no source setter). When the source carries inline codes
// it round-trips through RunsSemanticHTML — MT APIs preserve HTML tags
// natively, so semantic tags are the most robust transport for the codes.
func (t *MTTranslateTool) translate(v tool.VariantView) error {
	if !v.Translatable() {
		return nil
	}

	sourceText := v.SourceText()
	if sourceText == "" {
		return nil
	}

	sourceRuns := v.SourceRuns()
	if hasInlineCodes(sourceRuns) {
		resp, err := t.provider.Translate(v.Context(), mtprovider.TranslateRequest{
			Source:       model.RunsSemanticHTML(sourceRuns, t.vocab),
			SourceLocale: t.sourceLocale,
			TargetLocale: t.targetLocale,
		})
		if err != nil {
			return fmt.Errorf("%s-translate: %w", string(t.provider.Name()), err)
		}
		v.SetTargetRuns(t.targetLocale, model.ParseRunsSemanticHTML(resp.Translation, sourceRuns, t.vocab))
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

// mtOrigin describes a target produced by this MT tool.
func (t *MTTranslateTool) mtOrigin() model.Origin {
	return model.Origin{Kind: model.OriginMT, Engine: string(t.provider.Name())}
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
// the MT API — same incremental-work story as translate. MT is
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

// mtTargetCache is the payload stored in `targets/<locale>` overlays
// written by MT translate. Shape-compatible with translate's
// overlay so sessions can interop.
type mtTargetCache struct {
	Text     string `json:"text"`
	Provider string `json:"provider,omitempty"`
	// Config is the tool-config fingerprint at write time; a cached target is
	// reused only when it matches the current tool's fingerprint.
	Config string `json:"config,omitempty"`
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
	hash := block.ID
	if hash == "" {
		return t.translate(tool.NewVariantViewWithContext(ctx, block))
	}

	if randomAccess {
		if sc, err := sess.GetOverlay(overlayKind, hash); err == nil && len(sc.Payload) > 0 {
			var cached mtTargetCache
			if err := json.Unmarshal(sc.Payload, &cached); err == nil && cached.Text != "" && cached.Config == t.configFP {
				block.SetTargetText(t.targetLocale, cached.Text)
				block.StampTargetProvenance(t.targetLocale, model.TargetStatusDraft, t.mtOrigin())
				return nil
			}
		}
	}

	if err := t.translate(tool.NewVariantViewWithContext(ctx, block)); err != nil {
		return err
	}

	if target := block.TargetText(t.targetLocale); target != "" {
		payload, err := json.Marshal(mtTargetCache{Text: target, Provider: string(t.provider.Name()), Config: t.configFP})
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
