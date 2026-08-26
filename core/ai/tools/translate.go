package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/neokapi/neokapi/core/ai/prompt"
	"github.com/neokapi/neokapi/core/blockstore"
	corememory "github.com/neokapi/neokapi/core/memory"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// Compile-time assertion: this tool implements SessionTool.
var _ tool.SessionTool = (*AITranslateTool)(nil)

// AITranslateTool translates untranslated Blocks using an LLM provider.
type AITranslateTool struct {
	tool.BaseTool
	usageAccumulator
	provider     aiprovider.LLMProvider
	streaming    aiprovider.StreamingLLMProvider // nil when provider doesn't support streaming
	sourceLocale model.LocaleID
	targetLocale model.LocaleID
	// termRules is the terminology governing this run, kept whole so each call
	// can send the rules its own text could use. See termsFor.
	termRules []coreprofile.TermRule
	// termMap is every rule projected to term→replacement. It is what the
	// context fingerprint is computed over — deliberately the whole set, since
	// the fingerprint is a staleness detector and a rule added about words a
	// block does not contain should still re-check that block.
	termMap     map[string]string
	dnt         []string // do-not-translate terms; masked before the model, restored after
	voiceGuide  string   // compact voice profile guidance injected into every prompt
	instruction string   // caller-supplied directive injected into every prompt
	// profileID and profileVersion identify the context profile whose guidance
	// went into voiceGuide, stamped onto every target this tool produces. The
	// guidance itself is rendered once and the profile discarded, so these are
	// kept separately — without them the governing context is unrecoverable
	// after the profile is next edited.
	profileID      string
	profileVersion string
	// contextFP hashes the governing context as it actually reached the model —
	// the rendered voice guidance and the terminology — so a target can be told
	// stale against a context that has since moved. Narrower than configFP on
	// purpose: see tool.ContextFingerprint.
	contextFP     string
	skipMatched   bool
	batchSize     int
	contextPolicy string
	contextWindow int
	// docEntries is the document in order, when the tool has it. The batched
	// path buffers the stream, so it can offer a block its neighbours; the
	// streaming path cannot, and there a block gets only its key.
	docEntries  []blockEntry
	concurrency int
	onProgress  func(aiprovider.ProgressEvent)
	blockIndex  atomic.Int32
	totalBlocks int
	// corpus answers what a block said before, gated on governance. Nil when the
	// run has no content memory, which is every ad-hoc translate.
	corpus corememory.Provider
	// point is where this run's content sits, so a block's history is read from
	// the place it belongs to rather than from wherever the corpus answers first.
	point string
	// priorCache memoizes priorVersions by chain unit. See priorFor: one block
	// asks five times in a run, and every asker must get the same answer.
	priorMu    sync.Mutex
	priorCache map[string]*prompt.PriorVersion
	// configFP fingerprints the output-affecting config (provider, model, locales,
	// term rules, voice profile). The session overlay cache stores it so a re-run with
	// a changed model/prompt/voice re-translates instead of serving the stale
	// cached target. See tool.OverlayConfigFingerprint.
	configFP string
}

// Default values for AITranslateConfig — used in both the schema tags
// (for UI display) and NewAITranslateTool (for runtime fallback).
const (
	// BatchingAuto lets kapi size each call from the model's output ceiling.
	BatchingAuto = "auto"
	// BatchingSingle translates one block per call: the most reliable and most
	// expensive setting, and what on-device models get regardless.
	BatchingSingle          = "single"
	DefaultBatchConcurrency = 1

	// ContextNone sends the block and nothing else — what kapi did before, and
	// what leaves a bare "Save" ambiguous between a verb and a noun.
	ContextNone = "none"
	// ContextKey sends the block's key or path. The default: cheap, stable, and
	// — because a key travels with its block — it cannot make a cached
	// translation wrong.
	ContextKey = "key"
	// ContextNeighbours also sends the surrounding blocks as reference material.
	// The model reads them; it must not translate them.
	ContextNeighbours = "neighbours"

	// DefaultContextWindow is how many blocks either side ContextNeighbours sends.
	DefaultContextWindow = 2
)

