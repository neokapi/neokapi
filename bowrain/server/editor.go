package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/labstack/echo/v4"

	"github.com/gokapi/gokapi/bowrain/credentials"
	sqltm "github.com/gokapi/gokapi/bowrain/sievepen"
	"github.com/gokapi/gokapi/bowrain/storage"
	sqltb "github.com/gokapi/gokapi/bowrain/termbase"
	"github.com/gokapi/gokapi/core/ai/provider"
	"github.com/gokapi/gokapi/core/ai/tools"
	"github.com/gokapi/gokapi/core/editor"
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/registry"
	"github.com/gokapi/gokapi/core/sievepen"
	"github.com/gokapi/gokapi/core/termbase"
	"github.com/gokapi/gokapi/core/tool"
	"github.com/gokapi/gokapi/platform/store"
)

// ---------------------------------------------------------------------------
// Workspace TM/TB management (persistent, file-backed)
// ---------------------------------------------------------------------------

// workspaceTMTB holds workspace-scoped TM and terminology stores.
type workspaceTMTB struct {
	tm sqltm.TMStore
	tb termbase.TermBase
}

// workspaceStores manages per-workspace TM and terminology stores.
type workspaceStores struct {
	mu      sync.RWMutex
	stores  map[string]*workspaceTMTB
	dataDir string
	pgDB    *storage.PgDB // non-nil in PostgreSQL (SaaS) mode
}

func newWorkspaceStores(dataDir string) *workspaceStores {
	return &workspaceStores{
		stores:  make(map[string]*workspaceTMTB),
		dataDir: dataDir,
	}
}

func (ws *workspaceStores) getOrCreate(wsSlug string) *workspaceTMTB {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	w, ok := ws.stores[wsSlug]
	if !ok {
		w = &workspaceTMTB{}
		ws.stores[wsSlug] = w
	}
	return w
}

func (ws *workspaceStores) getTM(wsSlug string) (sqltm.TMStore, error) {
	w := ws.getOrCreate(wsSlug)
	if w.tm != nil {
		return w.tm, nil
	}

	// PostgreSQL mode: all workspaces share the same database, scoped by workspace_id.
	if ws.pgDB != nil {
		tm, err := sqltm.NewPostgresTMFromDB(ws.pgDB, wsSlug)
		if err != nil {
			return nil, err
		}
		w.tm = tm
		return tm, nil
	}

	// SQLite mode: file-backed per workspace (or in-memory).
	tmPath := ":memory:"
	if ws.dataDir != "" {
		tmDir := filepath.Join(ws.dataDir, "tm")
		if err := os.MkdirAll(tmDir, 0755); err != nil {
			return nil, fmt.Errorf("create TM dir: %w", err)
		}
		tmPath = filepath.Join(tmDir, wsSlug+".db")
	}

	tm, err := sqltm.NewSQLiteTM(tmPath)
	if err != nil {
		return nil, err
	}
	w.tm = tm
	return tm, nil
}

func (ws *workspaceStores) getTB(wsSlug string) termbase.TermBase {
	w := ws.getOrCreate(wsSlug)
	if w.tb != nil {
		return w.tb
	}

	// PostgreSQL mode: persistent workspace-scoped termbase.
	if ws.pgDB != nil {
		tb, err := sqltb.NewPostgresTermBaseFromDB(ws.pgDB, wsSlug)
		if err == nil {
			w.tb = tb
			return tb
		}
		// Fall back to in-memory on error.
	}

	// SQLite / in-memory mode.
	w.tb = termbase.NewInMemoryTermBase()
	return w.tb
}

// ---------------------------------------------------------------------------
// API response/request types
// ---------------------------------------------------------------------------

// ProjectInfoResponse is the API response for a translation project.
type ProjectInfoResponse struct {
	ID            string                `json:"id"`
	Name          string                `json:"name"`
	SourceLocale  string                `json:"source_locale"`
	TargetLocales []string              `json:"target_locales"`
	Items         []ProjectItemResponse `json:"items"`
	CreatedAt     string                `json:"created_at"`
	ModifiedAt    string                `json:"modified_at"`
}

// ProjectItemResponse describes an item within a project.
type ProjectItemResponse struct {
	Name       string `json:"name"`
	Format     string `json:"format"`
	Type       string `json:"type"`
	Size       int64  `json:"size"`
	BlockCount int    `json:"block_count"`
	WordCount  int    `json:"word_count"`
}

// SpanInfoResponse describes an inline span element.
type SpanInfoResponse struct {
	SpanType    string `json:"span_type"`
	Type        string `json:"type"`
	ID          string `json:"id"`
	Data        string `json:"data"`
	SubType     string `json:"sub_type,omitempty"`
	DisplayText string `json:"display_text,omitempty"`
	EquivText   string `json:"equiv_text,omitempty"`
	Deletable   bool   `json:"deletable,omitempty"`
	Cloneable   bool   `json:"cloneable,omitempty"`
	CanReorder  bool   `json:"can_reorder,omitempty"`
}

// BlockInfoResponse is a serializable representation of a translatable block.
type BlockInfoResponse struct {
	ID           string               `json:"id"`
	Source       string               `json:"source"`
	SourceCoded  string               `json:"source_coded,omitempty"`
	SourceSpans  []SpanInfoResponse   `json:"source_spans,omitempty"`
	Targets      map[string]string    `json:"targets"`
	TargetsCoded map[string]string    `json:"targets_coded,omitempty"`
	Translatable bool                 `json:"translatable"`
	HasSpans     bool                 `json:"has_spans"`
	Properties   map[string]string    `json:"properties"`
	Entities     []EntityInfoResponse `json:"entities,omitempty"`
}

