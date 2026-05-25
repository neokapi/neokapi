package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/neokapi/neokapi/core/ai/ner"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/providers/ai"
)

// AIEntityExtractTool extracts named entities and term candidates from Blocks
// using an LLM provider (via ChatStructured) and an optional NER provider for
// fast entity detection. Follows the hybrid extraction approach from Bowrain AD-015.
type AIEntityExtractTool struct {
	tool.BaseTool
	usageAccumulator
	llm         aiprovider.LLMProvider
	nerProvider ner.Provider // optional; nil means LLM-only
	locale      model.LocaleID
	knownTerms  map[string]bool // terms already in termbase (skip during extraction)
	batchSize   int
	concurrency int
}

// AIEntityExtractConfig holds configuration for the entity extraction tool.
type AIEntityExtractConfig struct {
	Provider    string         `json:"provider,omitempty" schema:"title=AI Provider,description=AI provider,default=anthropic,group=provider"`
	APIKey      string         `json:"apiKey,omitempty" schema:"title=API Key,description=API key for the AI provider,group=provider"`
	Model       string         `json:"model,omitempty" schema:"title=Model,description=AI model name,group=provider"`
	Locale      model.LocaleID `json:"locale,omitempty" schema:"description=Locale of the source content"`
	KnownTerms  []string       `json:"knownTerms,omitempty" schema:"description=Terms to exclude from extraction (already in termbase)"`                       // terms to exclude from extraction (already in termbase)
	BatchSize   int            `json:"batchSize,omitempty" schema:"description=Number of blocks per LLM call (0 or 1 = one block per call),default=1,min=1"`   // Blocks per LLM call. 0 or 1 = one block per call.
	Concurrency int            `json:"batchConcurrency,omitempty" schema:"description=Number of concurrent batch calls (0 or 1 = sequential),default=1,min=1"` // Concurrent batch calls. 0 or 1 = sequential.
}

// AIEntityExtractSchema returns the auto-generated schema for the
// ai-entity-extract tool.
func AIEntityExtractSchema() *schema.ComponentSchema {
	s := schema.FromStruct(&AIEntityExtractConfig{}, schema.ToolMeta{
		ID:          "ai-entity-extract",
		Category:    schema.CategoryAnalysis,
		DisplayName: "AI Entity Extract",
		Description: "Detect named entities (people, organizations, products, locations) using an LLM",
		Inputs:      []string{schema.PartTypeBlock},
		Tags:        []string{"ai-powered"},
		Requires:    []string{schema.RequiresCredentials},
		Cardinality: schema.Monolingual,
		SideEffects: []schema.SideEffect{schema.SideEffectAPICall},
	})
	injectProviderOptions(s)
	return s
}

// NewAIEntityExtractFromConfig creates an entity-extraction tool from a config
// map, resolving the LLM provider the same way ai-translate does.
func NewAIEntityExtractFromConfig(config map[string]any, _ string) (tool.Tool, error) {
	var cfg AIEntityExtractConfig
	if err := schema.ApplyConfig(config, &cfg); err != nil {
		return nil, fmt.Errorf("ai-entity-extract config: %w", err)
	}
	p, err := ProviderFromConfig(cfg.Provider, aiprovider.Config{APIKey: cfg.APIKey, Model: cfg.Model})
	if err != nil {
		return nil, err
	}
	return NewAIEntityExtractTool(p, nil, cfg), nil
}

// NewAIEntityExtractTool creates a new entity/term extraction tool.
// The nerProvider parameter is optional — pass nil for LLM-only extraction.
func NewAIEntityExtractTool(llm aiprovider.LLMProvider, nerProvider ner.Provider, cfg AIEntityExtractConfig) *AIEntityExtractTool {
	known := make(map[string]bool, len(cfg.KnownTerms))
	for _, t := range cfg.KnownTerms {
		known[strings.ToLower(t)] = true
	}
	et := &AIEntityExtractTool{
		llm:         llm,
		nerProvider: nerProvider,
		locale:      cfg.Locale,
		knownTerms:  known,
		batchSize:   cfg.BatchSize,
		concurrency: cfg.Concurrency,
	}
	if et.batchSize < 1 {
		et.batchSize = 1
	}
	if et.concurrency < 1 {
		et.concurrency = 1
	}
	et.ToolName = "ai-entity-extract"
	et.ToolDescription = "Extracts named entities and term candidates using AI/LLM + optional NER"
	et.HandleBlockFn = et.handleBlock
	return et
}

// Process overrides BaseTool.Process to support batch + concurrent extraction.
func (t *AIEntityExtractTool) Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	if t.batchSize <= 1 && t.concurrency <= 1 {
		return t.BaseTool.Process(ctx, in, out)
	}
	return t.processBatched(ctx, in, out)
}

