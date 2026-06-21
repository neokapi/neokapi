package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/ai/ner"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/schema"
	"github.com/neokapi/neokapi/core/tool"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
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

// Extraction engines for AIEntityExtractConfig.Engine.
const (
	// EngineLLM extracts with the configured LLM provider (the default).
	EngineLLM = "llm"
	// EngineNER extracts with the local on-device NER model only — no LLM
	// call, no credentials, and no content leaves the machine. Requires a
	// registered local NER provider (ner.LocalProvider): the browser build's
	// JS-bridged GLiNER model, or a native local-model provider.
	EngineNER = "ner"
	// EngineHybrid runs the local NER model AND the LLM, merging results.
	EngineHybrid = "hybrid"
)

// AIEntityExtractConfig holds configuration for the entity extraction tool.
type AIEntityExtractConfig struct {
	Engine      string         `json:"engine,omitempty" schema:"title=Extraction Engine,description=llm (AI provider; default) / ner (local on-device model — nothing leaves the machine) / hybrid (both),enum=llm|ner|hybrid,default=llm"`
	Provider    string         `json:"provider,omitempty" schema:"title=AI Provider,description=AI provider,default=anthropic,group=provider"`
	APIKey      string         `json:"apiKey,omitempty" schema:"title=API Key,description=API key for the AI provider,group=provider"`
	Model       string         `json:"model,omitempty" schema:"title=Model,description=AI model name,group=provider"`
	Locale      model.LocaleID `json:"locale,omitempty" schema:"description=Locale of the source content"`
	KnownTerms  []string       `json:"knownTerms,omitempty" schema:"description=Terms to exclude from extraction (already in termbase)"`                       // terms to exclude from extraction (already in termbase)
	BatchSize   int            `json:"batchSize,omitempty" schema:"description=Number of blocks per LLM call (0 or 1 = one block per call),default=1,min=1"`   // Blocks per LLM call. 0 or 1 = one block per call.
	Concurrency int            `json:"batchConcurrency,omitempty" schema:"description=Number of concurrent batch calls (0 or 1 = sequential),default=1,min=1"` // Concurrent batch calls. 0 or 1 = sequential.
}

// entityExtractFull is the reflected schema of the whole config — the source of
// the common fields and the LLM member fields.
func entityExtractFull() *schema.ComponentSchema {
	return schema.FromStruct(&AIEntityExtractConfig{}, schema.ToolMeta{
		ID:          "ai-entity-extract",
		Category:    schema.CategoryAnalysis,
		DisplayName: "AI Entity Extract",
		Description: "Detect named entities (people, organizations, products, locations) with an LLM, a local NER model, or both",
		Tags:        []string{"ai-powered"},
		Requires:    []string{schema.RequiresCredentials},
		Cardinality: schema.Monolingual,
		SideEffects: []schema.SideEffect{schema.SideEffectAPICall, schema.SideEffectRemoteSourceEgress},
		// NER annotator: produces the source-side entity and term-candidate
		// overlays a later tool (e.g. redact) consumes. Declaring them lets the
		// data-flow contract satisfy redact's required entity input when the two
		// run in one flow's settle stage (AD-006).
		Produces: []schema.IOPort{
			{Type: string(model.OverlayEntity), Side: model.SideSource},
			{Type: string(model.OverlayTermCandidate), Side: model.SideSource},
		},
	})
}

// entityExtractCommonSchema is the group's shared config: the engine selector
// plus locale and known-terms, common to every engine.
func entityExtractCommonSchema() *schema.ComponentSchema {
	full := entityExtractFull()
	return &schema.ComponentSchema{
		ID:          full.ID,
		Title:       full.Title,
		Description: full.Description,
		Type:        "object",
		ToolMeta:    full.ToolMeta,
		Properties: map[string]schema.PropertySchema{
			"engine":     full.Properties["engine"],
			"locale":     full.Properties["locale"],
			"knownTerms": full.Properties["knownTerms"],
		},
		Groups: []schema.ParameterGroup{{ID: "extract", Label: "Extraction", Fields: []string{"engine", "locale", "knownTerms"}}},
	}
}