// EntityInfoResponse represents an entity annotation on a block.
type EntityInfoResponse struct {
	Key    string `json:"key"`              // annotation key (e.g. "entity:0")
	Text   string `json:"text"`
	Type   string `json:"type"`
	Start  int    `json:"start"`
	End    int    `json:"end"`
	DNT    bool   `json:"dnt"`
	Source string `json:"source,omitempty"` // "llm", "ner", "manual"
	Locale string `json:"locale,omitempty"`
}

// TermCandidateInfoResponse represents a term candidate annotation on a block.
type TermCandidateInfoResponse struct {
	Key             string  `json:"key"` // annotation key (e.g. "term-candidate:0")
	Text            string  `json:"text"`
	Definition      string  `json:"definition"`
	Category        string  `json:"category"`
	Translatability string  `json:"translatability"`
	Confidence      float64 `json:"confidence"`
	Start           int     `json:"start"`
	End             int     `json:"end"`
	Source          string  `json:"source,omitempty"`
	Status          string  `json:"status,omitempty"`
}

// UpdateBlockTargetRequest holds parameters for updating a block target.
type UpdateBlockTargetRequest struct {
	TargetLocale string `json:"target_locale"`
	Text         string `json:"text"`
}

// UpdateBlockTargetCodedRequest holds parameters for coded text update.
type UpdateBlockTargetCodedRequest struct {
	TargetLocale string             `json:"target_locale"`
	CodedText    string             `json:"coded_text"`
	Spans        []SpanInfoResponse `json:"spans"`
}

// TranslateRequest holds parameters for translation operations.
type TranslateRequest struct {
	TargetLocale     string `json:"target_locale"`
	Provider         string `json:"provider,omitempty"`
	APIKey           string `json:"api_key,omitempty"`
	Model            string `json:"model,omitempty"`
	ProviderConfigID string `json:"provider_config_id,omitempty"`
	BatchSize        int    `json:"batch_size,omitempty"`
	Concurrency      int    `json:"concurrency,omitempty"`
}

// TranslationStatsResponse holds statistics about a translation operation.
type TranslationStatsResponse struct {
	TotalBlocks      int `json:"total_blocks"`
	TranslatedBlocks int `json:"translated_blocks"`
	WordCount        int `json:"word_count"`
}

// WordCountResponse holds word and character counts.
type WordCountResponse struct {
	SourceWords int            `json:"source_words"`
	SourceChars int            `json:"source_chars"`
	TargetWords map[string]int `json:"target_words"`
	TargetChars map[string]int `json:"target_chars"`
}

// TMMatchInfoResponse is a TM match result.
type TMMatchInfoResponse struct {
	Source    string  `json:"source"`
	Target    string  `json:"target"`
	Score     float64 `json:"score"`
	MatchType string  `json:"match_type"`
	ProjectID string  `json:"project_id,omitempty"` // which project this match came from
}

// BlockTermMatchResponse is a term match for a block.
type BlockTermMatchResponse struct {
	SourceTerm  string   `json:"source_term"`
	TargetTerms []string `json:"target_terms"`
	Domain      string   `json:"domain"`
	Status      string   `json:"status"`
	Start       int      `json:"start"`
	End         int      `json:"end"`
	ProjectID   string   `json:"project_id,omitempty"` // scope info
}

// --- TM types ---

// TMEntryInfoResponse is the API response for a TM entry.
type TMEntryInfoResponse struct {
	ID           string `json:"id"`
	Source       string `json:"source"`
	Target       string `json:"target"`
	SourceLocale string `json:"source_locale"`
	TargetLocale string `json:"target_locale"`
	ProjectID    string `json:"project_id,omitempty"`
	UpdatedAt    string `json:"updated_at"`
}

// TMSearchResponse holds a page of TM search results.
type TMSearchResponse struct {
	Entries    []TMEntryInfoResponse `json:"entries"`
	TotalCount int                   `json:"total_count"`
}

// TMAddRequest holds parameters for adding a TM entry.
type TMAddRequest struct {
	Source       string `json:"source"`
	Target       string `json:"target"`
	SourceLocale string `json:"source_locale"`
	TargetLocale string `json:"target_locale"`
	ProjectID    string `json:"project_id"` // which project to associate with
}

// TMUpdateRequest holds parameters for updating a TM entry.
type TMUpdateRequest struct {
	Source       string `json:"source"`
	Target       string `json:"target"`
	SourceLocale string `json:"source_locale"`
	TargetLocale string `json:"target_locale"`
}

// --- Terminology types ---

// TermInfoResponse is a term in a concept.
type TermInfoResponse struct {
	Text         string `json:"text"`
	Locale       string `json:"locale"`
	Status       string `json:"status"`
	PartOfSpeech string `json:"part_of_speech,omitempty"`
	Gender       string `json:"gender,omitempty"`
	Note         string `json:"note,omitempty"`
}

// ConceptInfoResponse is the API response for a concept.
type ConceptInfoResponse struct {
	ID         string             `json:"id"`
	ProjectID  string             `json:"project_id,omitempty"` // empty = workspace-scoped
	Domain     string             `json:"domain"`
	Definition string             `json:"definition"`
	Terms      []TermInfoResponse `json:"terms"`
	Properties map[string]string  `json:"properties,omitempty"`
	CreatedAt  string             `json:"created_at"`
	UpdatedAt  string             `json:"updated_at"`
}