// ---------------------------------------------------------------------------
// Single-block processing
// ---------------------------------------------------------------------------

func (t *AIEntityExtractTool) handleBlock(part *model.Part) (*model.Part, error) {
	block, ok := part.Resource.(*model.Block)
	if !ok {
		return part, nil
	}

	sourceText := block.SourceText()
	if strings.TrimSpace(sourceText) == "" {
		return part, nil
	}

	// Run NER if available.
	var nerEntities []ner.DetectedEntity
	if t.nerProvider != nil {
		resp, err := t.nerProvider.DetectEntities(context.Background(), ner.Request{
			Text:   sourceText,
			Locale: t.locale,
		})
		if err == nil && resp != nil {
			nerEntities = resp.Entities
		}
	}

	// Run LLM extraction.
	llmResult, err := t.extractWithLLM(context.Background(), []extractionEntry{
		{blockID: block.ID, text: sourceText},
	})
	if err != nil {
		return nil, fmt.Errorf("ai-entity-extract: %w", err)
	}

	// Merge and attach annotations.
	t.attachAnnotations(block, nerEntities, llmResult)

	return part, nil
}

// ---------------------------------------------------------------------------
// Batch + concurrent processing
// ---------------------------------------------------------------------------

type extractionEntry struct {
	index   int
	part    *model.Part
	block   *model.Block
	blockID string
	text    string
}

func (t *AIEntityExtractTool) processBatched(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	// 1. Drain input.
	var parts []*model.Part
	for part := range in {
		parts = append(parts, part)
	}

	// 2. Identify blocks with text.
	var entries []extractionEntry
	for i, part := range parts {
		if part.Type != model.PartBlock {
			continue
		}
		block, ok := part.Resource.(*model.Block)
		if !ok {
			continue
		}
		text := block.SourceText()
		if strings.TrimSpace(text) == "" {
			continue
		}
		entries = append(entries, extractionEntry{
			index: i, part: part, block: block,
			blockID: block.ID, text: text,
		})
	}

	// 3. Run NER batch (if available).
	nerResults := make(map[int][]ner.DetectedEntity)
	if t.nerProvider != nil {
		reqs := make([]ner.Request, len(entries))
		for i, e := range entries {
			reqs[i] = ner.Request{Text: e.text, Locale: t.locale}
		}
		responses, err := t.nerProvider.DetectEntitiesBatch(ctx, reqs)
		if err == nil {
			for i, resp := range responses {
				nerResults[i] = resp.Entities
			}
		}
	}

	// 4. LLM batch extraction.
	batches := make([][]extractionEntry, 0, (len(entries)+t.batchSize-1)/t.batchSize)
	for i := 0; i < len(entries); i += t.batchSize {
		end := i + t.batchSize
		if end > len(entries) {
			end = len(entries)
		}
		batches = append(batches, entries[i:end])
	}

	type batchLLMResult struct {
		startIdx int
		result   *llmExtractionResult
		err      error
	}

	var (
		mu         sync.Mutex
		llmResults []batchLLMResult
		wg         sync.WaitGroup
	)
	sem := make(chan struct{}, t.concurrency)

	batchOffset := 0
	for _, batch := range batches {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case sem <- struct{}{}:
		}

		offset := batchOffset
		wg.Go(func() {
			defer func() { <-sem }()
			llmEntries := make([]extractionEntry, len(batch))
			copy(llmEntries, batch)
			result, err := t.extractWithLLM(ctx, llmEntries)
			mu.Lock()
			llmResults = append(llmResults, batchLLMResult{startIdx: offset, result: result, err: err})
			mu.Unlock()
		})
		batchOffset += len(batch)
	}
	wg.Wait()

	// Check for errors.
	for _, r := range llmResults {
		if r.err != nil {
			return r.err
		}
	}

	// 5. Merge LLM results by entry index and attach annotations.
	llmByEntry := make(map[int]*llmExtractionResult)
	for _, r := range llmResults {
		if r.result == nil {
			continue
		}
		// The LLM result covers entries at indices [startIdx, startIdx+len(batch)).
		// Each block result is keyed by block_id, so we map back.
		llmByEntry[r.startIdx] = r.result
	}

	entryOffset := 0
	for batchIdx, batch := range batches {
		_ = batchIdx
		for i, entry := range batch {
			globalIdx := entryOffset + i
			var nerEnts []ner.DetectedEntity
			if ents, ok := nerResults[globalIdx]; ok {
				nerEnts = ents
			}
			// Find LLM result for this batch.
			var llmResult *llmExtractionResult
			if r, ok := llmByEntry[entryOffset]; ok {
				llmResult = r
			}
			t.attachAnnotationsFromBatch(entry.block, nerEnts, llmResult, entry.blockID)
		}
		entryOffset += len(batch)
	}

	// 6. Write all parts in original order.
	for _, part := range parts {
		select {
		case out <- part:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return nil
}

// ---------------------------------------------------------------------------
// LLM extraction
// ---------------------------------------------------------------------------

// llmExtractionResult is the parsed JSON from the LLM.
type llmExtractionResult struct {
	Blocks []llmBlockResult `json:"blocks"`
}

type llmBlockResult struct {
	BlockID        string             `json:"block_id"`
	Entities       []llmEntity        `json:"entities"`
	TermCandidates []llmTermCandidate `json:"term_candidates"`
}

type llmEntity struct {
	Text       string  `json:"text"`
	Type       string  `json:"type"`
	DNT        bool    `json:"dnt"`
	Offset     int     `json:"offset"`
	Length     int     `json:"length"`
	Confidence float64 `json:"confidence"`
}

type llmTermCandidate struct {
	Text            string  `json:"text"`
	Definition      string  `json:"definition"`
	Category        string  `json:"category"`
	Translatability string  `json:"translatability"`
	Confidence      float64 `json:"confidence"`
	Offset          int     `json:"offset"`
	Length          int     `json:"length"`
}

func extractionSchema() aiprovider.JSONSchema {
	return aiprovider.JSONSchema{
		Name:        "extraction_result",
		Description: "Named entities and terminology candidates extracted from localization content",
		Strict:      true,
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"blocks": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"block_id": map[string]any{"type": "string"},
							"entities": map[string]any{
								"type": "array",
								"items": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"text":       map[string]any{"type": "string"},
										"type":       map[string]any{"type": "string", "enum": []string{"person", "organization", "product", "location", "date", "time", "currency", "measurement", "other"}},
										"dnt":        map[string]any{"type": "boolean"},
										"offset":     map[string]any{"type": "integer"},
										"length":     map[string]any{"type": "integer"},
										"confidence": map[string]any{"type": "number"},
									},
									"required":             []string{"text", "type", "dnt", "offset", "length", "confidence"},
									"additionalProperties": false,
								},
							},
							"term_candidates": map[string]any{
								"type": "array",
								"items": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"text":            map[string]any{"type": "string"},
										"definition":      map[string]any{"type": "string"},
										"category":        map[string]any{"type": "string", "enum": []string{"brand", "technical", "ui", "legal", "marketing", "general"}},
										"translatability": map[string]any{"type": "string", "enum": []string{"dnt", "consistent", "free"}},
										"confidence":      map[string]any{"type": "number"},
										"offset":          map[string]any{"type": "integer"},
										"length":          map[string]any{"type": "integer"},
									},
									"required":             []string{"text", "definition", "category", "translatability", "confidence", "offset", "length"},
									"additionalProperties": false,
								},
							},
						},
						"required":             []string{"block_id", "entities", "term_candidates"},
						"additionalProperties": false,
					},
				},
			},
			"required":             []string{"blocks"},
			"additionalProperties": false,
		},
	}
}