// entityExtractMembers are the three extraction engines. The LLM provider/batch
// config is shared by the engines that call an LLM — llm and hybrid — via When.
func entityExtractMembers() []registry.ToolGroupMember {
	full := entityExtractFull()
	llmParams := &schema.ComponentSchema{
		Type: "object",
		Properties: map[string]schema.PropertySchema{
			"provider":         full.Properties["provider"],
			"apiKey":           full.Properties["apiKey"],
			"model":            full.Properties["model"],
			"batchSize":        full.Properties["batchSize"],
			"batchConcurrency": full.Properties["batchConcurrency"],
		},
		Groups: []schema.ParameterGroup{{ID: "provider", Label: "Provider", Fields: []string{"provider", "apiKey", "model"}}},
	}
	setProviderOptions(llmParams, aiProviderOptions())
	usesLLM := &schema.ConditionExpr{Any: []*schema.ConditionExpr{
		{Field: "engine", Eq: EngineLLM},
		{Field: "engine", Eq: EngineHybrid},
	}}
	return []registry.ToolGroupMember{
		{Name: EngineLLM, Label: "LLM (AI provider)", Description: "Extract with an AI provider.", Schema: llmParams, When: usesLLM},
		{Name: EngineNER, Label: "Local NER (on-device)", Description: "On-device model — no credentials, nothing leaves the machine."},
		{Name: EngineHybrid, Label: "Hybrid (NER + LLM)", Description: "Run the local model and the LLM, merging results."},
	}
}

// entityExtractGroup is the ai-entity-extract tool group: engine members (llm /
// ner / hybrid), llm as the default.
func entityExtractGroup() registry.ToolGroupDef {
	return registry.ToolGroupDef{
		Name:          "ai-entity-extract",
		Discriminator: "engine",
		Default:       EngineLLM,
		Common:        entityExtractCommonSchema(),
		Members:       entityExtractMembers(),
		ConfigFactory: NewAIEntityExtractFromConfig,
		Resolver:      ResolveEntityExtractContract,
	}
}

// AIEntityExtractSchema returns the composed (flat) projection of the group.
func AIEntityExtractSchema() *schema.ComponentSchema {
	return registry.ComposeGroupSchema(entityExtractGroup())
}