// TermSearchResponse holds a page of term search results.
type TermSearchResponse struct {
	Concepts   []ConceptInfoResponse `json:"concepts"`
	TotalCount int                   `json:"total_count"`
}

// AddConceptRequest holds parameters for adding a concept.
type AddConceptRequest struct {
	ProjectID  string             `json:"project_id"` // empty = workspace-scoped
	Domain     string             `json:"domain"`
	Definition string             `json:"definition"`
	Terms      []TermInfoResponse `json:"terms"`
}

// UpdateConceptRequest holds parameters for updating a concept.
type UpdateConceptRequest struct {
	Domain     string             `json:"domain"`
	Definition string             `json:"definition"`
	Terms      []TermInfoResponse `json:"terms"`
}

// ImportCSVRequest holds parameters for CSV term import.
type ImportCSVRequest struct {
	CSVContent   string `json:"csv_content"`
	SourceLocale string `json:"source_locale"`
	TargetLocale string `json:"target_locale"`
	Domain       string `json:"domain"`
	HasHeader    bool   `json:"has_header"`
}

// ImportJSONRequest holds parameters for JSON term import.
type ImportJSONRequest struct {
	JSONContent string `json:"json_content"`
}

// ExportJSONRequest holds parameters for JSON term export.
type ExportJSONRequest struct {
	Name string `json:"name"`
}

// --- Provider types ---

// ProviderConfigResponse is the API response for a provider config (no API key).
type ProviderConfigResponse struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ProviderType string `json:"provider_type"`
	Model        string `json:"model"`
	BaseURL      string `json:"base_url"`
}

// SaveProviderConfigRequest is used to create/update a provider with optional API key.
type SaveProviderConfigRequest struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ProviderType string `json:"provider_type"`
	Model        string `json:"model"`
	BaseURL      string `json:"base_url"`
	APIKey       string `json:"api_key"`
}

func toProviderConfigResponse(c credentials.ProviderConfig) ProviderConfigResponse {
	return ProviderConfigResponse{
		ID:           c.ID,
		Name:         c.Name,
		ProviderType: c.ProviderType,
		Model:        c.Model,
		BaseURL:      c.BaseURL,
	}
}

func (r SaveProviderConfigRequest) toCredentials() credentials.ProviderConfig {
	return credentials.ProviderConfig{
		ID:           r.ID,
		Name:         r.Name,
		ProviderType: r.ProviderType,
		Model:        r.Model,
		BaseURL:      r.BaseURL,
	}
}

// ---------------------------------------------------------------------------
// ContentStore-backed editor operations
// ---------------------------------------------------------------------------

// editorCreateProject creates a new project in the ContentStore.
func editorCreateProject(ctx context.Context, cs store.ContentStore, ws, name, sourceLang string, targetLangs []string) (*ProjectInfoResponse, error) {
	if name == "" {
		return nil, fmt.Errorf("project name is required")
	}
	if sourceLang == "" {
		return nil, fmt.Errorf("source language is required")
	}
	if len(targetLangs) == 0 {
		return nil, fmt.Errorf("at least one target language is required")
	}

	locales := make([]model.LocaleID, len(targetLangs))
	for i, l := range targetLangs {
		locales[i] = model.LocaleID(l)
	}

	p := &store.Project{
		Name:          name,
		SourceLocale:  model.LocaleID(sourceLang),
		TargetLocales: locales,
		WorkspaceID:   ws,
		Properties:    map[string]string{},
	}
	if err := cs.CreateProject(ctx, p); err != nil {
		return nil, fmt.Errorf("create project: %w", err)
	}

	return projectToInfoResponse(p), nil
}

// editorAddFiles parses uploaded files, stores items and blocks in ContentStore.
func editorAddFiles(ctx context.Context, cs store.ContentStore, formatReg *registry.FormatRegistry, projectID string, files map[string][]byte) (*ProjectInfoResponse, error) {
	proj, err := cs.GetProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}

	for itemName, data := range files {
		ext := filepath.Ext(itemName)
		fmtName, err := formatReg.Detector().DetectByExtension(ext)
		if err != nil {
			continue
		}

		reader, err := formatReg.NewReader(fmtName)
		if err != nil {
			continue
		}

		doc := &model.RawDocument{
			URI:          itemName,
			SourceLocale: proj.SourceLocale,
			Encoding:     "UTF-8",
			Reader:       io.NopCloser(bytes.NewReader(data)),
		}

		if err := reader.Open(ctx, doc); err != nil {
			return nil, fmt.Errorf("parse %q: %w", itemName, err)
		}

		var parts []*model.Part
		for result := range reader.Read(ctx) {
			if result.Error != nil {
				reader.Close()
				return nil, fmt.Errorf("read %q: %w", itemName, result.Error)
			}
			parts = append(parts, result.Part)
		}
		reader.Close()

		// Build block index.
		blockIndex := editor.BuildBlockIndex(parts, string(proj.SourceLocale), fmtName, itemName)
		blockIndexJSON, _ := json.Marshal(blockIndex)

		// Store item with source bytes.
		item := &store.Item{
			Name:        itemName,
			Format:      fmtName,
			ItemType:    "file",
			SourceBytes: data,
			BlockIndex:  string(blockIndexJSON),
			Properties:  map[string]string{},
		}
		if err := cs.StoreItem(ctx, projectID, "main", item); err != nil {
			return nil, fmt.Errorf("store item %q: %w", itemName, err)
		}

		// Extract blocks and store them.
		var blocks []*model.Block
		for _, pt := range parts {
			if pt.Type != model.PartBlock {
				continue
			}
			if block, ok := pt.Resource.(*model.Block); ok {
				blocks = append(blocks, block)
			}
		}
		if len(blocks) > 0 {
			if err := cs.StoreBlocksForItem(ctx, projectID, "main", itemName, blocks); err != nil {
				return nil, fmt.Errorf("store blocks for %q: %w", itemName, err)
			}
		}
	}

	return editorBuildProjectInfo(ctx, cs, proj)
}

