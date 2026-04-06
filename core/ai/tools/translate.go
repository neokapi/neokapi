package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/providers/ai"
)

// AITranslateTool translates untranslated Blocks using an LLM provider.
type AITranslateTool struct {
	tool.BaseTool
	usageAccumulator
	provider     aiprovider.LLMProvider
	streaming    aiprovider.StreamingLLMProvider // nil when provider doesn't support streaming
	sourceLocale model.LocaleID
	targetLocale model.LocaleID
	glossary     map[string]string
	skipMatched  bool
	batchSize    int
	concurrency  int
	onProgress   func(aiprovider.ProgressEvent)
	blockIndex   atomic.Int32
	totalBlocks  int
}

// AITranslateConfig holds configuration for the AI translate tool.
// Fields are exposed as CLI flags via schema tags and as flow config
// via json tags.
type AITranslateConfig struct {
	SourceLocale     model.LocaleID    `json:"sourceLocale,omitempty" schema:"-"`
	TargetLocale     model.LocaleID    `json:"targetLocale,omitempty" schema:"-"`
	Provider         string            `json:"provider,omitempty"     schema:"title=AI Provider,description=AI provider,default=anthropic,enum=anthropic|openai|gemini|ollama,group=provider"`
	APIKey           string            `json:"apiKey,omitempty"       schema:"title=API Key,description=API key for the AI provider,group=provider"`
	Model            string            `json:"model,omitempty"        schema:"title=Model,description=AI model name,group=provider"`
	Glossary         map[string]string `json:"glossary,omitempty"     schema:"-"`
	SkipMatched      bool              `json:"skipMatched,omitempty"  schema:"title=Skip Matched,description=Skip blocks that already have a target translation"`
	BatchSize        int               `json:"batchSize,omitempty"    schema:"title=Batch Size,description=Number of blocks per LLM call,default=100,min=1"`
	BatchConcurrency int               `json:"batchConcurrency,omitempty" schema:"title=Batch Concurrency,description=Number of concurrent batch calls (0 or 1 = sequential),default=1,min=1"`

	// OnProgress is called for each block during translation. It receives
	// live thinking summaries when the provider supports streaming.
	OnProgress func(aiprovider.ProgressEvent) `json:"-"`
}

// AITranslateSchema returns the auto-generated schema for the AI translate tool.
func AITranslateSchema() *schema.ComponentSchema {
	return schema.FromStruct(&AITranslateConfig{}, schema.ToolMeta{
		ID:          "ai-translate",
		Category:    schema.CategoryTranslate,
		DisplayName: "AI Translate",
		Description: "Translate content using an LLM provider",
		Inputs:      []string{schema.PartTypeBlock},
		Tags:        []string{"ai-powered"},
		Requires:    []string{schema.RequiresTargetLanguage, schema.RequiresCredentials},
	})
}

// ProviderFromConfig creates an LLM provider from a provider name and config.
func ProviderFromConfig(name string, cfg aiprovider.Config) (aiprovider.LLMProvider, error) {
	if name == "" {
		name = "anthropic"
	}

	switch name {
	case "anthropic":
		return aiprovider.NewAnthropicProvider(cfg), nil
	case "openai":
		return aiprovider.NewOpenAIProvider(cfg), nil
	case "gemini":
		return aiprovider.NewGeminiProvider(cfg), nil
	case "ollama":
		return aiprovider.NewOllamaProvider(cfg), nil
	default:
		return nil, fmt.Errorf("unknown AI provider: %s (supported: anthropic, openai, gemini, ollama)", name)
	}
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

	var cfg AITranslateConfig
	if err := schema.ApplyConfig(config, &cfg); err != nil {
		return nil, fmt.Errorf("ai-translate config: %w", err)
	}
	cfg.OnProgress = onProgress

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
		skipMatched:  cfg.SkipMatched,
		batchSize:    cfg.BatchSize,
		concurrency:  cfg.BatchConcurrency,
		onProgress:   cfg.OnProgress,
	}
	if sp, ok := p.(aiprovider.StreamingLLMProvider); ok {
		t.streaming = sp
	}
	if t.batchSize < 1 {
		t.batchSize = 1
	}
	if t.concurrency < 1 {
		t.concurrency = 1
	}
	t.ToolName = "ai-translate"
	t.ToolDescription = "Translates Blocks using AI/LLM"
	t.HandleBlockFn = t.handleBlock
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