// NewAIEntityExtractFromConfig creates an entity-extraction tool from a config
// map, resolving the LLM provider the same way translate does. With
// `engine: ner` no LLM provider is resolved at all — extraction runs on the
// registered local NER model (ner.LocalProvider) and nothing leaves the
// machine; `hybrid` resolves both.
func NewAIEntityExtractFromConfig(config map[string]any, _ string) (tool.Tool, error) {
	var cfg AIEntityExtractConfig
	if err := schema.ApplyConfig(config, &cfg); err != nil {
		return nil, fmt.Errorf("ai-entity-extract config: %w", err)
	}

	var nerProvider ner.Provider
	if cfg.Engine == EngineNER || cfg.Engine == EngineHybrid {
		nerProvider = ner.LocalProvider()
		if nerProvider == nil {
			return nil, fmt.Errorf("ai-entity-extract: engine %q needs a local NER model, but none is available in this environment — in the browser load the local NER model first; natively use the default llm engine", cfg.Engine)
		}
	}
	if cfg.Engine == EngineNER {
		return NewAIEntityExtractTool(nil, nerProvider, cfg), nil
	}

	p, err := ProviderFromConfig(cfg.Provider, aiprovider.Config{APIKey: cfg.APIKey, Model: cfg.Model})
	if err != nil {
		return nil, err
	}
	return NewAIEntityExtractTool(p, nerProvider, cfg), nil
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
	// Annotate: extraction reads source and writes only entity/term-candidate
	// annotations. The batched path (Process override) reuses it via NewBlockView.
	et.Annotate = et.annotate
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

func (t *AIEntityExtractTool) annotate(v tool.BlockView) error {
	sourceText := v.SourceText()
	if strings.TrimSpace(sourceText) == "" {
		return nil
	}

	// Run NER if available.
	var nerEntities []ner.DetectedEntity
	if t.nerProvider != nil {
		resp, err := t.nerProvider.DetectEntities(v.Context(), ner.Request{
			Text:   sourceText,
			Locale: t.locale,
		})
		if err == nil && resp != nil {
			nerEntities = resp.Entities
		}
	}

	// Run LLM extraction (skipped for the ner engine — t.llm is nil and the
	// local model's entities are the whole result).
	var llmResult *llmExtractionResult
	if t.llm != nil {
		var err error
		llmResult, err = t.extractWithLLM(v.Context(), []extractionEntry{
			{blockID: v.ID(), text: sourceText},
		})
		if err != nil {
			return fmt.Errorf("ai-entity-extract: %w", err)
		}
	}

	// Merge and attach annotations.
	t.attachAnnotations(v, nerEntities, llmResult)

	return nil
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

	// 4. Run LLM extraction over batches with bounded concurrency (skipped for
	// the ner engine — t.llm is nil). Each batch writes its own result slot,
	// so no mutex is needed; goBatches returns the first error.
	batches := chunkBlocks(entries, t.batchSize)
	llmResults := make([]*llmExtractionResult, len(batches))
	if t.llm != nil {
		if err := goBatches(batches, t.concurrency, func(idx int, batch []extractionEntry) error {
			llmEntries := make([]extractionEntry, len(batch))
			copy(llmEntries, batch)
			result, err := t.extractWithLLM(ctx, llmEntries)
			if err != nil {
				return err
			}
			llmResults[idx] = result
			return nil
		}); err != nil {
			return err
		}
	}

	// 5. Merge each batch's LLM result with its NER results and attach.
	entryOffset := 0
	for batchIdx, batch := range batches {
		llmResult := llmResults[batchIdx]
		for i, entry := range batch {
			globalIdx := entryOffset + i
			var nerEnts []ner.DetectedEntity
			if ents, ok := nerResults[globalIdx]; ok {
				nerEnts = ents
			}
			t.attachAnnotationsFromBatch(tool.NewBlockView(entry.block), nerEnts, llmResult, entry.blockID)
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
		aiprovider.TextMessage("system", extractionSystemPrompt),
		aiprovider.TextMessage("user", prompt.String()),
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

func (t *AIEntityExtractTool) attachAnnotations(v tool.BlockView, nerEntities []ner.DetectedEntity, llmResult *llmExtractionResult) {
	// Find the LLM block result matching this block.
	var blockResult *llmBlockResult
	if llmResult != nil {
		for i := range llmResult.Blocks {
			if llmResult.Blocks[i].BlockID == v.ID() {
				blockResult = &llmResult.Blocks[i]
				break
			}
		}
	}
	t.mergeAndAttach(v, nerEntities, blockResult)
}

func (t *AIEntityExtractTool) attachAnnotationsFromBatch(v tool.BlockView, nerEntities []ner.DetectedEntity, llmResult *llmExtractionResult, blockID string) {
	var blockResult *llmBlockResult
	if llmResult != nil {
		for i := range llmResult.Blocks {
			if llmResult.Blocks[i].BlockID == blockID {
				blockResult = &llmResult.Blocks[i]
				break
			}
		}
	}
	t.mergeAndAttach(v, nerEntities, blockResult)
}

func (t *AIEntityExtractTool) mergeAndAttach(v tool.BlockView, nerEntities []ner.DetectedEntity, llmBlock *llmBlockResult) {
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
				Text:   e.Text,
				Type:   llmEntityType(e.Type),
				Locale: t.locale,
				DNT:    e.DNT,
				Source: model.ExtractionSourceLLM,
			}
			v.AddOverlaySpan(model.OverlayEntity, model.Span{
				ID:    fmt.Sprintf("entity:%d", entityIdx),
				Range: model.RunRangeForBytes(v.SourceRuns(), e.Offset, e.Offset+e.Length),
				Value: ann,
			})
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
			Text:   e.Text,
			Type:   e.Type,
			Locale: t.locale,
			DNT:    isDefaultDNT(e.Type),
			Source: model.ExtractionSourceNER,
		}
		v.AddOverlaySpan(model.OverlayEntity, model.Span{
			ID:    fmt.Sprintf("entity:%d", entityIdx),
			Range: model.RunRangeForBytes(v.SourceRuns(), e.Offset, e.Offset+e.Length),
			Value: ann,
		})
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
				Locale:          t.locale,
				Source:          model.ExtractionSourceLLM,
				Status:          model.CandidateStatusPending,
			}
			v.AddOverlaySpan(model.OverlayTermCandidate, model.Span{
				ID:    fmt.Sprintf("term-candidate:%d", termIdx),
				Range: model.RunRangeForBytes(v.SourceRuns(), tc.Offset, tc.Offset+tc.Length),
				Value: ann,
			})
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