// editorRemoveFile removes an item and its blocks from ContentStore.
func editorRemoveFile(ctx context.Context, cs store.ContentStore, projectID, fileName string) (*ProjectInfoResponse, error) {
	proj, err := cs.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}

	if err := cs.DeleteItem(ctx, projectID, "main", fileName); err != nil {
		return nil, err
	}

	return editorBuildProjectInfo(ctx, cs, proj)
}

// editorGetBlocks returns blocks for a specific item, formatted for the API.
func editorGetBlocks(ctx context.Context, cs store.ContentStore, projectID, itemName string, targetLocales []string) ([]BlockInfoResponse, error) {
	storedBlocks, err := cs.GetBlocks(ctx, store.BlockQuery{
		ProjectID: projectID,
		Stream:    "main",
		ItemName:  itemName,
	})
	if err != nil {
		return nil, err
	}

	blocks := make([]BlockInfoResponse, 0, len(storedBlocks))
	for _, sb := range storedBlocks {
		bi := storedBlockToInfoResponse(sb, targetLocales)
		blocks = append(blocks, bi)
	}
	return blocks, nil
}

// editorUpdateBlockTarget loads a block, updates its target, and stores it back.
func editorUpdateBlockTarget(ctx context.Context, cs store.ContentStore, projectID, blockID string, req UpdateBlockTargetRequest) error {
	sb, err := cs.GetBlock(ctx, projectID, "main", blockID)
	if err != nil {
		return err
	}

	sb.Block.SetTargetText(model.LocaleID(req.TargetLocale), req.Text)

	return cs.StoreBlocks(ctx, projectID, "main", []*model.Block{sb.Block})
}

// editorUpdateBlockTargetCoded loads a block, updates its target with coded text, and stores it back.
func editorUpdateBlockTargetCoded(ctx context.Context, cs store.ContentStore, projectID, blockID string, req UpdateBlockTargetCodedRequest) error {
	sb, err := cs.GetBlock(ctx, projectID, "main", blockID)
	if err != nil {
		return err
	}

	frag := &model.Fragment{
		CodedText: req.CodedText,
	}
	for _, si := range req.Spans {
		frag.Spans = append(frag.Spans, editorInfoToSpan(si))
	}
	sb.Block.SetTargetFragment(model.LocaleID(req.TargetLocale), frag)

	return cs.StoreBlocks(ctx, projectID, "main", []*model.Block{sb.Block})
}

// editorPseudoTranslate pseudo-translates all blocks for an item.
func editorPseudoTranslate(ctx context.Context, cs store.ContentStore, projectID, itemName, targetLocale string) (*TranslationStatsResponse, error) {
	storedBlocks, err := cs.GetBlocks(ctx, store.BlockQuery{
		ProjectID: projectID,
		Stream:    "main",
		ItemName:  itemName,
	})
	if err != nil {
		return nil, err
	}

	// Convert to parts for tool processing.
	parts := storedBlocksToParts(storedBlocks)

	pseudoTool := &tool.BaseTool{
		ToolName:        "pseudo-translate",
		ToolDescription: "Pseudo-translates blocks",
	}
	pseudoTool.HandleBlockFn = func(part *model.Part) (*model.Part, error) {
		block, ok := part.Resource.(*model.Block)
		if !ok || !block.Translatable {
			return part, nil
		}
		locale := model.LocaleID(targetLocale)
		frag := block.FirstFragment()
		if frag != nil && frag.HasSpans() {
			pseudoCoded := "[" + editorPseudoAccent(frag.CodedText) + "]"
			targetFrag := frag.Clone()
			targetFrag.CodedText = pseudoCoded
			block.SetTargetFragment(locale, targetFrag)
		} else {
			src := block.SourceText()
			pseudo := "[" + editorPseudoAccent(src) + "]"
			block.SetTargetText(locale, pseudo)
		}
		return part, nil
	}

	outParts, err := runToolOnParts(ctx, pseudoTool, parts)
	if err != nil {
		return nil, fmt.Errorf("pseudo-translate: %w", err)
	}

	// Store updated blocks back — they already have internal IDs from GetBlocks.
	blocks := partsToBlocks(outParts)
	if len(blocks) > 0 {
		if err := cs.StoreBlocks(ctx, projectID, "main", blocks); err != nil {
			return nil, fmt.Errorf("store blocks: %w", err)
		}
	}

	return editorComputeStats(outParts, targetLocale), nil
}