const extractionSystemPrompt = `You are a localization specialist analyzing source content for a translation project.

Given text blocks, identify:

1. Named entities: people, organizations, products, locations, dates, times, currencies, measurements. For each, indicate whether it should be marked do-not-translate (DNT).
   - Person names: usually DNT unless the project localizes names
   - Brand/product names: usually DNT
   - Dates/times/currencies/measurements: usually NOT DNT (they need locale-specific formatting)
   - Locations: context-dependent

2. Terminology candidates: domain-specific terms that should be translated consistently across the project. These are words/phrases that carry specific meaning in this context and would benefit from a termbase entry. Exclude common words.
   - "dnt" = never translate (brand names, acronyms that stay in source language)
   - "consistent" = translate, but the same way everywhere
   - "free" = translate naturally, no consistency requirement

Report character offsets relative to each block's text. Only report genuinely useful entities and terms — quality over quantity.`

func (t *AIEntityExtractTool) extractWithLLM(ctx context.Context, entries []extractionEntry) (*llmExtractionResult, error) {
	var prompt strings.Builder

	fmt.Fprintf(&prompt, "Analyze these %d text blocks from a %s localization project:\n\n", len(entries), t.locale)

	for _, entry := range entries {
		fmt.Fprintf(&prompt, "Block (id: %s):\n\"%s\"\n\n", entry.blockID, entry.text)
	}

	if len(t.knownTerms) > 0 {
		prompt.WriteString("Existing terms (do not re-propose):")
		for term := range t.knownTerms {
			fmt.Fprintf(&prompt, " %s,", term)
		}
		prompt.WriteByte('\n')
	}

	resp, err := t.llm.ChatStructured(ctx, []aiprovider.Message{
		{Role: "system", Content: extractionSystemPrompt},
		{Role: "user", Content: prompt.String()},
	}, extractionSchema())
	if err != nil {
		return nil, err
	}
	t.addUsage(resp.Usage)

	var result llmExtractionResult
	if err := json.Unmarshal([]byte(resp.Content), &result); err != nil {
		return nil, fmt.Errorf("unmarshal extraction result: %w", err)
	}

	return &result, nil
}