// ---------------------------------------------------------------------------
// Single-block processing (existing behavior)
// ---------------------------------------------------------------------------

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

	t.blockIndex.Add(1)

	// Check if the source fragment has inline spans.
	frag := block.FirstFragment()
	if frag != nil && frag.HasSpans() {
		return t.handleBlockWithSpans(part, block, frag)
	}

	// Plain text translation.
	resp, err := t.translateBlock(context.Background(), aiprovider.TranslateRequest{
		Source:         sourceText,
		SourceLanguage: t.sourceLocale,
		TargetLocale:   t.targetLocale,
		Glossary:       t.glossary,
	})
	if err != nil {
		return nil, fmt.Errorf("ai-translate: %w", err)
	}
	t.addUsage(resp.Usage)

	block.SetTargetText(t.targetLocale, resp.Translation)
	t.annotateTranslation(block, resp)

	t.emitProgress(true, "")
	return part, nil
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

	if len(req.Glossary) > 0 {
		prompt.WriteString("\n\nGlossary:\n")
		for term, translation := range req.Glossary {
			prompt.WriteString(fmt.Sprintf("- %s → %s\n", term, translation))
		}
	}

	resp, err := t.streaming.ChatStream(ctx, []aiprovider.Message{
		{Role: "user", Content: prompt.String()},
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

// handleBlockWithSpans translates a block that contains inline spans.
// Uses PlaceholderText to preserve span structure through the LLM.
func (t *AITranslateTool) handleBlockWithSpans(part *model.Part, block *model.Block, frag *model.Fragment) (*model.Part, error) {
	// Use placeholder text so the LLM can preserve tag positions.
	sourceText := frag.PlaceholderText()

	prompt := fmt.Sprintf(
		"Translate the following text from %s to %s. Preserve all XML tags exactly as they appear (do not modify, add, or remove any tags). Return only the translated text with tags.\n\n%s",
		t.sourceLocale, t.targetLocale, sourceText,
	)

	resp, err := t.translateBlock(context.Background(), aiprovider.TranslateRequest{
		Source:         prompt,
		SourceLanguage: t.sourceLocale,
		TargetLocale:   t.targetLocale,
		Glossary:       t.glossary,
	})
	if err != nil {
		return nil, fmt.Errorf("ai-translate: %w", err)
	}
	t.addUsage(resp.Usage)

	// Reconstruct Fragment from the LLM response.
	targetFrag := model.ParsePlaceholderText(resp.Translation, frag.Spans)
	block.SetTargetFragment(t.targetLocale, targetFrag)
	t.annotateTranslation(block, resp)

	t.emitProgress(true, "")
	return part, nil
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

func (t *AITranslateTool) annotateTranslation(block *model.Block, resp *aiprovider.TranslateResponse) {
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
}

// ---------------------------------------------------------------------------
// Batch + concurrent processing
// ---------------------------------------------------------------------------

// blockEntry tracks a translatable block within the batch pipeline.
type blockEntry struct {
	index      int             // position in the original parts slice
	part       *model.Part     // the Part containing the block
	block      *model.Block    // the Block resource
	sourceText string          // text to send to the LLM
	hasSpans   bool            // whether the source has inline spans
	frag       *model.Fragment // source fragment (for span reconstruction)
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
		frag := block.FirstFragment()
		hasSpans := frag != nil && frag.HasSpans()
		text := src
		if hasSpans {
			text = frag.PlaceholderText()
		}
		entries = append(entries, blockEntry{
			index: i, part: part, block: block,
			sourceText: text, hasSpans: hasSpans, frag: frag,
		})
	}

	// Set total for progress reporting since we know the full count.
	t.totalBlocks = len(entries)

	// 3. Group into batches.
	batches := make([][]blockEntry, 0, (len(entries)+t.batchSize-1)/t.batchSize)
	for i := 0; i < len(entries); i += t.batchSize {
		end := i + t.batchSize
		if end > len(entries) {
			end = len(entries)
		}
		batches = append(batches, entries[i:end])
	}

	// 4. Process batches concurrently.
	var (
		mu       sync.Mutex
		firstErr error
		wg       sync.WaitGroup
	)
	sem := make(chan struct{}, t.concurrency)

	for _, batch := range batches {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case sem <- struct{}{}:
		}

		wg.Go(func() {
			defer func() { <-sem }()
			if err := t.translateBatch(ctx, batch); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
			}
		})
	}
	wg.Wait()

	if firstErr != nil {
		return firstErr
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
		_, err := t.handleBlock(entries[0].part)
		return err
	}

	// Build numbered prompt.
	var prompt strings.Builder
	fmt.Fprintf(&prompt,
		"Translate each numbered segment from %s to %s.\n"+
			"Preserve any XML/HTML tags exactly as they appear.\n\n",
		t.sourceLocale, t.targetLocale,
	)

	if len(t.glossary) > 0 {
		prompt.WriteString("Glossary:\n")
		for term, translation := range t.glossary {
			fmt.Fprintf(&prompt, "- %s → %s\n", term, translation)
		}
		prompt.WriteByte('\n')
	}

	for i, entry := range entries {
		fmt.Fprintf(&prompt, "[%d] %s\n", i+1, entry.sourceText)
	}

	messages := []aiprovider.Message{{Role: "user", Content: prompt.String()}}
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
			if _, err := t.handleBlock(entry.part); err != nil {
				return err
			}
			continue
		}

		if entry.hasSpans {
			targetFrag := model.ParsePlaceholderText(text, entry.frag.Spans)
			entry.block.SetTargetFragment(t.targetLocale, targetFrag)
		} else {
			entry.block.SetTargetText(t.targetLocale, text)
		}
		t.annotateTranslation(entry.block, &aiprovider.TranslateResponse{
			Translation: text,
			Confidence:  0.85,
			Model:       resp.Model,
		})
		t.blockIndex.Add(1)
		t.emitProgress(true, "")
	}

	return nil
}