// editorAITranslate translates blocks using an AI provider.
func editorAITranslate(ctx context.Context, cs store.ContentStore, projectID, itemName string, req TranslateRequest, credStore *credentials.Store) (*TranslationStatsResponse, error) {
	proj, err := cs.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}

	storedBlocks, err := cs.GetBlocks(ctx, store.BlockQuery{
		ProjectID: projectID,
		Stream:    "main",
		ItemName:  itemName,
	})
	if err != nil {
		return nil, err
	}

	parts := storedBlocksToParts(storedBlocks)

	var prov provider.LLMProvider
	if req.ProviderConfigID != "" && credStore != nil {
		prov, err = credentials.NewProvider(credStore, req.ProviderConfigID)
		if err != nil {
			return nil, fmt.Errorf("resolve provider config: %w", err)
		}
	} else {
		prov = editorCreateProvider(req.Provider, req.APIKey, req.Model)
	}

	translateTool := tools.NewAITranslateTool(prov, tools.AITranslateConfig{
		SourceLocale: proj.SourceLocale,
		TargetLocale: model.LocaleID(req.TargetLocale),
		BatchSize:    req.BatchSize,
		Concurrency:  req.Concurrency,
	})

	outParts, err := runToolOnParts(ctx, translateTool, parts)
	if err != nil {
		return nil, fmt.Errorf("AI translate: %w", err)
	}

	blocks := partsToBlocks(outParts)
	if len(blocks) > 0 {
		if err := cs.StoreBlocks(ctx, projectID, "main", blocks); err != nil {
			return nil, fmt.Errorf("store blocks: %w", err)
		}
	}

	return editorComputeStats(outParts, req.TargetLocale), nil
}

// editorTMTranslate leverages translation memory to translate blocks.
func editorTMTranslate(ctx context.Context, cs store.ContentStore, wsStores *workspaceStores, ws, projectID, itemName, targetLocale string) (*TranslationStatsResponse, error) {
	proj, err := cs.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}

	storedBlocks, err := cs.GetBlocks(ctx, store.BlockQuery{
		ProjectID: projectID,
		Stream:    "main",
		ItemName:  itemName,
	})
	if err != nil {
		return nil, err
	}

	tm, err := wsStores.getTM(ws)
	if err != nil {
		return nil, fmt.Errorf("init TM: %w", err)
	}

	parts := storedBlocksToParts(storedBlocks)

	tmTool := sievepen.NewTMLeverageTool(tm, sievepen.TMLeverageConfig{
		MinScore:     0.7,
		MaxResults:   5,
		SourceLocale: proj.SourceLocale,
		TargetLocale: model.LocaleID(targetLocale),
	})

	outParts, err := runToolOnParts(ctx, tmTool, parts)
	if err != nil {
		return nil, fmt.Errorf("TM translate: %w", err)
	}

	blocks := partsToBlocks(outParts)
	if len(blocks) > 0 {
		if err := cs.StoreBlocks(ctx, projectID, "main", blocks); err != nil {
			return nil, fmt.Errorf("store blocks: %w", err)
		}
	}

	return editorComputeStats(outParts, targetLocale), nil
}

// editorGetWordCount computes word/char counts from stored blocks.
func editorGetWordCount(ctx context.Context, cs store.ContentStore, projectID, itemName string, targetLocales []string) (*WordCountResponse, error) {
	storedBlocks, err := cs.GetBlocks(ctx, store.BlockQuery{
		ProjectID: projectID,
		Stream:    "main",
		ItemName:  itemName,
	})
	if err != nil {
		return nil, err
	}

	result := &WordCountResponse{
		TargetWords: make(map[string]int),
		TargetChars: make(map[string]int),
	}

	for _, sb := range storedBlocks {
		if !sb.Block.Translatable {
			continue
		}
		src := sb.Block.SourceText()
		result.SourceWords += editorCountWords(src)
		result.SourceChars += len([]rune(src))

		for _, locale := range targetLocales {
			t := sb.Block.TargetText(model.LocaleID(locale))
			if t != "" {
				result.TargetWords[locale] += editorCountWords(t)
				result.TargetChars[locale] += len([]rune(t))
			}
		}
	}

	return result, nil
}

