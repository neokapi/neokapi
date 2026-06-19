package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/model"
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
	glossary     map[string]string
	voiceGuide   string // compact brand voice guidance injected into every prompt
	skipMatched  bool
	batchSize    int
	concurrency  int
	onProgress   func(aiprovider.ProgressEvent)
	blockIndex   atomic.Int32
	totalBlocks  int
}

// Default values for AITranslateConfig — used in both the schema tags
// (for UI display) and NewAITranslateTool (for runtime fallback).
const (
	DefaultBatchSize        = 100
	DefaultBatchConcurrency = 1
)

// AITranslateConfig holds configuration for the AI translate tool.
// Fields are exposed as CLI flags via schema tags and as flow config
// via json tags.
type AITranslateConfig struct {
	SourceLocale model.LocaleID    `json:"sourceLocale,omitempty" schema:"-"`
	TargetLocale model.LocaleID    `json:"targetLocale,omitempty" schema:"-"`
	Provider     string            `json:"provider,omitempty"     schema:"title=AI Provider,description=AI provider,default=anthropic,group=provider"`
	APIKey       string            `json:"apiKey,omitempty"       schema:"title=API Key,description=API key for the AI provider,group=provider"`
	Model        string            `json:"model,omitempty"        schema:"title=Model,description=AI model name,group=provider"`
	Glossary     map[string]string `json:"glossary,omitempty"     schema:"-"`
	// Profile is an optional brand voice profile. When set, its guidance is
	// injected into the translation prompt so output is on-brand at generation
	// time. Not serializable via the schema/CLI; supplied programmatically or
	// via the .kapi brand binding.
	Profile          *brand.VoiceProfile `json:"-" schema:"-"`
	SkipMatched      bool                `json:"skipMatched,omitempty"  schema:"title=Skip Matched,description=Skip blocks that already have a target translation"`
	BatchSize        int                 `json:"batchSize,omitempty"    schema:"title=Batch Size,description=Number of blocks per LLM call,default=100,min=1"`
	BatchConcurrency int                 `json:"batchConcurrency,omitempty" schema:"title=Batch Concurrency,description=Number of concurrent batch calls (0 or 1 = sequential),default=1,min=1"`

	// OnProgress is called for each block during translation. It receives
	// live thinking summaries when the provider supports streaming.
	OnProgress func(aiprovider.ProgressEvent) `json:"-"`
}

// AITranslateSchema returns the auto-generated schema for the AI translate tool.
func AITranslateSchema() *schema.ComponentSchema {
	s := schema.FromStruct(&AITranslateConfig{}, schema.ToolMeta{
		ID:                    "ai-translate",
		Category:              schema.CategoryTranslation,
		DisplayName:           "AI Translate",
		Description:           "Translate content using an LLM provider",
		Tags:                  []string{"ai-powered"},
		Aliases:               []string{"translate"},
		WritesOutput:          true,
		DefaultParallelBlocks: 5,
		Requires:              []string{schema.RequiresTargetLanguage, schema.RequiresCredentials},
		Cardinality:           schema.Bilingual,
		Produces:              []schema.IOPort{{Type: schema.PortTarget, Side: model.SideTarget}},
		SideEffects:           []schema.SideEffect{schema.SideEffectAPICall, schema.SideEffectRemoteSourceEgress},
	})
	injectProviderOptions(s)
	return s
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
	var profile *brand.VoiceProfile
	if pf, ok := config["profile"].(*brand.VoiceProfile); ok {
		profile = pf
		delete(config, "profile")
	}

	var cfg AITranslateConfig
	if err := schema.ApplyConfig(config, &cfg); err != nil {
		return nil, fmt.Errorf("ai-translate config: %w", err)
	}
	cfg.OnProgress = onProgress
	cfg.Profile = profile

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
		glossary:     cfg.Glossary,
		voiceGuide:   brand.RenderVoiceGuideCompact(cfg.Profile),
		skipMatched:  cfg.SkipMatched,
		batchSize:    cfg.BatchSize,
		concurrency:  cfg.BatchConcurrency,
		onProgress:   cfg.OnProgress,
	}
	if sp, ok := p.(aiprovider.StreamingLLMProvider); ok {
		t.streaming = sp
	}
	if t.batchSize < 1 {
		t.batchSize = DefaultBatchSize
	}
	if t.concurrency < 1 {
		t.concurrency = DefaultBatchConcurrency
	}
	t.ToolName = "ai-translate"
	t.ToolDescription = "Translates Blocks using AI/LLM"
	// Translate: writes the target locale; source stays read-only. The batched
	// and session paths (Process overrides) reuse translate() via NewTargetView.
	t.Translate = t.translate
	return t
}