// AITranslateConfig holds configuration for the AI translate tool.
// Fields are exposed as CLI flags via schema tags and as flow config
// via json tags.
type AITranslateConfig struct {
	SourceLocale model.LocaleID `json:"sourceLocale,omitempty" schema:"-"`
	TargetLocale model.LocaleID `json:"targetLocale,omitempty" schema:"-"`
	Provider     string         `json:"provider,omitempty"     schema:"title=AI Provider,description=AI provider,default=anthropic,group=provider"`
	APIKey       string         `json:"apiKey,omitempty"       schema:"title=API Key,description=API key for the AI provider,group=provider"`
	Model        string         `json:"model,omitempty"        schema:"title=Model,description=AI model name,group=provider"`
	// TermRules are the terminology constraints governing this translation: for
	// each, when the source says Term the target should say Replacement. The
	// same shape a voice profile's vocabulary uses, so one set of rules can be
	// rendered into the prompt here and checked by term-check afterwards.
	TermRules []coreprofile.TermRule `json:"term_rules,omitempty" schema:"-"`
	// Instruction is a directive applied while translating, rendered into the
	// prompt's Instruction section. It is the supported way to steer a
	// translation without replacing the prompt: "Informal register", "keep
	// product names in English". Also set programmatically — a reviewer's
	// re-translation guidance, or the findings a fix pass must resolve.
	//
	// It feeds aiConfigFingerprint, so changing it re-translates rather than
	// serving a cached target produced under different guidance.
	Instruction string `json:"instruction,omitempty" schema:"title=Instruction,description=Extra guidance for the model while translating (e.g. 'informal register; keep product names in English'),widget=textarea,group=prompt"`
	// Profile is an optional voice profile. When set, its guidance is
	// injected into the translation prompt so output is on-brand at generation
	// time. Not serializable via the schema/CLI; supplied programmatically or
	// via the .kapi voice binding.
	Profile *coreprofile.VoiceProfile `json:"-" schema:"-"`

	// PriorVersions supplies what a block said before. When set, a block whose
	// source was edited carries its previously approved translation into the
	// prompt as reference, so the model produces the edit rather than a fresh
	// translation that happens to differ everywhere the last one was settled.
	//
	// Not serializable: it is a live handle on the content memory, supplied the
	// way Profile is. A run without one translates exactly as it did before.
	Memory corememory.Provider `json:"-" schema:"-"`

	// Point is where this run's content sits (product, channel, collection). A
	// block's history is read from its own point, so wording approved for one
	// surface does not steer another. Supplied by the project bindings.
	Point string `json:"point,omitempty" schema:"-"`

	// DNT are do-not-translate terms (product names, trademarks, code
	// identifiers) that must survive VERBATIM into the target — the enforced
	// counterpart of the advisory TermRules. Where a term rule only asks the
	// model to prefer a rendering, DNT LOCKS the span: each occurrence in the
	// source is masked with a sentinel placeholder before the model sees it and
	// restored to the exact original after, so the term cannot be translated,
	// transliterated, or dropped no matter what the model returns. Sourced from
	// the recipe, a checkset, or terms do-not-translate/forbidden terms.
	//
	// Masking a span is only safe per block (the sentinel must survive a single
	// generation), so a tool configured with DNT terms translates one block per
	// call regardless of the batching setting — correctness over throughput for
	// the (usually small) set of protected terms.
	DNT []string `json:"dnt,omitempty" schema:"-"`

	SkipMatched bool `json:"skipMatched,omitempty"  schema:"title=Skip Matched,description=Skip blocks that already have a target translation"`

	// Batching selects how many blocks share one LLM call.
	//
	// Deliberately not a number. The right block count depends on the model's
	// output ceiling, on how long this project's segments happen to be, and on a
	// quality-vs-batch-size curve nobody has published — none of which a user is
	// in a position to know. The old "Batch Size: 100" field asked them to guess,
	// and a wrong guess truncated the model's reply and failed the run.
	//
	// So the meaningful choice is the one offered here: let kapi size the batch
	// from what the model can emit, or translate one block per call when
	// reliability matters more than cost.
	Batching string `json:"batching,omitempty" schema:"title=Batching,description=How many blocks share one LLM call,enum=auto|single,default=auto,group=provider"`

	// Context is what kapi tells the model about a block besides the block.
	//
	// `key` sends the block's key or path (`app.settings.title`), which is the
	// cheapest disambiguation there is — a bare "Save" is a coin flip between a
	// verb and a noun, and in German between two different words.
	//
	// `neighbours` adds the surrounding source blocks as reference material the
	// model must read but not translate. It costs tokens, and it makes a block's
	// translation depend on text that is not the block — so an edit to one string
	// re-translates its neighbours. That is correct, and it is not free, which is
	// why it is opt-in.
	Context string `json:"context,omitempty" schema:"title=Context,description=What the model is told about a block besides the block itself,enum=none|key|neighbours,default=key,group=prompt"`

	// ContextWindow is how many blocks either side to send when Context is
	// `neighbours`.
	ContextWindow int `json:"contextWindow,omitempty" schema:"title=Context Window,description=Blocks either side to send as reference (with context: neighbours),default=2,min=1,max=10,group=prompt,showIf=context=neighbours"`

	// BatchSize pins an exact block count, bypassing the packer. Not a form
	// field: it exists for a recipe that must reproduce a historical run, and for
	// the eval harness sweeping N to measure the quality curve we currently have
	// to take on faith.
	BatchSize        int `json:"batchSize,omitempty" schema:"-"`
	BatchConcurrency int `json:"batchConcurrency,omitempty" schema:"title=Batch Concurrency,description=Number of concurrent batch calls (0 or 1 = sequential),default=1,min=1"`

	// OnProgress is called for each block during translation. It receives
	// live thinking summaries when the provider supports streaming.
	OnProgress func(aiprovider.ProgressEvent) `json:"-"`
}

// injectProviderOptions sets the Provider field's options from the canonical
// provider registry, replacing any hardcoded enum values.
func injectProviderOptions(s *schema.ComponentSchema) {
	if s == nil || s.Properties == nil {
		return
	}
	if prop, ok := s.Properties["provider"]; ok {
		var options []schema.OptionItem
		for _, p := range aiprovider.Providers() {
			options = append(options, schema.OptionItem{
				Value: p.Name,
				Label: p.Label,
			})
		}
		prop.Options = options
		s.Properties["provider"] = prop
	}
}

// ProviderFromConfig creates an LLM provider from a provider name and config.
// The provider name is looked up in the global provider registry, which
// built-in providers populate at init time and plugins can extend.
func ProviderFromConfig(name string, cfg aiprovider.Config) (aiprovider.LLMProvider, error) {
	if name == "" {
		name = string(aiprovider.Anthropic)
	}
	return aiprovider.NewProvider(aiprovider.ProviderID(name), cfg)
}

// NewAITranslateFromConfig creates an AI translate tool from a config map.
// Used by the schema-driven CLI path and the flow engine.
//
// The config map may contain an "onProgress" key with a func(ProgressEvent)
// value for live progress reporting. This is extracted before JSON round-trip
// since functions aren't JSON-serializable.
func NewAITranslateFromConfig(config map[string]any, targetLang string) (tool.Tool, error) {
	// Extract non-serializable fields before JSON round-trip.
	var onProgress func(aiprovider.ProgressEvent)
	if fn, ok := config["onProgress"].(func(aiprovider.ProgressEvent)); ok {
		onProgress = fn
		delete(config, "onProgress")
	}
	var profile *coreprofile.VoiceProfile
	if pf, ok := config["profile"].(*coreprofile.VoiceProfile); ok {
		profile = pf
		delete(config, "profile")
	}
	var corpus corememory.Provider
	if c, ok := config[corememory.ConfigKey].(corememory.Provider); ok {
		corpus = c
		delete(config, corememory.ConfigKey)
	}

	var cfg AITranslateConfig
	if err := schema.ApplyConfig(config, &cfg); err != nil {
		return nil, fmt.Errorf("translate config: %w", err)
	}
	cfg.OnProgress = onProgress
	cfg.Profile = profile
	cfg.Memory = corpus

	if targetLang != "" {
		cfg.TargetLocale = model.LocaleID(targetLang)
	}

	p, err := ProviderFromConfig(cfg.Provider, aiprovider.Config{APIKey: cfg.APIKey, Model: cfg.Model})
	if err != nil {
		return nil, err
	}

	return NewAITranslateTool(p, cfg), nil
}