// editorExportTranslatedFile exports a translated file using source bytes + updated blocks.
func editorExportTranslatedFile(ctx context.Context, cs store.ContentStore, formatReg *registry.FormatRegistry, projectID, itemName, targetLocale, dataDir string) (string, error) {
	proj, err := cs.GetProject(ctx, projectID)
	if err != nil {
		return "", err
	}

	item, err := cs.GetItem(ctx, projectID, "main", itemName)
	if err != nil {
		return "", fmt.Errorf("get item: %w", err)
	}

	if len(item.SourceBytes) == 0 {
		return "", fmt.Errorf("item %q has no source bytes for export", itemName)
	}

	// Re-parse source bytes to get the Part stream.
	reader, err := formatReg.NewReader(item.Format)
	if err != nil {
		return "", fmt.Errorf("no reader for %q: %w", item.Format, err)
	}

	doc := &model.RawDocument{
		URI:          itemName,
		SourceLocale: proj.SourceLocale,
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader(item.SourceBytes)),
	}

	if err := reader.Open(ctx, doc); err != nil {
		reader.Close()
		return "", fmt.Errorf("parse source: %w", err)
	}

	var parts []*model.Part
	for result := range reader.Read(ctx) {
		if result.Error != nil {
			reader.Close()
			return "", fmt.Errorf("read source: %w", result.Error)
		}
		parts = append(parts, result.Part)
	}
	reader.Close()

	// Load updated blocks from ContentStore and inject targets into parts.
	storedBlocks, err := cs.GetBlocks(ctx, store.BlockQuery{
		ProjectID: projectID,
		Stream:    "main",
		ItemName:  itemName,
	})
	if err != nil {
		return "", fmt.Errorf("get blocks: %w", err)
	}

	// Build blockMap keyed by source_id (the format reader's ID) for target injection.
	blockMap := make(map[string]*model.Block, len(storedBlocks))
	for _, sb := range storedBlocks {
		key := sb.SourceID
		if key == "" {
			key = sb.Block.ID
		}
		blockMap[key] = sb.Block
	}

	// Inject stored targets into the parsed parts.
	for _, pt := range parts {
		if pt.Type != model.PartBlock {
			continue
		}
		block, ok := pt.Resource.(*model.Block)
		if !ok {
			continue
		}
		if stored, ok := blockMap[block.ID]; ok {
			block.Targets = stored.Targets
		}
	}

	// Write output.
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(itemName)), ".")
	baseName := itemName
	if ext != "" {
		baseName = itemName[:len(itemName)-len(ext)-1]
	}
	outputName := fmt.Sprintf("%s_%s.%s", baseName, targetLocale, ext)

	dir := dataDir
	if dir == "" {
		dir = os.TempDir()
	}
	outputPath := filepath.Join(dir, outputName)

	writer, err := formatReg.NewWriter(item.Format)
	if err != nil {
		return "", fmt.Errorf("no writer for %q: %w", item.Format, err)
	}

	if err := writer.SetOutput(outputPath); err != nil {
		return "", fmt.Errorf("set output: %w", err)
	}
	writer.SetLocale(model.LocaleID(targetLocale))

	ch := make(chan *model.Part, len(parts))
	for _, pt := range parts {
		ch <- pt
	}
	close(ch)

	if err := writer.Write(ctx, ch); err != nil {
		return "", fmt.Errorf("write: %w", err)
	}
	writer.Close()

	return outputPath, nil
}

// editorLookupTMForBlock looks up TM matches for a specific block.
func editorLookupTMForBlock(ctx context.Context, cs store.ContentStore, wsStores *workspaceStores, ws, projectID, blockID, targetLocale string) ([]TMMatchInfoResponse, error) {
	proj, err := cs.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}

	tm, err := wsStores.getTM(ws)
	if err != nil {
		return nil, fmt.Errorf("init TM: %w", err)
	}
	if tm.Count() == 0 {
		return nil, nil
	}

	sb, err := cs.GetBlock(ctx, projectID, "main", blockID)
	if err != nil {
		return nil, err
	}

	opts := sievepen.DefaultLookupOptions()
	opts.MaxResults = 5
	opts.ProjectID = projectID // for scoring boost
	matches, err := tm.Lookup(sb.Block, proj.SourceLocale, model.LocaleID(targetLocale), opts)
	if err != nil {
		return nil, err
	}

	result := make([]TMMatchInfoResponse, len(matches))
	for i, m := range matches {
		result[i] = TMMatchInfoResponse{
			Source:    m.Entry.SourceText(),
			Target:    m.Entry.TargetText(),
			Score:     m.Score,
			MatchType: string(m.MatchType),
			ProjectID: m.Entry.ProjectID,
		}
	}
	return result, nil
}

// editorLookupTermsForBlock looks up term matches for a block.
func editorLookupTermsForBlock(ctx context.Context, cs store.ContentStore, wsStores *workspaceStores, ws, projectID, blockID, targetLocale string) ([]BlockTermMatchResponse, error) {
	proj, err := cs.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}

	tb := wsStores.getTB(ws)
	if tb.Count() == 0 {
		return nil, nil
	}

	sb, err := cs.GetBlock(ctx, projectID, "main", blockID)
	if err != nil {
		return nil, err
	}

	sourceText := sb.Block.SourceText()
	if sourceText == "" {
		return nil, nil
	}

	matches := tb.LookupAll(sourceText, termbase.LookupOptions{
		SourceLocale: proj.SourceLocale,
		TargetLocale: model.LocaleID(targetLocale),
		ProjectID:    projectID,
	})

	result := make([]BlockTermMatchResponse, 0)
	for _, m := range matches {
		targetTerms := make([]string, 0)
		for _, t := range m.Concept.Terms {
			if t.Locale == model.LocaleID(targetLocale) {
				targetTerms = append(targetTerms, t.Text)
			}
		}
		result = append(result, BlockTermMatchResponse{
			SourceTerm:  m.Term.Text,
			TargetTerms: targetTerms,
			Domain:      m.Concept.Domain,
			Status:      string(m.Term.Status),
			Start:       m.Position.Start,
			End:         m.Position.End,
			ProjectID:   m.Concept.ProjectID,
		})
	}
	return result, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// fileParam extracts the filename from an Echo wildcard (*) route parameter.
// Wildcard params include a leading slash, so we trim it.
func fileParam(c echo.Context) string {
	return strings.TrimPrefix(c.Param("*"), "/")
}

// projectToInfoResponse converts a store.Project to ProjectInfoResponse.
func projectToInfoResponse(p *store.Project) *ProjectInfoResponse {
	locales := make([]string, len(p.TargetLocales))
	for i, l := range p.TargetLocales {
		locales[i] = string(l)
	}
	return &ProjectInfoResponse{
		ID:            p.ID,
		Name:          p.Name,
		SourceLocale:  string(p.SourceLocale),
		TargetLocales: locales,
		Items:         []ProjectItemResponse{},
		CreatedAt:     p.CreatedAt.Format(time.RFC3339),
		ModifiedAt:    p.UpdatedAt.Format(time.RFC3339),
	}
}