// ---------------------------------------------------------------------------
// Annotation attachment
// ---------------------------------------------------------------------------

func (t *AIEntityExtractTool) attachAnnotations(block *model.Block, nerEntities []ner.DetectedEntity, llmResult *llmExtractionResult) {
	// Find the LLM block result matching this block.
	var blockResult *llmBlockResult
	if llmResult != nil {
		for i := range llmResult.Blocks {
			if llmResult.Blocks[i].BlockID == block.ID {
				blockResult = &llmResult.Blocks[i]
				break
			}
		}
	}
	t.mergeAndAttach(block, nerEntities, blockResult)
}

func (t *AIEntityExtractTool) attachAnnotationsFromBatch(block *model.Block, nerEntities []ner.DetectedEntity, llmResult *llmExtractionResult, blockID string) {
	var blockResult *llmBlockResult
	if llmResult != nil {
		for i := range llmResult.Blocks {
			if llmResult.Blocks[i].BlockID == blockID {
				blockResult = &llmResult.Blocks[i]
				break
			}
		}
	}
	t.mergeAndAttach(block, nerEntities, blockResult)
}

func (t *AIEntityExtractTool) mergeAndAttach(block *model.Block, nerEntities []ner.DetectedEntity, llmBlock *llmBlockResult) {
	if block.Annotations == nil {
		block.Annotations = make(map[string]model.Annotation)
	}

	// Track entity positions to reconcile NER + LLM.
	type entityKey struct {
		offset int
		length int
	}
	seen := make(map[entityKey]bool)

	entityIdx := 0

	// Add LLM entities first (preferred classification).
	if llmBlock != nil {
		for _, e := range llmBlock.Entities {
			key := entityKey{e.Offset, e.Length}
			seen[key] = true
			ann := &model.EntityAnnotation{
				Text:     e.Text,
				Type:     llmEntityType(e.Type),
				Position: model.RunRangeForBytes(block.Source, e.Offset, e.Offset+e.Length),
				Locale:   t.locale,
				DNT:      e.DNT,
				Source:   model.ExtractionSourceLLM,
			}
			block.Annotations[fmt.Sprintf("entity:%d", entityIdx)] = ann
			entityIdx++
		}
	}

	// Add NER entities that don't overlap with LLM entities.
	for _, e := range nerEntities {
		key := entityKey{e.Offset, e.Length}
		if seen[key] {
			continue
		}
		ann := &model.EntityAnnotation{
			Text:     e.Text,
			Type:     e.Type,
			Position: model.RunRangeForBytes(block.Source, e.Offset, e.Offset+e.Length),
			Locale:   t.locale,
			DNT:      isDefaultDNT(e.Type),
			Source:   model.ExtractionSourceNER,
		}
		block.Annotations[fmt.Sprintf("entity:%d", entityIdx)] = ann
		entityIdx++
	}

	// Add term candidates (LLM only — NER doesn't produce these).
	if llmBlock != nil {
		termIdx := 0
		for _, tc := range llmBlock.TermCandidates {
			if t.knownTerms[strings.ToLower(tc.Text)] {
				continue
			}
			ann := &model.TermCandidateAnnotation{
				Text:            tc.Text,
				Definition:      tc.Definition,
				Category:        model.TermCategory(tc.Category),
				Translatability: model.Translatability(tc.Translatability),
				Confidence:      tc.Confidence,
				Position:        model.RunRangeForBytes(block.Source, tc.Offset, tc.Offset+tc.Length),
				Locale:          t.locale,
				Source:          model.ExtractionSourceLLM,
				Status:          model.CandidateStatusPending,
			}
			block.Annotations[fmt.Sprintf("term-candidate:%d", termIdx)] = ann
			termIdx++
		}
	}
}

// llmEntityType converts the LLM's entity type string to model.EntityType.
func llmEntityType(typeStr string) model.EntityType {
	switch typeStr {
	case "person":
		return model.EntityPerson
	case "organization":
		return model.EntityOrganization
	case "product":
		return model.EntityProduct
	case "location":
		return model.EntityLocation
	case "date":
		return model.EntityDate
	case "time":
		return model.EntityTime
	case "currency":
		return model.EntityCurrency
	case "measurement":
		return model.EntityMeasurement
	default:
		return model.EntityOther
	}
}

// isDefaultDNT returns the default DNT setting for obvious entity types.
func isDefaultDNT(entityType model.EntityType) bool {
	switch entityType {
	case model.EntityPerson, model.EntityOrganization, model.EntityProduct:
		return true
	default:
		return false
	}
}