// NewAITranslateTool creates a new AI translation tool.
func NewAITranslateTool(p aiprovider.LLMProvider, cfg AITranslateConfig) *AITranslateTool {
	t := &AITranslateTool{
		provider:     p,
		sourceLocale: cfg.SourceLocale,
		targetLocale: cfg.TargetLocale,
		termRules:    cfg.TermRules,
		termMap:      coreprofile.TermRuleMap(cfg.TermRules),
		dnt:          sanitizeDNT(cfg.DNT),
		voiceGuide:   coreprofile.RenderVoiceGuideCompact(cfg.Profile),
		instruction:  cfg.Instruction,
		skipMatched:  cfg.SkipMatched,
		batchSize:    cfg.BatchSize,
		concurrency:  cfg.BatchConcurrency,
		onProgress:   cfg.OnProgress,

		corpus: cfg.Memory,
		point:  cfg.Point,
	}
	t.profileID, t.profileVersion, t.contextFP = coreprofile.GovernanceContext(cfg.Profile, cfg.TermRules)
	if sp, ok := p.(aiprovider.StreamingLLMProvider); ok {
		t.streaming = sp
	}
	t.contextPolicy = cfg.Context
	if t.contextPolicy == "" {
		t.contextPolicy = ContextKey
	}
	t.contextWindow = cfg.ContextWindow
	if t.contextWindow < 1 {
		t.contextWindow = DefaultContextWindow
	}

	// Batching intent → packing behaviour. batchSize 0 means "let the packer
	// decide"; an explicit BatchSize pin overrides both.
	if cfg.BatchSize < 1 {
		switch cfg.Batching {
		case BatchingSingle:
			t.batchSize = 1
		default: // BatchingAuto, and the zero value
			t.batchSize = 0
		}
	}
	// On-device models translate far better per-block than batched into one giant
	// structured-JSON generation: a small local model tends to ignore the "return
	// JSON" instruction and emit plain numbered text (the cause of the
	// "unmarshal response" failures), and a 100-block prompt is a very long, slow
	// generation with no intermediate feedback. Force per-block for local
	// providers unless the user deliberately chose a smaller batch. (The schema
	// default of 100 is already applied by here, so this overrides it rather than
	// keying off batchSize < 1.)
	if cfg.BatchSize < 1 && aiprovider.IsLocalProvider(aiprovider.ProviderID(cfg.Provider)) {
		t.batchSize = 1
	}
	// Do-not-translate enforcement masks each protected span with a sentinel
	// that must survive a single generation intact, so a tool with DNT terms
	// translates one block per call — the batch path packs many sources into one
	// structured-JSON generation where a per-span sentinel cannot be tracked
	// safely. Correctness for the (small) protected set outweighs the batch win.
	// Neighbour context also routes through the batched path (Process), which
	// never applies the mask, so DNT downgrades context to `key` — the mask,
	// which needs the single-block path, wins over the neighbourhood hint.
	if len(t.dnt) > 0 {
		t.batchSize = 1
		t.concurrency = 1
		if t.contextPolicy == ContextNeighbours {
			t.contextPolicy = ContextKey
		}
	}
	if t.concurrency < 1 {
		t.concurrency = DefaultBatchConcurrency
	}
	t.ToolName = "translate"
	t.ToolDescription = "Translates Blocks using AI/LLM"
	t.configFP = aiConfigFingerprint(cfg, t.voiceGuide)
	// Translate: writes the target locale; source stays read-only. The batched
	// and session paths (Process overrides) reuse translate() via NewVariantView.
	t.Produce = t.translate
	return t
}

// aiConfigFingerprint hashes the AI translate settings that change a block's
// output, so the session overlay cache reuses a cached target only when they are
// unchanged. The API key, batch sizing, and progress callback do not shape the
// result and are excluded.
//
// Everything prompt-shaped — the task and constraint wording, the instruction,
// the voice profile guidance, the term rules, the locales — enters through the
// prompt builder's own Fingerprint, hashed from the text it actually renders.
// Listing those inputs here by hand would mean that rewording a prompt (which
// changes output) left the fingerprint untouched, and cached targets produced by
// the old prompt would be served as if current.
func aiConfigFingerprint(cfg AITranslateConfig, voiceGuide string) string {
	p := prompt.Translate{
		SourceLocale:   cfg.SourceLocale,
		TargetLocale:   cfg.TargetLocale,
		Instruction:    cfg.Instruction,
		VoiceGuide:     voiceGuide,
		PreferredTerms: coreprofile.TermRuleMap(cfg.TermRules),
	}
	// The context *policy* is part of the fingerprint: turning context on changes
	// every prompt, so cached targets produced without it must not be served. The
	// per-block context is not — it varies per block, and is accounted for by the
	// neighbourhood digest on the cache entry itself.
	contextPolicy := cfg.Context
	if contextPolicy == "" {
		contextPolicy = ContextKey
	}
	return tool.OverlayConfigFingerprint(
		"ai",
		cfg.Provider,
		cfg.Model,
		p.Fingerprint(),
		contextPolicy,
	)
}

// Process overrides BaseTool.Process to support batch + concurrent translation.
// When batchSize > 1 or concurrency > 1, blocks are grouped into batches of
// batchSize and processed with up to concurrency goroutines in parallel.
func (t *AITranslateTool) Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	// Neighbour context needs the document in order, and only the batched path
	// buffers the stream. So `batching: single` + `context: neighbours` routes
	// here too — and lands on exactly the shape the evidence favours: a large
	// non-translated context with a small translation unit.
	if t.batchSize <= 1 && t.concurrency <= 1 && t.contextPolicy != ContextNeighbours {
		return t.BaseTool.Process(ctx, in, out)
	}
	return t.processBatched(ctx, in, out)
}