// editorBuildProjectInfo builds a full ProjectInfoResponse from store data.
func editorBuildProjectInfo(ctx context.Context, cs store.ContentStore, proj *store.Project) (*ProjectInfoResponse, error) {
	info := projectToInfoResponse(proj)

	items, err := cs.ListItems(ctx, proj.ID, "main")
	if err != nil {
		return nil, fmt.Errorf("list items: %w", err)
	}

	for _, item := range items {
		blocks, err := cs.GetBlocks(ctx, store.BlockQuery{
			ProjectID: proj.ID,
			ItemName:  item.Name,
		})
		if err != nil {
			return nil, fmt.Errorf("get blocks for %q: %w", item.Name, err)
		}

		wordCount := 0
		for _, sb := range blocks {
			if sb.Block.Translatable {
				wordCount += editorCountWords(sb.Block.SourceText())
			}
		}

		info.Items = append(info.Items, ProjectItemResponse{
			Name:       item.Name,
			Format:     item.Format,
			Type:       item.ItemType,
			Size:       int64(len(item.SourceBytes)),
			BlockCount: len(blocks),
			WordCount:  wordCount,
		})
	}

	return info, nil
}

// storedBlockToInfoResponse converts a StoredBlock to a BlockInfoResponse.
func storedBlockToInfoResponse(sb *store.StoredBlock, targetLocales []string) BlockInfoResponse {
	targets := make(map[string]string)
	for _, locale := range targetLocales {
		if t := sb.Block.TargetText(model.LocaleID(locale)); t != "" {
			targets[locale] = t
		}
	}

	props := make(map[string]string)
	for k, v := range sb.Block.Properties {
		props[k] = v
	}

	bi := BlockInfoResponse{
		ID:           sb.Block.ID,
		Source:       sb.Block.SourceText(),
		Targets:      targets,
		Translatable: sb.Block.Translatable,
		Properties:   props,
	}

	enrichBlockInfoResponse(&bi, sb.Block, targetLocales)
	enrichBlockEntities(&bi, sb.Block)
	return bi
}

func enrichBlockInfoResponse(bi *BlockInfoResponse, block *model.Block, targetLocales []string) {
	if len(block.Source) == 0 || block.Source[0].Content == nil {
		return
	}
	frag := block.Source[0].Content
	if !frag.HasSpans() {
		return
	}

	bi.HasSpans = true
	bi.SourceCoded = frag.CodedText
	bi.SourceSpans = make([]SpanInfoResponse, len(frag.Spans))
	for i, s := range frag.Spans {
		bi.SourceSpans[i] = editorSpanToInfo(s)
	}

	bi.TargetsCoded = make(map[string]string)
	for _, locale := range targetLocales {
		segs, ok := block.Targets[model.LocaleID(locale)]
		if !ok || len(segs) == 0 {
			continue
		}
		if segs[0].Content != nil {
			bi.TargetsCoded[locale] = segs[0].Content.CodedText
		}
	}
}

// enrichBlockEntities extracts entity and term-candidate annotations from a block.
func enrichBlockEntities(bi *BlockInfoResponse, block *model.Block) {
	if block.Annotations == nil {
		return
	}

	for key, ann := range block.Annotations {
		switch a := ann.(type) {
		case *model.EntityAnnotation:
			bi.Entities = append(bi.Entities, EntityInfoResponse{
				Key:    key,
				Text:   a.Text,
				Type:   string(a.Type),
				Start:  a.Position.Start,
				End:    a.Position.End,
				DNT:    a.DNT,
				Source: string(a.Source),
				Locale: string(a.Locale),
			})
		}
	}
}

// storedBlocksToParts wraps stored blocks as Part objects for tool processing.
func storedBlocksToParts(storedBlocks []*store.StoredBlock) []*model.Part {
	parts := make([]*model.Part, 0, len(storedBlocks))
	for _, sb := range storedBlocks {
		parts = append(parts, &model.Part{
			Type:     model.PartBlock,
			Resource: sb.Block,
		})
	}
	return parts
}

// partsToBlocks extracts model.Block objects from a Part slice.
func partsToBlocks(parts []*model.Part) []*model.Block {
	var blocks []*model.Block
	for _, pt := range parts {
		if pt.Type != model.PartBlock {
			continue
		}
		if block, ok := pt.Resource.(*model.Block); ok {
			blocks = append(blocks, block)
		}
	}
	return blocks
}

// runToolOnParts executes a tool on parts using channels.
func runToolOnParts(ctx context.Context, t tool.Tool, parts []*model.Part) ([]*model.Part, error) {
	in := make(chan *model.Part, len(parts))
	out := make(chan *model.Part, len(parts))
	for _, pt := range parts {
		in <- pt
	}
	close(in)

	if err := t.Process(ctx, in, out); err != nil {
		return nil, err
	}
	close(out)

	var result []*model.Part
	for pt := range out {
		result = append(result, pt)
	}
	return result, nil
}

func editorSpanToInfo(s *model.Span) SpanInfoResponse {
	var spanType string
	switch s.SpanType {
	case model.SpanOpening:
		spanType = "opening"
	case model.SpanClosing:
		spanType = "closing"
	case model.SpanPlaceholder:
		spanType = "placeholder"
	}
	return SpanInfoResponse{
		SpanType:    spanType,
		Type:        s.Type,
		ID:          s.ID,
		Data:        s.Data,
		SubType:     s.SubType,
		DisplayText: s.DisplayText,
		EquivText:   s.EquivText,
		Deletable:   s.Deletable,
		Cloneable:   s.Cloneable,
		CanReorder:  s.CanReorder,
	}
}

