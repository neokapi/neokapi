package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/providers/ai"
)

// AITranslateTool translates untranslated Blocks using an LLM provider.
type AITranslateTool struct {
	tool.BaseTool
	provider     provider.LLMProvider
	sourceLocale model.LocaleID
	targetLocale model.LocaleID
	glossary     map[string]string
	skipMatched  bool
	batchSize    int
	concurrency  int
}

// AITranslateConfig holds configuration for the AI translate tool.
type AITranslateConfig struct {
	SourceLocale model.LocaleID
	TargetLocale model.LocaleID
	Glossary     map[string]string
	SkipMatched  bool
	BatchSize    int // Blocks per LLM call. 0 or 1 = one block per call.
	Concurrency  int // Concurrent batch calls. 0 or 1 = sequential.
}

// NewAITranslateTool creates a new AI translation tool.
func NewAITranslateTool(p provider.LLMProvider, cfg AITranslateConfig) *AITranslateTool {
	t := &AITranslateTool{
		provider:     p,
		sourceLocale: cfg.SourceLocale,
		targetLocale: cfg.TargetLocale,
		glossary:     cfg.Glossary,
		skipMatched:  cfg.SkipMatched,
		batchSize:    cfg.BatchSize,
		concurrency:  cfg.Concurrency,
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

	// Check if the source fragment has inline spans.
	frag := block.FirstFragment()
	if frag != nil && frag.HasSpans() {
		return t.handleBlockWithSpans(part, block, frag)
	}

	// Plain text translation.
	resp, err := t.provider.Translate(context.Background(), provider.TranslateRequest{
		Source:       sourceText,
		SourceLanguage: t.sourceLocale,
		TargetLocale: t.targetLocale,
		Glossary:     t.glossary,
	})
	if err != nil {
		return nil, fmt.Errorf("ai-translate: %w", err)
	}

	block.SetTargetText(t.targetLocale, resp.Translation)
	t.annotateTranslation(block, resp)
	return part, nil
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

	resp, err := t.provider.Translate(context.Background(), provider.TranslateRequest{
		Source:       prompt,
		SourceLanguage: t.sourceLocale,
		TargetLocale: t.targetLocale,
		Glossary:     t.glossary,
	})
	if err != nil {
		return nil, fmt.Errorf("ai-translate: %w", err)
	}

	// Reconstruct Fragment from the LLM response.
	targetFrag := model.ParsePlaceholderText(resp.Translation, frag.Spans)
	block.SetTargetFragment(t.targetLocale, targetFrag)
	t.annotateTranslation(block, resp)
	return part, nil
}

func (t *AITranslateTool) annotateTranslation(block *model.Block, resp *provider.TranslateResponse) {
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

		wg.Add(1)
		go func(batch []blockEntry) {
			defer func() { <-sem; wg.Done() }()
			if err := t.translateBatch(ctx, batch); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
			}
		}(batch)
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
func batchTranslationSchema() provider.JSONSchema {
	return provider.JSONSchema{
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

	resp, err := t.provider.ChatStructured(ctx, []provider.Message{
		{Role: "user", Content: prompt.String()},
	}, batchTranslationSchema())
	if err != nil {
		return fmt.Errorf("ai-translate batch: %w", err)
	}

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
		t.annotateTranslation(entry.block, &provider.TranslateResponse{
			Translation: text,
			Confidence:  0.85,
			Model:       resp.Model,
		})
	}

	return nil
}