// Process overrides BaseTool.Process to support batch + concurrent translation.
// When batchSize > 1 or concurrency > 1, blocks are grouped into batches of
// batchSize and processed with up to concurrency goroutines in parallel.
func (t *AITranslateTool) Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	if t.batchSize <= 1 && t.concurrency <= 1 {
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
	hash := block.ID
	if hash == "" {
		return t.translate(tool.NewTargetViewWithContext(ctx, block))
	}

	// Skip if already cached.
	if randomAccess {
		if sc, err := sess.GetOverlay(overlayKind, hash); err == nil && len(sc.Payload) > 0 {
			var cached aiTargetCache
			if err := json.Unmarshal(sc.Payload, &cached); err == nil && cached.Text != "" {
				block.SetTargetText(t.targetLocale, cached.Text)
				return nil
			}
		}
	}

	if err := t.translate(tool.NewTargetViewWithContext(ctx, block)); err != nil {
		return err
	}

	if target := block.TargetText(t.targetLocale); target != "" {
		payload, err := json.Marshal(aiTargetCache{
			Text:     target,
			Provider: string(t.provider.Name()),
		})
		if err != nil {
			return fmt.Errorf("ai-translate: encode overlay: %w", err)
		}
		if err := sess.PutOverlay(blockstore.Overlay{
			Kind:      overlayKind,
			BlockHash: hash,
			Payload:   payload,
		}); err != nil && !errors.Is(err, blockstore.ErrReadOnly) {
			return fmt.Errorf("ai-translate: write overlay: %w", err)
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
				if caps.RandomAccess {
					if sc, err := sess.GetOverlay(overlayKind, block.ID); err == nil && len(sc.Payload) > 0 {
						var cached aiTargetCache
						if err := json.Unmarshal(sc.Payload, &cached); err == nil && cached.Text != "" {
							block.SetTargetText(t.targetLocale, cached.Text)
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
				})
				if err == nil {
					if werr := sess.PutOverlay(blockstore.Overlay{
						Kind:      overlayKind,
						BlockHash: block.ID,
						Payload:   payload,
					}); werr != nil && !errors.Is(werr, blockstore.ErrReadOnly) {
						return fmt.Errorf("ai-translate: write overlay: %w", werr)
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
// written by ai-translate. Kept small and JSON-compatible with
// other translators so sessions can interop freely.
type aiTargetCache struct {
	Text     string `json:"text"`
	Provider string `json:"provider,omitempty"`
}

// ---------------------------------------------------------------------------
// Single-block processing (existing behavior)
// ---------------------------------------------------------------------------

// translate writes the AI target for one block. Source is read-only (the
// TargetView exposes no source setter). Dispatched directly for the
// block-by-block path; the batched and session paths call it via NewTargetView.
func (t *AITranslateTool) translate(v tool.TargetView) error {
	if !v.Translatable() {
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

	// If the source has inline codes, route through the
	// placeholder-preserving LLM path so the model can keep them
	// intact.
	sourceRuns := v.SourceRuns()
	if hasInlineCodes(sourceRuns) {
		return t.translateWithInlineCodes(v, sourceRuns)
	}

	// Plain text translation.
	resp, err := t.translateBlock(v.Context(), aiprovider.TranslateRequest{
		Source:         sourceText,
		SourceLanguage: t.sourceLocale,
		TargetLocale:   t.targetLocale,
		Glossary:       t.glossary,
		VoiceGuide:     t.voiceGuide,
	})
	if err != nil {
		return fmt.Errorf("ai-translate: %w", err)
	}
	t.addUsage(resp.Usage)

	v.SetTargetText(t.targetLocale, resp.Translation)
	t.annotateTranslation(v, resp)

	t.emitProgress(true, "")
	return nil
}

// translateBlock translates text using streaming when available (for live
// thinking progress), falling back to the standard Translate method.
func (t *AITranslateTool) translateBlock(ctx context.Context, req aiprovider.TranslateRequest) (*aiprovider.TranslateResponse, error) {
	if t.streaming == nil || t.onProgress == nil {
		return t.provider.Translate(ctx, req)
	}

	// Build the same prompt that Translate would, but use ChatStream
	// so we can surface thinking progress.
	var prompt strings.Builder
	prompt.WriteString(fmt.Sprintf(
		"Translate the following text from %s to %s. Return ONLY the translation, no explanation.\n\nText: %s",
		req.SourceLanguage, req.TargetLocale, req.Source,
	))
	prompt.WriteString(req.Directives())

	resp, err := t.streaming.ChatStream(ctx, []aiprovider.Message{
		aiprovider.TextMessage("user", prompt.String()),
	}, func(e aiprovider.ChatStreamEvent) {
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
	}, nil
}

// handleBlockWithInlineCodes translates a block that contains
// inline codes. Renders source runs as placeholder-tagged text so
// the LLM can preserve tag positions, then reconstructs the target
// Run sequence from the response via ParseRunsPlaceholderText.
func (t *AITranslateTool) translateWithInlineCodes(v tool.TargetView, sourceRuns []model.Run) error {
	sourceText := model.RunsPlaceholderText(sourceRuns)

	prompt := fmt.Sprintf(
		"Translate the following text from %s to %s. Preserve all XML tags exactly as they appear (do not modify, add, or remove any tags). Return only the translated text with tags.\n\n%s",
		t.sourceLocale, t.targetLocale, sourceText,
	)

	resp, err := t.translateBlock(v.Context(), aiprovider.TranslateRequest{
		Source:         prompt,
		SourceLanguage: t.sourceLocale,
		TargetLocale:   t.targetLocale,
		Glossary:       t.glossary,
		VoiceGuide:     t.voiceGuide,
	})
	if err != nil {
		return fmt.Errorf("ai-translate: %w", err)
	}
	t.addUsage(resp.Usage)

	targetRuns := model.ParseRunsPlaceholderText(resp.Translation, sourceRuns)
	v.SetTargetRuns(t.targetLocale, targetRuns)
	t.annotateTranslation(v, resp)

	t.emitProgress(true, "")
	return nil
}

// hasInlineCodes reports whether a Run sequence contains any
// non-text run.
func hasInlineCodes(runs []model.Run) bool {
	for _, r := range runs {
		if r.Text == nil {
			return true
		}
	}
	return false
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

func (t *AITranslateTool) annotateTranslation(v tool.TargetView, resp *aiprovider.TranslateResponse) {
	v.AddAltTranslation(&model.AltTranslation{
		Target:    []model.Run{{Text: &model.TextRun{Text: resp.Translation}}},
		Locale:    t.targetLocale,
		Origin:    "ai:" + string(t.provider.Name()),
		Score:     resp.Confidence,
		MatchType: model.MatchAI,
	})
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
		if t.skipMatched && block.HasTarget(t.targetLocale) {
			continue
		}
		src := block.SourceText()
		if src == "" {
			continue
		}
		sourceRuns := block.SourceRuns()
		inline := hasInlineCodes(sourceRuns)
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

	// 3. Group into batches and translate them with bounded concurrency.
	batches := chunkBlocks(entries, t.batchSize)
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

// batchTranslationSchema returns a JSON schema for structured batch translation output.
func batchTranslationSchema() aiprovider.JSONSchema {
	return aiprovider.JSONSchema{
		Name:        "batch_translations",
		Description: "Batch translation results with index-text pairs",
		Strict:      true,
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"translations": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"index": map[string]any{"type": "integer"},
							"text":  map[string]any{"type": "string"},
						},
						"required":             []string{"index", "text"},
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
		Index int    `json:"index"`
		Text  string `json:"text"`
	} `json:"translations"`
}

// translateBatch translates a batch of blocks in a single LLM call using
// structured output. The response is constrained to a JSON schema with
// index-text pairs, eliminating text parsing ambiguity.
// Falls back to individual translation for any missing entries.
func (t *AITranslateTool) translateBatch(ctx context.Context, entries []blockEntry) error {
	if len(entries) == 1 {
		return t.translate(tool.NewTargetViewWithContext(ctx, entries[0].block))
	}

	// Build numbered prompt.
	var prompt strings.Builder
	fmt.Fprintf(&prompt,
		"Translate each numbered segment from %s to %s.\n"+
			"Preserve any XML/HTML tags exactly as they appear.",
		t.sourceLocale, t.targetLocale,
	)
	// Inject deterministic brand-voice + glossary directives.
	prompt.WriteString(aiprovider.TranslateRequest{
		Glossary:   t.glossary,
		VoiceGuide: t.voiceGuide,
	}.Directives())
	prompt.WriteString("\n\n")

	for i, entry := range entries {
		fmt.Fprintf(&prompt, "[%d] %s\n", i+1, entry.sourceText)
	}

	messages := []aiprovider.Message{aiprovider.TextMessage("user", prompt.String())}
	schema := batchTranslationSchema()

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
		return fmt.Errorf("ai-translate batch: %w", err)
	}
	t.addUsage(resp.Usage)

	// Parse structured JSON response.
	var result batchResult
	if err := json.Unmarshal([]byte(resp.Content), &result); err != nil {
		return fmt.Errorf("ai-translate batch: unmarshal response: %w", err)
	}

	// Build index → text map from the structured response.
	translations := make(map[int]string, len(result.Translations))
	for _, tr := range result.Translations {
		translations[tr.Index] = tr.Text
	}

	// Apply translations (fall back to individual calls for missing entries).
	for i, entry := range entries {
		text, ok := translations[i+1]
		if !ok || text == "" {
			if err := t.translate(tool.NewTargetViewWithContext(ctx, entry.block)); err != nil {
				return err
			}
			continue
		}

		ev := tool.NewTargetViewWithContext(ctx, entry.block)
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