func editorInfoToSpan(si SpanInfoResponse) *model.Span {
	var st model.SpanType
	switch si.SpanType {
	case "opening":
		st = model.SpanOpening
	case "closing":
		st = model.SpanClosing
	case "placeholder":
		st = model.SpanPlaceholder
	}
	return &model.Span{
		SpanType:    st,
		Type:        si.Type,
		ID:          si.ID,
		Data:        si.Data,
		SubType:     si.SubType,
		DisplayText: si.DisplayText,
		EquivText:   si.EquivText,
		Deletable:   si.Deletable,
		Cloneable:   si.Cloneable,
		CanReorder:  si.CanReorder,
	}
}

func editorComputeStats(parts []*model.Part, targetLocale string) *TranslationStatsResponse {
	stats := &TranslationStatsResponse{}
	for _, pt := range parts {
		if pt.Type != model.PartBlock {
			continue
		}
		block, ok := pt.Resource.(*model.Block)
		if !ok || !block.Translatable {
			continue
		}
		stats.TotalBlocks++
		stats.WordCount += editorCountWords(block.SourceText())
		if block.TargetText(model.LocaleID(targetLocale)) != "" {
			stats.TranslatedBlocks++
		}
	}
	return stats
}

func editorCountWords(text string) int {
	count := 0
	inWord := false
	for _, r := range text {
		if unicode.IsSpace(r) {
			inWord = false
		} else if !inWord {
			inWord = true
			count++
		}
	}
	return count
}

func editorPseudoAccent(text string) string {
	var buf bytes.Buffer
	for _, r := range text {
		if replacement, ok := editorAccentMap[r]; ok {
			buf.WriteRune(replacement)
		} else {
			buf.WriteRune(r)
		}
	}
	return buf.String()
}

var editorAccentMap = map[rune]rune{
	'a': '\u00e0', 'b': '\u0180', 'c': '\u00e7', 'd': '\u00f0',
	'e': '\u00e9', 'f': '\u0192', 'g': '\u011d', 'h': '\u0125',
	'i': '\u00ee', 'j': '\u0135', 'k': '\u0137', 'l': '\u013c',
	'm': '\u1e3f', 'n': '\u00f1', 'o': '\u00f6', 'p': '\u00fe',
	'q': '\u01eb', 'r': '\u0155', 's': '\u0161', 't': '\u0163',
	'u': '\u00fb', 'v': '\u1e7d', 'w': '\u0175', 'x': '\u1e8b',
	'y': '\u00fd', 'z': '\u017e',
	'A': '\u00c0', 'B': '\u0181', 'C': '\u00c7', 'D': '\u00d0',
	'E': '\u00c9', 'F': '\u0191', 'G': '\u011c', 'H': '\u0124',
	'I': '\u00ce', 'J': '\u0134', 'K': '\u0136', 'L': '\u013b',
	'M': '\u1e3e', 'N': '\u00d1', 'O': '\u00d6', 'P': '\u00de',
	'Q': '\u01ea', 'R': '\u0154', 'S': '\u0160', 'T': '\u0162',
	'U': '\u00db', 'V': '\u1e7c', 'W': '\u0174', 'X': '\u1e8a',
	'Y': '\u00dd', 'Z': '\u017d',
}

func editorCreateProvider(provType, apiKey, modelName string) provider.LLMProvider {
	return credentials.NewProviderFromConfig(credentials.ProviderConfig{
		ProviderType: provType,
		Model:        modelName,
	}, apiKey)
}

func editorEntryToInfo(e sievepen.TMEntry) TMEntryInfoResponse {
	return TMEntryInfoResponse{
		ID:           e.ID,
		Source:       e.SourceText(),
		Target:       e.TargetText(),
		SourceLocale: string(e.SourceLocale),
		TargetLocale: string(e.TargetLocale),
		ProjectID:    e.ProjectID,
		UpdatedAt:    e.UpdatedAt.Format(time.RFC3339),
	}
}

func editorConceptToInfo(c termbase.Concept) ConceptInfoResponse {
	terms := make([]TermInfoResponse, len(c.Terms))
	for i, t := range c.Terms {
		terms[i] = TermInfoResponse{
			Text:         t.Text,
			Locale:       string(t.Locale),
			Status:       string(t.Status),
			PartOfSpeech: t.PartOfSpeech,
			Gender:       t.Gender,
			Note:         t.Note,
		}
	}
	return ConceptInfoResponse{
		ID:         c.ID,
		ProjectID:  c.ProjectID,
		Domain:     c.Domain,
		Definition: c.Definition,
		Terms:      terms,
		Properties: c.Properties,
		CreatedAt:  c.CreatedAt.Format(time.RFC3339),
		UpdatedAt:  c.UpdatedAt.Format(time.RFC3339),
	}
}

func editorTermsFromInfo(terms []TermInfoResponse) []termbase.Term {
	result := make([]termbase.Term, len(terms))
	for i, t := range terms {
		result[i] = termbase.Term{
			Text:         t.Text,
			Locale:       model.LocaleID(t.Locale),
			Status:       model.TermStatus(t.Status),
			PartOfSpeech: t.PartOfSpeech,
			Gender:       t.Gender,
			Note:         t.Note,
		}
		if result[i].Status == "" {
			result[i].Status = model.TermApproved
		}
	}
	return result
}