// SessionProcess consults the session's `targets/<locale>` overlays
// before calling the LLM. A block whose target is already cached
// gets hydrated from the overlay and skipped — key for incremental
// AI translation runs where re-calling the model is expensive.
// After a fresh translation, the resulting target is written back
// to the session so the next run can skip it.
//
// Batch + concurrent paths remain unchanged — they route through
// Process without session involvement. Users who want session-aware
// translation run with batchSize/concurrency at defaults (1) so the
// block-by-block path applies.
func (t *AITranslateTool) SessionProcess(
	ctx context.Context,
	sess blockstore.Session,
	in <-chan *model.Part,
	out chan<- *model.Part,
) error {
	if t.batchSize > 1 || t.concurrency > 1 {
		// Batched path has its own goroutine fan-out; session-aware
		// hydration is a sequential optimisation. Let the batch
		// path run and write overlays on the way out.
		return t.processBatchedWithSession(ctx, sess, in, out)
	}

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

// sessionHandleBlock wraps handleBlock with an overlay lookup before
// and an overlay write after. Falls back to pass-through for non-
// Block parts.
func (t *AITranslateTool) sessionHandleBlock(
	ctx context.Context,
	sess blockstore.Session,
	randomAccess bool,
	overlayKind string,
	part *model.Part,
) error {
	block, ok := part.Resource.(*model.Block)
	if !ok || block == nil {
		return nil
	}
	if !block.Translatable {
		return nil
	}
	if block.ID == "" {
		return t.translate(tool.NewVariantViewWithContext(ctx, block)) //nolint:contextcheck // ctx travels inside the VariantView; translate keeps the view-only Produce signature
	}
	// Key overlays globally-unique per source file (falls back to the raw id for
	// ad-hoc single-document runs) so multi-file projects don't collide.
	hash := blockstore.OverlayKey(ctx, block.ID, block.SourceText())

	// The cache key for this block: the tool config, plus the digest of its
	// neighbourhood when neighbours are being sent.
	//
	// Without the digest, sending neighbours would be a correctness bug rather
	// than a feature: a block's translation would depend on text that is not the
	// block, while its cache entry pretended otherwise — so editing one string
	// would leave the strings around it holding translations produced under a
	// neighbourhood that no longer exists.
	cacheFP := t.cacheFingerprint(ctx, block)

	// Skip if already cached under the same config (a changed model/prompt/voice
	// invalidates the cached target — re-translate rather than serve it stale).
	if randomAccess {
		if sc, err := sess.GetOverlay(overlayKind, hash); err == nil && len(sc.Payload) > 0 {
			var cached aiTargetCache
			if err := json.Unmarshal(sc.Payload, &cached); err == nil && cached.Text != "" && cached.Config == cacheFP {
				block.SetTargetText(t.targetLocale, cached.Text)
				block.StampTargetProvenance(t.targetLocale, model.TargetStatusDraft, t.aiOrigin())
				return nil
			}
		}
	}

	if err := t.translate(tool.NewVariantViewWithContext(ctx, block)); err != nil { //nolint:contextcheck // ctx travels inside the VariantView; translate keeps the view-only Produce signature
		return err
	}

	if target := block.TargetText(t.targetLocale); target != "" {
		payload, err := json.Marshal(aiTargetCache{
			Text:     target,
			Provider: string(t.provider.Name()),
			Config:   cacheFP,
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
	_ = ctx // ctx reserved for future streaming hooks
	return nil
}

// processBatchedWithSession funnels the batched path through the
// same overlay-aware gate. We pre-filter the input channel: blocks
// whose target is cached get hydrated + forwarded immediately;
// everything else goes through processBatched, then the results
// get overlay-written before they're forwarded.
func (t *AITranslateTool) processBatchedWithSession(
	ctx context.Context,
	sess blockstore.Session,
	in <-chan *model.Part,
	out chan<- *model.Part,
) error {
	overlayKind := "targets/" + string(t.targetLocale)
	caps := sess.Capabilities()

	// Filter: split cached vs. needs-translation.
	toTranslate := make(chan *model.Part, t.batchSize*2)

	forward := func(p *model.Part) bool {
		select {
		case toTranslate <- p:
			return true
		case <-ctx.Done():
			return false
		}
	}
	go func() {
		defer close(toTranslate)
		for {
			select {
			case <-ctx.Done():
				return
			case part, ok := <-in:
				if !ok {
					return
				}
				block, ok := part.Resource.(*model.Block)
				if !ok || block == nil || !block.Translatable || block.ID == "" {
					if !forward(part) {
						return
					}
					continue
				}
				// Hydrating from the cache here means deciding, while streaming, that
				// a block needs no work. With neighbour context that decision cannot
				// be made yet: the cache entry is only valid for the neighbourhood it
				// was produced under, and the neighbourhood is not known until the
				// document has been drained. So in that mode the block goes through
				// to the batch path, which knows both. A cache miss is a cost; a
				// cache hit under the wrong neighbourhood is a wrong answer.
				if caps.RandomAccess && t.contextPolicy != ContextNeighbours {
					if sc, err := sess.GetOverlay(overlayKind, blockstore.OverlayKey(ctx, block.ID, block.SourceText())); err == nil && len(sc.Payload) > 0 {
						var cached aiTargetCache
						if err := json.Unmarshal(sc.Payload, &cached); err == nil && cached.Text != "" && cached.Config == t.cacheFingerprint(ctx, block) {
							block.SetTargetText(t.targetLocale, cached.Text)
							block.StampTargetProvenance(t.targetLocale, model.TargetStatusDraft, t.aiOrigin())
							select {
							case out <- part:
							case <-ctx.Done():
								return
							}
							continue
						}
					}
				}
				if !forward(part) {
					return
				}
			}
		}
	}()

	// processBatched writes the translated parts to batchOut; we wrap each
	// with the overlay write before forwarding to the caller.
	batchOut := make(chan *model.Part, t.batchSize*2)
	done := make(chan error, 1)
	go func() {
		done <- t.processBatched(ctx, toTranslate, batchOut)
		close(batchOut)
	}()

	for part := range batchOut {
		if block, ok := part.Resource.(*model.Block); ok && block != nil && block.ID != "" {
			if target := block.TargetText(t.targetLocale); target != "" {
				payload, err := json.Marshal(aiTargetCache{
					Text:     target,
					Provider: string(t.provider.Name()),
					Config:   t.cacheFingerprint(ctx, block),
				})
				if err == nil {
					if werr := sess.PutOverlay(blockstore.Overlay{
						Kind:      overlayKind,
						BlockHash: blockstore.OverlayKey(ctx, block.ID, block.SourceText()),
						Payload:   payload,
					}); werr != nil && !errors.Is(werr, blockstore.ErrReadOnly) {
						return fmt.Errorf("translate: write overlay: %w", werr)
					}
				}
			}
		}
		select {
		case out <- part:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if err := <-done; err != nil {
		return err
	}
	return nil
}

// aiTargetCache is the payload stored in `targets/<locale>` overlays
// written by translate. Kept small and JSON-compatible with
// other translators so sessions can interop freely.
type aiTargetCache struct {
	Text     string `json:"text"`
	Provider string `json:"provider,omitempty"`
	// Config is the tool-config fingerprint at write time. A cached target is
	// reused only when it matches the current tool's fingerprint, so a changed
	// model/prompt/voice re-translates rather than serving stale output.
	Config string `json:"config,omitempty"`
}

// ---------------------------------------------------------------------------
// Single-block processing (existing behavior)
// ---------------------------------------------------------------------------

// translate writes the AI target for one block. Source is read-only (the
// VariantView exposes no source setter). Dispatched directly for the
// block-by-block path; the batched and session paths call it via NewVariantView.
func (t *AITranslateTool) translate(v tool.VariantView) error {
	if !v.Translatable() {
		return nil
	}

	// Source-gate hold (epic 019): a block whose source ranks below the active
	// source gate carries the hold marker set by the leading source-gate stage.
	// Do not translate it — its source is un-settled; it holds until settled or
	// the gate is lowered. The gate is off (or the source cleared) when the
	// marker is absent, so this is a no-op for the ungated path.
	if v.Property(model.PropSourceHeld) == "1" {
		return nil
	}

	if t.skipMatched && v.HasTarget(t.targetLocale) {
		return nil
	}

	sourceText := v.SourceText()
	if sourceText == "" {
		return nil
	}

	t.blockIndex.Add(1)
	// Signal the block is starting, so the progress line advances when a (slow,
	// on-device) generation begins rather than only when it finishes.
	t.emitProgress(false, "")

	// If the source has inline codes, route through the
	// placeholder-preserving LLM path so the model can keep them
	// intact.
	sourceRuns := v.SourceRuns()
	if model.RunsHaveInlineCodes(sourceRuns) {
		return t.translateWithInlineCodes(v, sourceRuns)
	}

	// Do-not-translate enforcement: mask each protected span before the model
	// sees it and restore it verbatim after, so a DNT term cannot be translated
	// or dropped (epic 019, item 4). No-op when no DNT terms are configured.
	maskedSource, dntRestoreMap := dntMask(sourceText, t.dnt)

	// Plain text translation.
	ctx := aiprovider.WithMaxOutputTokens(v.Context(), blockOutputBudget(maskedSource))
	resp, err := t.translateBlock(ctx, aiprovider.TranslateRequest{
		Source:         maskedSource,
		SourceLanguage: t.sourceLocale,
		TargetLocale:   t.targetLocale,
		PreferredTerms: t.termsFor(maskedSource),
		VoiceGuide:     t.voiceGuide,
		Instruction:    t.instruction,
		BlockContext:   t.contextFor(ctx, v.ID(), v.Name(), v.ChainUnit()),
	})
	if err != nil {
		return fmt.Errorf("translate: %w", err)
	}
	t.addUsage(resp.Usage)

	v.SetTargetText(t.targetLocale, dntRestore(resp.Translation, dntRestoreMap))
	t.annotateTranslation(v, resp)

	t.emitProgress(true, "")
	return nil
}

// translateBlock translates text using streaming when available (for live
// thinking progress), falling back to the standard Translate method.
//
// A reply cut off at the model's output cap is refused here rather than returned:
// a fragment of a translation is indistinguishable from a translation once it is
// committed as the target, and every caller of this function commits what it gets
// back. The block is already alone in its call with a budget sized for it, so
// there is no smaller request left to make — the run stops and says so.
func (t *AITranslateTool) translateBlock(ctx context.Context, req aiprovider.TranslateRequest) (*aiprovider.TranslateResponse, error) {
	resp, err := t.translateBlockRaw(ctx, req)
	if err != nil {
		return nil, err
	}
	if resp.Truncated {
		return nil, fmt.Errorf("%w: this block alone exceeds what %s can emit in one reply",
			aiprovider.ErrOutputTruncated, t.provider.Name())
	}
	return resp, nil
}

// translateBlockRaw performs the call, over the streaming API when one is
// available (for live thinking progress) and the plain Translate otherwise.
func (t *AITranslateTool) translateBlockRaw(ctx context.Context, req aiprovider.TranslateRequest) (*aiprovider.TranslateResponse, error) {
	if t.streaming == nil || t.onProgress == nil {
		return t.provider.Translate(ctx, req)
	}

	// Same prompt Translate would build — rendered from the one builder rather
	// than restated here, so the streaming and non-streaming paths cannot drift.
	p := req.Prompt()
	turns := p.SingleWithContext(req.Source, req.PreserveTags, req.BlockContext)
	ctx = prompt.WithMeta(ctx, p.Meta(prompt.IDTranslateSingle))

	resp, err := t.streaming.ChatStream(ctx, aiprovider.MessagesFromTurns(turns),
		func(e aiprovider.ChatStreamEvent) {
			if e.Type == aiprovider.StreamEventThinking {
				t.emitProgress(false, e.Content)
			}
		})
	if err != nil {
		return nil, err
	}

	return &aiprovider.TranslateResponse{
		Translation: resp.Content,
		Confidence:  0.85,
		Model:       resp.Model,
		Usage:       resp.Usage,
		Truncated:   resp.Truncated,
	}, nil
}

// handleBlockWithInlineCodes translates a block that contains
// inline codes. Renders source runs as placeholder-tagged text so
// the LLM can preserve tag positions, then reconstructs the target
// Run sequence from the response via ParseRunsPlaceholderText.
func (t *AITranslateTool) translateWithInlineCodes(v tool.VariantView, sourceRuns []model.Run) error {
	sourceText := model.RunsPlaceholderText(sourceRuns)

	// DNT enforcement (epic 019, item 4): mask protected spans in the
	// placeholder text before translation and restore them in the model's reply
	// before it is parsed back into runs — the sentinels ride through as plain
	// text runs, so restoration lands the exact original term in the target.
	maskedSource, dntRestoreMap := dntMask(sourceText, t.dnt)

	// Source is the content to translate, never an instruction: PreserveTags
	// carries the tag rule into the prompt. (This path used to pass a fully
	// built instruction as Source, which standardTranslate then wrapped in a
	// second instruction — so the model was handed a prompt to translate.)
	ctx := aiprovider.WithMaxOutputTokens(v.Context(), blockOutputBudget(maskedSource))
	resp, err := t.translateBlock(ctx, aiprovider.TranslateRequest{
		Source:         maskedSource,
		SourceLanguage: t.sourceLocale,
		TargetLocale:   t.targetLocale,
		PreferredTerms: t.termsFor(maskedSource),
		VoiceGuide:     t.voiceGuide,
		Instruction:    t.instruction,
		PreserveTags:   true,
	})
	if err != nil {
		return fmt.Errorf("translate: %w", err)
	}
	t.addUsage(resp.Usage)

	targetRuns := model.ParseRunsPlaceholderText(dntRestore(resp.Translation, dntRestoreMap), sourceRuns)
	// The DNT masking above protects configured TERMS. A code span is
	// protected structurally instead — the reader marked its runs — so it is
	// restored from the source rather than matched by string, which cannot
	// over-match a command that also appears in the prose.
	v.SetTargetRuns(t.targetLocale, model.RestoreNonTranslatable(targetRuns, sourceRuns))
	t.annotateTranslation(v, resp)

	t.emitProgress(true, "")
	return nil
}

// decodeFirstJSON decodes the first JSON value in s into v, tolerating a leading
// markdown code fence / prose and any trailing text. Local models frequently pad
// their structured output (a ```json fence, a "Here is the translation:" lead-in,
// or an explanatory note after the JSON), which a strict json.Unmarshal rejects
// with "invalid character ... after top-level value". It errors only when no
// JSON value can be found at all.
func decodeFirstJSON(s string, v any) error {
	s = strings.TrimSpace(s)
	// Unwrap a ```json … ``` (or bare ```) code fence if present.
	if i := strings.Index(s, "```"); i >= 0 {
		s = s[i+3:]
		s = strings.TrimPrefix(s, "json")
		s = strings.TrimPrefix(s, "JSON")
		if j := strings.Index(s, "```"); j >= 0 {
			s = s[:j]
		}
		s = strings.TrimSpace(s)
	}
	// Skip any leading prose to the first JSON object/array.
	switch k := strings.IndexAny(s, "{["); {
	case k > 0:
		s = s[k:]
	case k < 0:
		return errors.New("no JSON object or array found in model output")
	}
	// json.Decoder reads exactly one value and ignores any trailing data.
	return json.NewDecoder(strings.NewReader(s)).Decode(v)
}

// SetTotalBlocks sets the total number of translatable blocks for progress
// reporting. Call this before Process if the count is known.
func (t *AITranslateTool) SetTotalBlocks(n int) {
	t.totalBlocks = n
}

func (t *AITranslateTool) emitProgress(done bool, thinking string) {
	if t.onProgress == nil {
		return
	}
	t.onProgress(aiprovider.ProgressEvent{
		Block:       int(t.blockIndex.Load()),
		TotalBlocks: t.totalBlocks,
		Thinking:    thinking,
		Done:        done,
	})
}

func (t *AITranslateTool) annotateTranslation(v tool.VariantView, resp *aiprovider.TranslateResponse) {
	v.AddAltTranslation(&model.AltTranslation{
		Target:    []model.Run{{Text: &model.TextRun{Text: resp.Translation}}},
		Locale:    t.targetLocale,
		Origin:    "ai:" + string(t.provider.Name()),
		Score:     resp.Confidence,
		MatchType: model.MatchAI,
	})
	// Stamp how the committed target was produced so coverage and ship gates can
	// see it. A fresh machine translation lands at draft (produced, not yet
	// verified); the convergence loop promotes it on a passing check / review.
	v.StampTargetProvenance(t.targetLocale, model.TargetStatusDraft, t.aiOrigin())
}

// aiOrigin describes a target produced by this AI tool: how it was made (the
// provider), and which context governed it. The context half is stamped here
// because it is only knowable at production time — the profile is edited in
// place and the terminology carries no version at all, so a later reader cannot
// recover what shaped this target.
func (t *AITranslateTool) aiOrigin() model.Origin {
	return model.Origin{
		Kind:               model.OriginAI,
		Engine:             string(t.provider.Name()),
		Profile:            t.profileID,
		ProfileVersion:     t.profileVersion,
		ContextFingerprint: t.contextFP,
	}
}

// ---------------------------------------------------------------------------
// Batch + concurrent processing
// ---------------------------------------------------------------------------

// blockEntry tracks a translatable block within the batch pipeline.
type blockEntry struct {
	index          int          // position in the original parts slice
	part           *model.Part  // the Part containing the block
	block          *model.Block // the Block resource
	sourceText     string       // text to send to the LLM (placeholder-rendered when inline codes present)
	hasInlineCodes bool         // whether the source has inline codes (Ph/PcOpen/PcClose/Sub)
	sourceRuns     []model.Run  // source Run sequence (for placeholder reconstruction)
}

// processBatched drains all input parts, groups translatable blocks into
// batches, translates them concurrently, and writes all parts to the output
// channel in their original order.
func (t *AITranslateTool) processBatched(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	// 1. Drain input into a slice.
	var parts []*model.Part
	for part := range in {
		parts = append(parts, part)
	}

	// 2. Identify translatable blocks.
	var entries []blockEntry
	for i, part := range parts {
		if part.Type != model.PartBlock {
			continue
		}
		block, ok := part.Resource.(*model.Block)
		if !ok || !block.Translatable {
			continue
		}
		// Source-gate hold (epic 019): a block held on its un-settled source is
		// not batched — it passes through untranslated (written to output below).
		if block.SourceHeld() {
			continue
		}
		if t.skipMatched && block.HasTarget(t.targetLocale) {
			continue
		}
		src := block.SourceText()
		if src == "" {
			continue
		}
		sourceRuns := block.SourceRuns()
		inline := model.RunsHaveInlineCodes(sourceRuns)
		text := src
		if inline {
			text = model.RunsPlaceholderText(sourceRuns)
		}
		entries = append(entries, blockEntry{
			index: i, part: part, block: block,
			sourceText: text, hasInlineCodes: inline, sourceRuns: sourceRuns,
		})
	}

	// Set total for progress reporting since we know the full count.
	t.totalBlocks = len(entries)
	// The document, in order: what lets a block be given its neighbours.
	t.docEntries = entries

	// 3. Group into batches the model can actually answer, and translate them
	// with bounded concurrency. Packing is by output-token budget, not a fixed
	// count — see pack.go.
	batches := t.packBatches(entries)
	if err := goBatches(batches, t.concurrency, func(_ int, batch []blockEntry) error {
		return t.translateBatch(ctx, batch)
	}); err != nil {
		return err
	}

	// 5. Write all parts to output in original order.
	for _, part := range parts {
		select {
		case out <- part:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return nil
}

// splitAndRetry halves a batch that the model could not answer in one reply and
// translates each half. Recursion bottoms out at a single block, which takes the
// per-block path with a budget sized for that block alone; a block still too
// large there fails the run rather than committing a fragment.
func (t *AITranslateTool) splitAndRetry(ctx context.Context, entries []blockEntry) error {
	mid := len(entries) / 2
	if err := t.translateBatch(ctx, entries[:mid]); err != nil {
		return err
	}
	return t.translateBatch(ctx, entries[mid:])
}

// batchTranslationSchema returns a JSON schema for structured batch translation output.
func batchTranslationSchema() aiprovider.JSONSchema {
	return aiprovider.JSONSchema{
		Name:        "batch_translations",
		Description: "Batch translation results, one per segment id",
		Strict:      true,
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"translations": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id":   map[string]any{"type": "string"},
							"text": map[string]any{"type": "string"},
						},
						"required":             []string{"id", "text"},
						"additionalProperties": false,
					},
				},
			},
			"required":             []string{"translations"},
			"additionalProperties": false,
		},
	}
}

// batchResult is the JSON structure returned by structured batch translation.
type batchResult struct {
	Translations []struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	} `json:"translations"`
}

// blockContext builds the reference material for one block: its key, and — when
// asked — the source text of the blocks around it.
//
// neighbours is the full document-ordered entry list, and i the block's position
// in it. A nil list means the caller has no document order to offer (the
// streaming path), in which case only the key is available.
func (t *AITranslateTool) blockContext(ctx context.Context, b *model.Block, neighbours []blockEntry, i int) prompt.Context {
	if t.contextPolicy == ContextNone {
		return prompt.Context{}
	}

	// The key travels with the block, so it is available on every path.
	pc := prompt.Context{Key: blockKey(b)}
	t.attachPrior(ctx, b.ChainUnit(), &pc)

	if t.contextPolicy != ContextNeighbours || neighbours == nil {
		return pc
	}
	for j := max(i-t.contextWindow, 0); j < i; j++ {
		if txt := neighbours[j].sourceText; txt != "" {
			pc.Before = append(pc.Before, txt)
		}
	}
	for j := i + 1; j <= i+t.contextWindow && j < len(neighbours); j++ {
		if txt := neighbours[j].sourceText; txt != "" {
			pc.After = append(pc.After, txt)
		}
	}
	return pc
}

// cacheFingerprint is the config a cached target was produced under: the tool
// fingerprint, plus the digest of anything else the prompt carried.
//
// Two things can vary independently of the block: the neighbourhood, and the
// block's own previously approved translation. Both change what the model was
// asked, so both belong in the key. A prior version that reached the model
// without moving this fingerprint would be right once and stale for every run
// afterwards, serving a translation steered by an answer the chain has left
// behind.
//
// The block's *key* is deliberately not in here. A key travels with its block,
// so a cache already keyed by the block accounts for it; folding it in would
// churn the cache for no gain.
// It asks the context for its digest unconditionally rather than gating on which
// context sources are configured. Context.Digest already returns "" when nothing
// beyond the key is present, so the gate bought nothing but a place for the next
// context source to be forgotten — which is exactly how the prior version was
// nearly shipped outside the cache key.
func (t *AITranslateTool) cacheFingerprint(ctx context.Context, b *model.Block) string {
	if b == nil {
		return t.configFP
	}
	if d := t.contextFor(ctx, b.ID, b.Name, b.ChainUnit()).Digest(); d != "" {
		return t.configFP + ":" + d
	}
	return t.configFP
}

// attachPrior adds the block's previous approved translation to the prompt
// context, when there is one and it still stands.
//
// The fingerprint passed is the tool's own — the governance that is about to
// reach the model — so the comparison is between what governed the old answer
// and what governs this one, rather than against anything a caller supplied.
func (t *AITranslateTool) attachPrior(ctx context.Context, unit string, pc *prompt.Context) {
	if pv := t.priorFor(ctx, unit); pv != nil {
		pc.Prior = pv
	}
}

// priorFor is the memoized corpus read.
//
// One block asks for its prior version five times in a run: the packer asks to
// decide whether to batch it, cacheFingerprint asks three times (the skip check,
// the hit comparison and the write-back), and the translation itself asks once.
// They must all get the same answer — a fingerprint computed from a different
// answer than the prompt carried is a cache key that describes nothing — and
// paying for five reads to guarantee that would be the wrong way round.
//
// A run has one point and one governing fingerprint, so the unit is the whole
// key. A nil entry means asked and answered nothing, which is why presence in
// the map is the test rather than a nil check.
func (t *AITranslateTool) priorFor(ctx context.Context, unit string) *prompt.PriorVersion {
	if t.corpus == nil || unit == "" {
		return nil
	}

	t.priorMu.Lock()
	defer t.priorMu.Unlock()
	if pv, asked := t.priorCache[unit]; asked {
		return pv
	}

	var pv *prompt.PriorVersion
	if v, ok := t.corpus.PriorVersion(ctx, corememory.VersionRequest{
		Unit:       unit,
		Point:      t.point,
		Source:     t.sourceLocale,
		Target:     t.targetLocale,
		GovernedBy: t.contextFP,
	}); ok && v.Target != "" {
		pv = &prompt.PriorVersion{Source: v.Source, Target: v.Target}
	}
	if t.priorCache == nil {
		t.priorCache = make(map[string]*prompt.PriorVersion)
	}
	t.priorCache[unit] = pv
	return pv
}

// termsFor is the terminology one call sends: the rules whose term could appear
// in the text it carries.
//
// Scoped per call rather than per run because that is the grain at which it
// matters — a batch of twenty segments sends the union of what those twenty can
// use, not the four hundred rules the collection is governed by.
//
// The fingerprint is NOT scoped, and the asymmetry is the point: what reaches
// the model is what the model needs, and what detects staleness is everything
// that governs the content. Narrowing the fingerprint here would make a
// governance change invisible to exactly the blocks it was meant to reach.
func (t *AITranslateTool) termsFor(texts ...string) map[string]string {
	if len(t.termRules) == 0 {
		return nil
	}
	return coreprofile.ScopedTermRuleMap(t.termRules, texts...)
}

// contextFor is the reference material for one block: its key always, and its
// neighbours when the tool holds the document in order.
func (t *AITranslateTool) contextFor(ctx context.Context, id, name, unit string) prompt.Context {
	if t.contextPolicy == ContextNone {
		return prompt.Context{}
	}
	pc := prompt.Context{Key: strings.TrimSpace(name)}
	for i := range t.docEntries {
		if t.docEntries[i].block != nil && t.docEntries[i].block.ID == id {
			return t.blockContext(ctx, t.docEntries[i].block, t.docEntries, i)
		}
	}
	// No document in hand, so neighbours are unavailable. The prior version does
	// not need them: it is the block's own history, keyed on the unit the caller
	// passed, and the single-block path never buffers a document.
	t.attachPrior(ctx, unit, &pc)
	return pc
}

// batchContext is the neighbourhood *around a batch*: the blocks before the
// first and after the last. The blocks inside the batch are not context for each
// other — they are being translated, and calling a co-worker "context" is the
// confusion this whole design exists to avoid.
func (t *AITranslateTool) batchContext(batch []blockEntry, all []blockEntry) prompt.Context {
	if t.contextPolicy != ContextNeighbours || len(batch) == 0 || all == nil {
		return prompt.Context{}
	}
	first, last := indexOfEntry(all, batch[0]), indexOfEntry(all, batch[len(batch)-1])
	if first < 0 || last < 0 {
		return prompt.Context{}
	}

	var ctx prompt.Context
	for j := max(first-t.contextWindow, 0); j < first; j++ {
		if txt := all[j].sourceText; txt != "" {
			ctx.Before = append(ctx.Before, txt)
		}
	}
	for j := last + 1; j <= last+t.contextWindow && j < len(all); j++ {
		if txt := all[j].sourceText; txt != "" {
			ctx.After = append(ctx.After, txt)
		}
	}
	return ctx
}

func indexOfEntry(all []blockEntry, e blockEntry) int {
	for i := range all {
		if all[i].index == e.index {
			return i
		}
	}
	return -1
}

// blockKey is where the block lives in the document — `app.settings.title`, the
// thing that makes a bare "Save" mean something. Readers put it in Name.
func blockKey(b *model.Block) string {
	if b == nil {
		return ""
	}
	return strings.TrimSpace(b.Name)
}

// packBatches groups entries into calls. An explicit batchSize override (a
// recipe pin, or the eval harness sweeping N) is honoured verbatim; otherwise
// kapi sizes the batch from what the model can emit.
//
// A block with a previously approved answer used to be packed alone, because
// the batch payload had one shared context and a key per segment with nowhere
// to put a per-block reference. That un-batched exactly the blocks the feature
// applied to — and on a model migration, where every approved block has a
// governed prior, that was every block in the corpus. BatchSegment.Prior
// removed the restriction; this is what it bought.
func (t *AITranslateTool) packBatches(entries []blockEntry) [][]blockEntry {
	if t.batchSize > 0 {
		return chunkBlocks(entries, t.batchSize)
	}
	return packBlocks(entries, outputBudget(t.provider), MaxBlocksPerCall)
}

// translateBatch translates a batch of blocks in a single LLM call using
// structured output. The response is constrained to a JSON schema with
// index-text pairs, eliminating text parsing ambiguity.
// Falls back to individual translation for any missing entries.
func (t *AITranslateTool) translateBatch(ctx context.Context, entries []blockEntry) error {
	if len(entries) == 1 {
		return t.translate(tool.NewVariantViewWithContext(ctx, entries[0].block)) //nolint:contextcheck // ctx travels inside the VariantView; translate keeps the view-only Produce signature
	}

	texts := make([]string, len(entries))
	for i, entry := range entries {
		texts[i] = entry.sourceText
	}
	segments := prompt.BatchSegments(texts)
	// Each segment carries the key it lives under; the keys differ per segment,
	// so they ride in the payload rather than in a shared preamble.
	if t.contextPolicy != ContextNone {
		for i, entry := range entries {
			segments[i].Key = blockKey(entry.block)
		}
	}
	// Each segment carries its own previously approved answer. Per-segment
	// because it is per-block: hoisting it into the shared preamble would offer
	// one block's history to every other block in the call.
	for i, entry := range entries {
		if entry.block == nil {
			continue
		}
		segments[i].Prior = t.priorFor(ctx, entry.block.ChainUnit())
	}
	batchCtx := t.batchContext(entries, t.docEntries)
	p := prompt.Translate{
		SourceLocale:   t.sourceLocale,
		TargetLocale:   t.targetLocale,
		PreferredTerms: t.termsFor(texts...),
		VoiceGuide:     t.voiceGuide,
		Instruction:    t.instruction,
	}
	turns := p.BatchWithContext(segments, batchCtx)
	ctx = prompt.WithMeta(ctx, p.Meta(prompt.IDTranslateBatch))

	messages := aiprovider.MessagesFromTurns(turns)
	schema := batchTranslationSchema()

	// Ask for what this batch could plausibly need, not a fixed constant. The
	// provider clamps it to the model's ceiling.
	//
	// Two costs, and both have to be counted. The translation may run longer than
	// its source (German, Finnish), hence the doubling — and every reply item drags
	// its JSON scaffolding and echoed id along with it, a cost that scales with the
	// *number* of segments rather than their length. Budgeting for text alone asks
	// for less than the reply needs on a large batch of short strings, and the
	// truncation retry then re-does the work at double the price.
	need := batchOutputBudget(segments)
	ctx = aiprovider.WithMaxOutputTokens(ctx, need)

	var resp *aiprovider.ChatResponse
	var err error

	if t.streaming != nil && t.onProgress != nil {
		// Use streaming for live thinking progress on batch calls.
		resp, err = t.streaming.ChatStructuredStream(ctx, messages, schema, func(e aiprovider.ChatStreamEvent) {
			if e.Type == aiprovider.StreamEventThinking {
				t.emitProgress(false, e.Content)
			}
		})
	} else {
		resp, err = t.provider.ChatStructured(ctx, messages, schema)
	}
	if err != nil {
		// The model could not emit a reply this large. Halve the batch and try
		// again rather than failing the run: the batch was our choice, not the
		// user's, so its consequences are ours to absorb.
		if errors.Is(err, aiprovider.ErrOutputTruncated) && len(entries) > 1 {
			return t.splitAndRetry(ctx, entries)
		}
		return fmt.Errorf("translate batch: %w", err)
	}
	t.addUsage(resp.Usage)

	// A reply that stopped at the cap is a fragment — under a JSON schema, invalid
	// JSON. Recognise it as *our* batch being too big rather than reporting it as
	// the model returning garbage.
	if resp.Truncated && len(entries) > 1 {
		return t.splitAndRetry(ctx, entries)
	}

	// Parse structured JSON response. Local models often wrap their output in a
	// markdown code fence or pad it with prose before/after the JSON, so decode
	// the first JSON value tolerantly rather than requiring the whole response to
	// be exactly one JSON document.
	var result batchResult
	if err := decodeFirstJSON(resp.Content, &result); err != nil {
		return fmt.Errorf("translate batch: unmarshal response: %w (raw: %.200q)", err, resp.Content)
	}

	// Map the reply back by the id we asked for, accepting only ids we sent.
	//
	// Keying by id rather than by position is what makes a dropped segment a
	// detectable absence instead of an off-by-one that silently shifts every
	// translation after it — the documented failure mode of batching, and the
	// one an injected instruction in the content would exploit.
	want := make(map[string]int, len(segments))
	for i, seg := range segments {
		want[seg.ID] = i
	}
	translations := make(map[int]string, len(result.Translations))
	for _, tr := range result.Translations {
		i, ok := want[tr.ID]
		if !ok {
			// An id we never sent. Ignore it rather than write it somewhere.
			continue
		}
		translations[i] = tr.Text
	}

	// Apply translations (fall back to individual calls for missing entries).
	for i, entry := range entries {
		text, ok := translations[i]
		if !ok || text == "" {
			if err := t.translate(tool.NewVariantViewWithContext(ctx, entry.block)); err != nil { //nolint:contextcheck // ctx travels inside the VariantView; translate keeps the view-only Produce signature
				return err
			}
			continue
		}

		ev := tool.NewVariantViewWithContext(ctx, entry.block)
		if entry.hasInlineCodes {
			targetRuns := model.ParseRunsPlaceholderText(text, entry.sourceRuns)
			ev.SetTargetRuns(t.targetLocale, targetRuns)
		} else {
			ev.SetTargetText(t.targetLocale, text)
		}
		t.annotateTranslation(ev, &aiprovider.TranslateResponse{
			Translation: text,
			Confidence:  0.85,
			Model:       resp.Model,
		})
		t.blockIndex.Add(1)
		t.emitProgress(true, "")
	}

	return nil
}
