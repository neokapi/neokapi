package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/labstack/echo/v4"

	"github.com/neokapi/neokapi/bowrain/billing"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/credentials"
	sqltm "github.com/neokapi/neokapi/bowrain/sievepen"
	"github.com/neokapi/neokapi/bowrain/storage"
	sqltb "github.com/neokapi/neokapi/bowrain/termbase"
	"github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/editor"
	"github.com/neokapi/neokapi/core/locale"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"github.com/neokapi/neokapi/sievepen"
	"github.com/neokapi/neokapi/termbase"
)

var (
	errNoPgDB = errors.New("PostgreSQL database not configured")
)

// ---------------------------------------------------------------------------
// Workspace TM/TB management (persistent, PostgreSQL-backed)
// ---------------------------------------------------------------------------

// workspaceTMTB holds workspace-scoped TM and terminology stores.
type workspaceTMTB struct {
	tm sievepen.TMStore
	tb termbase.TBStore
}

// workspaceStores manages per-workspace TM and terminology stores.
type workspaceStores struct {
	mu     sync.RWMutex
	stores map[string]*workspaceTMTB
	pgDB   *storage.PgDB // PostgreSQL database (required in production)

	// tmFactory and tbFactory are optional factory functions for creating
	// TM/TB stores without PostgreSQL. Used by tests to inject in-memory stores.
	tmFactory func() sievepen.TMStore
	tbFactory func() termbase.TBStore
}

func newWorkspaceStores() *workspaceStores {
	return &workspaceStores{
		stores: make(map[string]*workspaceTMTB),
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

func (ws *workspaceStores) getTM(wsSlug string) (sievepen.TMStore, error) {
	w := ws.getOrCreate(wsSlug)
	if w.tm != nil {
		return w.tm, nil
	}

	if ws.pgDB == nil {
		if ws.tmFactory != nil {
			w.tm = ws.tmFactory()
			return w.tm, nil
		}
		return nil, errNoPgDB
	}

	tm, err := sqltm.NewPostgresTMFromDB(ws.pgDB, wsSlug)
	if err != nil {
		return nil, err
	}
	w.tm = tm
	return tm, nil
}

func (ws *workspaceStores) getTB(wsSlug string) (termbase.TBStore, error) {
	w := ws.getOrCreate(wsSlug)
	if w.tb != nil {
		return w.tb, nil
	}

	if ws.pgDB == nil {
		if ws.tbFactory != nil {
			w.tb = ws.tbFactory()
			return w.tb, nil
		}
		return nil, errNoPgDB
	}

	tb, err := sqltb.NewPostgresTermBaseFromDB(ws.pgDB, wsSlug)
	if err != nil {
		return nil, err
	}
	w.tb = tb
	return tb, nil
}

// ---------------------------------------------------------------------------
// API response/request types
// ---------------------------------------------------------------------------

// ProjectInfoResponse is the API response for a translation project.
type ProjectInfoResponse struct {
	ID                    string                `json:"id"`
	Name                  string                `json:"name"`
	DefaultSourceLanguage string                `json:"default_source_language"`
	TargetLanguages       []string              `json:"target_languages"`
	TargetLanguageMode    string                `json:"target_language_mode"`
	DefaultStream         string                `json:"default_stream,omitempty"`
	DashboardVisibility   string                `json:"dashboard_visibility,omitempty"`
	Properties            map[string]string     `json:"properties,omitempty"`
	Items                 []ProjectItemResponse `json:"items"`
	Collections           []CollectionResponse  `json:"collections,omitempty"`
	Streams               []store.Stream        `json:"streams,omitempty"`
	ActiveStream          string                `json:"active_stream,omitempty"`
	CreatedAt             string                `json:"created_at"`
	ModifiedAt            string                `json:"modified_at"`
}

// ProjectItemResponse describes an item within a project.
type ProjectItemResponse struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Format       string `json:"format"`
	Type         string `json:"type"`
	CollectionID string `json:"collection_id,omitempty"`
	Size         int64  `json:"size"`
	BlockCount   int    `json:"block_count"`
	WordCount    int    `json:"word_count"`
}

// BlockInfoResponse is a serializable representation of a translatable block.
// Inline markup travels as RFC 0001 Run sequences (source_runs / targets_runs),
// the same content model the gRPC editor uses; there is no coded-text form.
type BlockInfoResponse struct {
	ID             string                 `json:"id"`
	Source         string                 `json:"source"`
	SourceRuns     []model.Run            `json:"source_runs,omitempty"`
	Targets        map[string]string      `json:"targets"`
	TargetsRuns    map[string][]model.Run `json:"targets_runs,omitempty"`
	Translatable   bool                   `json:"translatable"`
	HasInlineCodes bool                   `json:"has_inline_codes"`
	Properties     map[string]string      `json:"properties"`
	Entities       []EntityInfoResponse   `json:"entities,omitempty"`
}

// EntityInfoResponse represents an entity annotation on a block.
type EntityInfoResponse struct {
	Key    string `json:"key"` // annotation key (e.g. "entity:0")
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

// UpdateBlockTargetRunsRequest updates a block target from a Run sequence —
// the Run-native counterpart of UpdateBlockTargetRequest (which carries plain
// text). The runs are stored verbatim as the target's first segment.
type UpdateBlockTargetRunsRequest struct {
	TargetLocale string      `json:"target_locale"`
	Runs         []model.Run `json:"runs"`
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
	ID             string `json:"id"`
	Source         string `json:"source"`
	Target         string `json:"target"`
	SourceLanguage string `json:"source_language"`
	TargetLanguage string `json:"target_language"`
	ProjectID      string `json:"project_id,omitempty"`
	UpdatedAt      string `json:"updated_at"`
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

// streamParam extracts the active stream from the request.
// It checks the URL path param first (stream-scoped routes), then falls back
// to a query param, then defaults to "main".
// refParam extracts the ref (stream or tag) from the request.
// Bowrain AD-011: resource-first ref pattern — ref comes from :ref path param.
// Falls back to :stream (legacy) and ?stream= query param for backward compat.
func refParam(c echo.Context) string {
	if s := c.Param("ref"); s != "" {
		return s
	}
	if s := c.Param("stream"); s != "" {
		return s
	}
	if s := c.QueryParam("stream"); s != "" {
		return s
	}
	return "main"
}

// streamParam is an alias for refParam (backward compatibility).
func streamParam(c echo.Context) string {
	return refParam(c)
}

// refParamWithProject extracts the ref from the request,
// falling back to the project's configured default stream before "main".
func refParamWithProject(c echo.Context, p *store.Project) string {
	if s := c.Param("ref"); s != "" {
		return s
	}
	if s := c.Param("stream"); s != "" {
		return s
	}
	if s := c.QueryParam("stream"); s != "" {
		return s
	}
	if p != nil && p.DefaultStream != "" {
		return p.DefaultStream
	}
	return "main"
}

// streamParamWithProject is an alias for refParamWithProject (backward compatibility).
func streamParamWithProject(c echo.Context, p *store.Project) string {
	return refParamWithProject(c, p)
}

// projectParam extracts the project ID from either :id or :pid path parameter.
// Bowrain AD-011 uses :id consistently, but some handlers historically use :pid.
func projectParam(c echo.Context) string {
	if id := c.Param("id"); id != "" {
		return id
	}
	return c.Param("pid")
}

// ---------------------------------------------------------------------------
// ContentStore-backed editor operations
// ---------------------------------------------------------------------------

// editorCreateProject creates a new project in the ContentStore.
func editorCreateProject(ctx context.Context, cs store.ContentStore, ws, name, sourceLang string, targetLangs []string) (*ProjectInfoResponse, error) {
	if name == "" {
		return nil, errors.New("project name is required")
	}
	if sourceLang == "" {
		return nil, errors.New("source language is required")
	}
	if len(targetLangs) == 0 {
		return nil, errors.New("at least one target language is required")
	}

	locales := make([]model.LocaleID, len(targetLangs))
	for i, l := range targetLangs {
		locales[i] = model.LocaleID(l)
	}

	p := &store.Project{
		Name:                  name,
		DefaultSourceLanguage: model.LocaleID(sourceLang),
		TargetLanguages:       locales,
		WorkspaceID:           ws,
		Properties:            map[string]string{},
	}
	if err := cs.CreateProject(ctx, p); err != nil {
		return nil, fmt.Errorf("create project: %w", err)
	}

	// Create the default collection and main stream for the new project.
	_ = EnsureDefaultCollection(ctx, cs, p.ID)
	_ = EnsureMainStream(ctx, cs, p.ID)

	return projectToInfoResponse(p), nil
}

// editorAddFiles parses uploaded files, stores items and blocks in ContentStore.
func editorAddFiles(ctx context.Context, cs store.ContentStore, formatReg *registry.FormatRegistry, projectID, stream string, files map[string][]byte) (*ProjectInfoResponse, error) {
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

		reader, err := formatReg.NewReader(registry.FormatID(fmtName))
		if err != nil {
			continue
		}

		doc := &model.RawDocument{
			URI:          itemName,
			SourceLocale: proj.DefaultSourceLanguage,
			Encoding:     "UTF-8",
			Reader:       io.NopCloser(bytes.NewReader(data)),
		}

		result, err := editor.ParseItem(ctx, reader, doc, string(proj.DefaultSourceLanguage), fmtName, itemName)
		if err != nil {
			return nil, err
		}

		item := &store.Item{
			Name:        itemName,
			Format:      fmtName,
			ItemType:    "file",
			BlockIndex:  result.BlockIndexJSON,
			PreviewHTML: result.PreviewHTML,
			Properties:  map[string]string{},
		}
		if err := cs.StoreItem(ctx, projectID, stream, item); err != nil {
			return nil, fmt.Errorf("store item %q: %w", itemName, err)
		}

		if len(result.Blocks) > 0 {
			if err := cs.StoreBlocksForItem(ctx, projectID, stream, itemName, result.Blocks); err != nil {
				return nil, fmt.Errorf("store blocks for %q: %w", itemName, err)
			}
		}
	}

	return editorBuildProjectInfo(ctx, cs, proj, stream)
}

// editorAddFilesToCollection parses uploaded files and stores them in a specific collection.
func editorAddFilesToCollection(ctx context.Context, cs store.ContentStore, formatReg *registry.FormatRegistry, projectID, stream, collectionID string, files map[string][]byte) (*ProjectInfoResponse, error) {
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

		reader, err := formatReg.NewReader(registry.FormatID(fmtName))
		if err != nil {
			continue
		}

		doc := &model.RawDocument{
			URI:          itemName,
			SourceLocale: proj.DefaultSourceLanguage,
			Encoding:     "UTF-8",
			Reader:       io.NopCloser(bytes.NewReader(data)),
		}

		result, err := editor.ParseItem(ctx, reader, doc, string(proj.DefaultSourceLanguage), fmtName, itemName)
		if err != nil {
			return nil, err
		}

		item := &store.Item{
			Name:         itemName,
			Format:       fmtName,
			ItemType:     "file",
			CollectionID: collectionID,
			BlockIndex:   result.BlockIndexJSON,
			PreviewHTML:  result.PreviewHTML,
			Properties:   map[string]string{},
		}
		if err := cs.StoreItem(ctx, projectID, stream, item); err != nil {
			return nil, fmt.Errorf("store item %q: %w", itemName, err)
		}

		if len(result.Blocks) > 0 {
			if err := cs.StoreBlocksForItem(ctx, projectID, stream, itemName, result.Blocks); err != nil {
				return nil, fmt.Errorf("store blocks for %q: %w", itemName, err)
			}
		}
	}

	return editorBuildProjectInfo(ctx, cs, proj, stream)
}

// editorRemoveFile removes an item and its blocks from ContentStore.
func editorRemoveFile(ctx context.Context, cs store.ContentStore, projectID, stream, fileName string) (*ProjectInfoResponse, error) {
	proj, err := cs.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}

	if err := cs.DeleteItem(ctx, projectID, stream, fileName); err != nil {
		return nil, err
	}

	return editorBuildProjectInfo(ctx, cs, proj, stream)
}

// editorGetBlocks returns blocks for a specific item, formatted for the API.
func editorGetBlocks(ctx context.Context, cs store.ContentStore, projectID, stream, itemName string, targetLocales []string) ([]BlockInfoResponse, error) {
	storedBlocks, err := cs.GetBlocks(ctx, store.BlockQuery{
		ProjectID: projectID,
		Stream:    stream,
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
func editorUpdateBlockTarget(ctx context.Context, cs store.ContentStore, projectID, stream, blockID string, req UpdateBlockTargetRequest) error {
	sb, err := cs.GetBlock(ctx, projectID, stream, blockID)
	if err != nil {
		return err
	}

	sb.Block.SetTargetText(model.LocaleID(req.TargetLocale), req.Text)

	return cs.StoreBlocks(ctx, projectID, stream, []*model.Block{sb.Block})
}

// editorUpdateBlockTargetRuns loads a block, updates its target with the given
// Run sequence, and stores it back.
func editorUpdateBlockTargetRuns(ctx context.Context, cs store.ContentStore, projectID, stream, blockID string, req UpdateBlockTargetRunsRequest) error {
	sb, err := cs.GetBlock(ctx, projectID, stream, blockID)
	if err != nil {
		return err
	}

	sb.Block.SetTargetRuns(model.LocaleID(req.TargetLocale), req.Runs)

	return cs.StoreBlocks(ctx, projectID, stream, []*model.Block{sb.Block})
}

// editorPseudoTranslate pseudo-translates all blocks for an item.
func editorPseudoTranslate(ctx context.Context, cs store.ContentStore, projectID, stream, itemName, targetLocale string) (*TranslationStatsResponse, error) {
	storedBlocks, err := cs.GetBlocks(ctx, store.BlockQuery{
		ProjectID: projectID,
		Stream:    stream,
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
		seg := block.FirstSegment()
		if seg != nil && seg.HasInlineCodes() {
			block.SetTargetRuns(locale, editorPseudoRuns(seg.Runs))
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
		if err := cs.StoreBlocks(ctx, projectID, stream, blocks); err != nil {
			return nil, fmt.Errorf("store blocks: %w", err)
		}
	}

	return editorComputeStats(outParts, targetLocale), nil
}

// editorAITranslate translates blocks using an AI provider.
func editorAITranslate(ctx context.Context, cs store.ContentStore, projectID, stream, itemName string, req TranslateRequest, credStore *credentials.Store, billingHooks *billing.UsageHooks, workspaceID string) (*TranslationStatsResponse, error) {
	proj, err := cs.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}

	storedBlocks, err := cs.GetBlocks(ctx, store.BlockQuery{
		ProjectID: projectID,
		Stream:    stream,
		ItemName:  itemName,
	})
	if err != nil {
		return nil, err
	}

	parts := storedBlocksToParts(storedBlocks)

	var prov aiprovider.LLMProvider
	if req.ProviderConfigID != "" && credStore != nil {
		prov, err = credentials.NewProvider(credStore, req.ProviderConfigID)
		if err != nil {
			return nil, fmt.Errorf("resolve provider config: %w", err)
		}
	} else {
		prov = editorCreateProvider(req.Provider, req.APIKey, req.Model)
	}

	translateTool := tools.NewAITranslateTool(prov, tools.AITranslateConfig{
		SourceLocale:     proj.DefaultSourceLanguage,
		TargetLocale:     model.LocaleID(req.TargetLocale),
		BatchSize:        req.BatchSize,
		BatchConcurrency: req.Concurrency,
	})

	outParts, err := runToolOnParts(ctx, translateTool, parts)
	if err != nil {
		return nil, fmt.Errorf("AI translate: %w", err)
	}

	// Deduct billing credits based on actual token usage from the provider.
	if usage := translateTool.TotalUsage(); usage.TotalTokens() > 0 {
		billingHooks.DeductTokens(ctx, workspaceID, usage.TotalTokens(), "ai_translation", projectID)
	}

	blocks := partsToBlocks(outParts)
	if len(blocks) > 0 {
		if err := cs.StoreBlocks(ctx, projectID, stream, blocks); err != nil {
			return nil, fmt.Errorf("store blocks: %w", err)
		}
	}

	return editorComputeStats(outParts, req.TargetLocale), nil
}

// editorTMTranslate leverages translation memory to translate blocks.
func editorTMTranslate(ctx context.Context, cs store.ContentStore, wsStores *workspaceStores, ws, projectID, stream, itemName, targetLocale string) (*TranslationStatsResponse, error) {
	proj, err := cs.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}

	storedBlocks, err := cs.GetBlocks(ctx, store.BlockQuery{
		ProjectID: projectID,
		Stream:    stream,
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
		SourceLocale: proj.DefaultSourceLanguage,
		TargetLocale: model.LocaleID(targetLocale),
	})

	outParts, err := runToolOnParts(ctx, tmTool, parts)
	if err != nil {
		return nil, fmt.Errorf("TM translate: %w", err)
	}

	blocks := partsToBlocks(outParts)
	if len(blocks) > 0 {
		if err := cs.StoreBlocks(ctx, projectID, stream, blocks); err != nil {
			return nil, fmt.Errorf("store blocks: %w", err)
		}
	}

	return editorComputeStats(outParts, targetLocale), nil
}

// editorGetWordCount computes word/char counts from stored blocks.
func editorGetWordCount(ctx context.Context, cs store.ContentStore, projectID, stream, itemName string, targetLocales []string) (*WordCountResponse, error) {
	storedBlocks, err := cs.GetBlocks(ctx, store.BlockQuery{
		ProjectID: projectID,
		Stream:    stream,
		ItemName:  itemName,
	})
	if err != nil {
		return nil, err
	}

	result := &WordCountResponse{
		TargetWords: make(map[string]int, len(targetLocales)),
		TargetChars: make(map[string]int, len(targetLocales)),
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

// editorExportTranslatedFile is no longer supported server-side.
// Source bytes are no longer stored in the Item model. Use the CLI
// ('kapi pull') for translated file export.
func editorExportTranslatedFile(_ context.Context, _ store.ContentStore, _ *registry.FormatRegistry, _, _, itemName, _, _ string) (string, error) {
	return "", fmt.Errorf("server-side export not available for %q: use 'kapi pull' for translated file export", itemName)
}

// editorLookupTMForBlock looks up TM matches for a specific block.
func editorLookupTMForBlock(ctx context.Context, cs store.ContentStore, wsStores *workspaceStores, ws, projectID, stream, blockID, targetLocale string) ([]TMMatchInfoResponse, error) {
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

	sb, err := cs.GetBlock(ctx, projectID, stream, blockID)
	if err != nil {
		return nil, err
	}

	opts := sievepen.DefaultLookupOptions()
	opts.MaxResults = 5
	opts.ProjectID = projectID // for scoring boost
	matches, err := tm.Lookup(sb.Block, proj.DefaultSourceLanguage, model.LocaleID(targetLocale), opts)
	if err != nil {
		return nil, err
	}

	srcLoc := proj.DefaultSourceLanguage
	tgtLoc := model.LocaleID(targetLocale)
	result := make([]TMMatchInfoResponse, len(matches))
	for i, m := range matches {
		result[i] = TMMatchInfoResponse{
			Source:    m.Entry.VariantText(srcLoc),
			Target:    m.Entry.VariantText(tgtLoc),
			Score:     m.Score,
			MatchType: string(m.MatchType),
			ProjectID: m.Entry.ProjectID,
		}
	}
	return result, nil
}

// editorLookupTermsForBlock looks up term matches for a block.
func editorLookupTermsForBlock(ctx context.Context, cs store.ContentStore, wsStores *workspaceStores, ws, projectID, stream, blockID, targetLocale string) ([]BlockTermMatchResponse, error) {
	proj, err := cs.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}

	tb, err := wsStores.getTB(ws)
	if err != nil {
		return nil, fmt.Errorf("init termbase: %w", err)
	}
	if tb.Count() == 0 {
		return nil, nil
	}

	sb, err := cs.GetBlock(ctx, projectID, stream, blockID)
	if err != nil {
		return nil, err
	}

	sourceText := sb.Block.SourceText()
	if sourceText == "" {
		return nil, nil
	}

	matches := tb.LookupAll(sourceText, termbase.LookupOptions{
		SourceLocale: proj.DefaultSourceLanguage,
		TargetLocale: model.LocaleID(targetLocale),
		ProjectID:    projectID,
	})

	result := make([]BlockTermMatchResponse, 0, len(matches))
	for _, m := range matches {
		var targetTerms []string
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

// buildStreamChain resolves the parent chain for a stream by walking the
// ContentStore. Returns a slice of stream names from most specific to least
// (e.g., ["feature/rebrand", "main"]). For "main" or empty, returns ["main"].
func buildStreamChain(ctx context.Context, cs store.ContentStore, projectID, stream string) []string {
	if stream == "" || stream == "main" {
		return []string{"main"}
	}

	chain := []string{stream}
	visited := map[string]bool{stream: true}

	current := stream
	for {
		st, err := cs.GetStream(ctx, projectID, current)
		if err != nil || st.Parent == "" {
			// Add "main" as final fallback if not already there.
			if current != "main" {
				chain = append(chain, "main")
			}
			break
		}
		if visited[st.Parent] {
			break // avoid cycles
		}
		visited[st.Parent] = true
		chain = append(chain, st.Parent)
		current = st.Parent
	}

	return chain
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// fileParam extracts the item/file path from the request.
// Bowrain AD-011: uses ?item= query param. Falls back to wildcard (*) for legacy routes.
func fileParam(c echo.Context) string {
	if item := c.QueryParam("item"); item != "" {
		return item
	}
	return strings.TrimPrefix(c.Param("*"), "/")
}

// projectToInfoResponse converts a store.Project to ProjectInfoResponse.
func projectToInfoResponse(p *store.Project) *ProjectInfoResponse {
	locales := make([]string, len(p.TargetLanguages))
	for i, l := range p.TargetLanguages {
		locales[i] = string(l)
	}
	mode := p.TargetLanguageMode
	if mode == "" {
		mode = "defined"
	}
	return &ProjectInfoResponse{
		ID:                    p.ID,
		Name:                  p.Name,
		DefaultSourceLanguage: string(p.DefaultSourceLanguage),
		TargetLanguages:       locales,
		TargetLanguageMode:    mode,
		DefaultStream:         p.DefaultStream,
		DashboardVisibility:   p.DashboardVisibility,
		Properties:            p.Properties,
		Items:                 []ProjectItemResponse{},
		CreatedAt:             p.CreatedAt.Format(time.RFC3339),
		ModifiedAt:            p.UpdatedAt.Format(time.RFC3339),
	}
}

// editorBuildProjectInfo builds a full ProjectInfoResponse from store data.
func editorBuildProjectInfo(ctx context.Context, cs store.ContentStore, proj *store.Project, stream string) (*ProjectInfoResponse, error) {
	info := projectToInfoResponse(proj)

	items, err := cs.ListItems(ctx, proj.ID, stream)
	if err != nil {
		return nil, fmt.Errorf("list items: %w", err)
	}

	// Count items per collection for the response.
	itemCounts := map[string]int{}

	for _, item := range items {
		blocks, err := cs.GetBlocks(ctx, store.BlockQuery{
			ProjectID: proj.ID,
			Stream:    stream,
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
			ID:           item.ID,
			Name:         item.Name,
			Format:       item.Format,
			Type:         item.ItemType,
			CollectionID: item.CollectionID,
			Size:         0,
			BlockCount:   len(blocks),
			WordCount:    wordCount,
		})
		itemCounts[item.CollectionID]++
	}

	// Include collections in the response.
	colls, _ := cs.ListCollections(ctx, proj.ID, stream)
	for _, coll := range colls {
		cr := collectionToResponse(coll)
		cr.ItemCount = itemCounts[coll.ID]
		info.Collections = append(info.Collections, cr)
	}

	streams, _ := cs.ListStreams(ctx, proj.ID, false)
	if streams != nil {
		deref := make([]store.Stream, len(streams))
		for i, st := range streams {
			deref[i] = *st
		}
		info.Streams = deref
	}
	info.ActiveStream = stream

	return info, nil
}

// storedBlockToInfoResponse converts a StoredBlock to a BlockInfoResponse.
func storedBlockToInfoResponse(sb *store.StoredBlock, targetLocales []string) BlockInfoResponse {
	targets := make(map[string]string, len(targetLocales))
	for _, locale := range targetLocales {
		if t := sb.Block.TargetText(model.LocaleID(locale)); t != "" {
			targets[locale] = t
		}
	}

	props := make(map[string]string, len(sb.Block.Properties))
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
	if len(block.Source) == 0 || len(block.Source[0].Runs) == 0 {
		return
	}
	srcRuns := block.Source[0].Runs
	if !runsHaveInlineCodes(srcRuns) {
		// Plain-text blocks carry their content in Source/Targets already;
		// only blocks with inline markup need the Run sequences.
		return
	}

	bi.HasInlineCodes = true
	bi.SourceRuns = srcRuns

	bi.TargetsRuns = make(map[string][]model.Run, len(targetLocales))
	for _, locale := range targetLocales {
		segs, ok := block.Targets[model.LocaleID(locale)]
		if !ok || len(segs) == 0 || len(segs[0].Runs) == 0 {
			continue
		}
		bi.TargetsRuns[locale] = segs[0].Runs
	}
}

// runsHaveInlineCodes reports whether a Run sequence contains any non-text
// run (paired code, placeholder, sub, plural, or select).
func runsHaveInlineCodes(runs []model.Run) bool {
	for _, r := range runs {
		if r.Text == nil {
			return true
		}
	}
	return false
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

// editorPseudoRuns walks a Run sequence and applies pseudo-accent to TextRun
// content only, leaving inline-code runs untouched. The result is wrapped
// with `[` / `]` brackets at the boundaries (as plain TextRuns) to mirror
// the legacy coded-form pseudo behaviour.
func editorPseudoRuns(runs []model.Run) []model.Run {
	out := make([]model.Run, 0, len(runs)+2)
	out = append(out, model.Run{Text: &model.TextRun{Text: "["}})
	for _, r := range runs {
		if r.Text != nil {
			out = append(out, model.Run{Text: &model.TextRun{Text: editorPseudoAccent(r.Text.Text)}})
			continue
		}
		out = append(out, r)
	}
	out = append(out, model.Run{Text: &model.TextRun{Text: "]"}})
	return out
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

func editorCreateProvider(provType, apiKey, modelName string) aiprovider.LLMProvider {
	return credentials.NewProviderFromConfig(credentials.ProviderConfig{
		ProviderType: provType,
		Model:        modelName,
	}, apiKey)
}

// editorEntryToInfo projects a multilingual TMEntry onto a bilingual
// response view for the requested (src, tgt) locale pair. When the source
// is empty, it falls back to the entry's HintSrcLang. When the target is
// empty, it picks any other variant on the entry.
func editorEntryToInfo(e sievepen.TMEntry, sourceLocale, targetLocale string) TMEntryInfoResponse {
	srcLoc := model.LocaleID(sourceLocale)
	tgtLoc := model.LocaleID(targetLocale)
	if srcLoc == "" && e.HintSrcLang != "" {
		srcLoc = e.HintSrcLang
	}
	if tgtLoc == "" {
		for loc := range e.Variants {
			if loc != srcLoc {
				tgtLoc = loc
				break
			}
		}
	}
	return TMEntryInfoResponse{
		ID:             e.ID,
		Source:         e.VariantText(srcLoc),
		Target:         e.VariantText(tgtLoc),
		SourceLanguage: string(srcLoc),
		TargetLanguage: string(tgtLoc),
		ProjectID:      e.ProjectID,
		UpdatedAt:      e.UpdatedAt.Format(time.RFC3339),
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

// ---------------------------------------------------------------------------
// Translation Dashboard Stats
// ---------------------------------------------------------------------------

const dashboardCacheTTL = 30 * time.Second

// dashboardCacheEntry holds a cached dashboard stats result.
type dashboardCacheEntry struct {
	stats     *store.TranslationDashboardStats
	expiresAt time.Time
}

// dashboardCacheKey returns the cache key for a project/stream combination.
func dashboardCacheKey(workspaceID, projectID, stream string) string {
	return workspaceID + ":" + projectID + ":" + stream
}

// invalidateDashboardCache clears the dashboard cache for a project.
func (s *Server) invalidateDashboardCache(workspaceID, projectID string) {
	// Delete all stream variants for this project by iterating the cache.
	s.dashboardCache.Range(func(key, _ any) bool {
		k, ok := key.(string)
		if !ok {
			return true
		}
		prefix := workspaceID + ":" + projectID + ":"
		if strings.HasPrefix(k, prefix) {
			s.dashboardCache.Delete(key)
		}
		return true
	})
}

// editorGetDashboardStats computes aggregated translation stats for a project.
// Uses GetBlockStats for a lightweight single-query approach that avoids full
// block deserialization (no Span objects, Properties, or Annotations).
func editorGetDashboardStats(ctx context.Context, cs store.ContentStore, proj *store.Project, stream string) (*store.TranslationDashboardStats, error) {
	items, err := cs.ListItems(ctx, proj.ID, stream)
	if err != nil {
		return nil, fmt.Errorf("list items: %w", err)
	}

	colls, _ := cs.ListCollections(ctx, proj.ID, stream)
	collMap := map[string]string{} // id → name
	for _, c := range colls {
		collMap[c.ID] = c.Name
	}

	// Build item metadata lookup.
	type itemMeta struct {
		id           string
		format       string
		collectionID string
	}
	itemLookup := make(map[string]*itemMeta, len(items))
	for _, item := range items {
		itemLookup[item.Name] = &itemMeta{
			id:           item.ID,
			format:       item.Format,
			collectionID: item.CollectionID,
		}
	}

	targetLocales := make([]string, len(proj.TargetLanguages))
	for i, l := range proj.TargetLanguages {
		targetLocales[i] = string(l)
	}
	targetLocaleSet := make(map[string]bool, len(targetLocales))
	for _, l := range targetLocales {
		targetLocaleSet[l] = true
	}

	// Single lightweight query — no full block deserialization.
	blockStats, err := cs.GetBlockStats(ctx, proj.ID, stream)
	if err != nil {
		return nil, fmt.Errorf("get block stats: %w", err)
	}

	// Aggregators
	totalBlocks := len(blockStats)
	translatableBlocks := 0
	totalSourceWords := 0

	type localeAgg struct {
		translatedBlocks int
		totalBlocks      int
		translatedWords  int
		totalWords       int
	}
	newLocaleAggs := func() map[string]*localeAgg {
		m := make(map[string]*localeAgg, len(targetLocales))
		for _, l := range targetLocales {
			m[l] = &localeAgg{}
		}
		return m
	}

	globalLocaleAggs := newLocaleAggs()

	// Per-item aggregation
	type itemAgg struct {
		blockCount int
		wordCount  int
		locales    map[string]*localeAgg
	}
	itemAggs := make(map[string]*itemAgg, len(items))

	// Per-collection aggregation
	type collAgg struct {
		itemSet    map[string]bool
		blockCount int
		wordCount  int
		locales    map[string]*localeAgg
	}
	collAggs := map[string]*collAgg{}

	for _, bs := range blockStats {
		if !bs.Translatable {
			continue
		}
		translatableBlocks++
		wc := bs.SourceWords
		totalSourceWords += wc

		// Per-item accumulation
		ia, ok := itemAggs[bs.ItemName]
		if !ok {
			ia = &itemAgg{locales: newLocaleAggs()}
			itemAggs[bs.ItemName] = ia
		}
		ia.blockCount++
		ia.wordCount += wc

		// Build set of translated locales for this block
		translatedSet := make(map[string]bool, len(bs.TargetLocales))
		for _, l := range bs.TargetLocales {
			translatedSet[l] = true
		}

		for _, locale := range targetLocales {
			gla := globalLocaleAggs[locale]
			ila := ia.locales[locale]
			gla.totalBlocks++
			gla.totalWords += wc
			ila.totalBlocks++
			ila.totalWords += wc

			if translatedSet[locale] {
				gla.translatedBlocks++
				gla.translatedWords += wc
				ila.translatedBlocks++
				ila.translatedWords += wc
			}
		}

		// Per-collection accumulation
		meta := itemLookup[bs.ItemName]
		if meta == nil {
			continue
		}
		cid := meta.collectionID
		ca, ok := collAggs[cid]
		if !ok {
			ca = &collAgg{itemSet: map[string]bool{}, locales: newLocaleAggs()}
			collAggs[cid] = ca
		}
		ca.itemSet[bs.ItemName] = true
		ca.blockCount++
		ca.wordCount += wc
		for _, locale := range targetLocales {
			cla := ca.locales[locale]
			cla.totalBlocks++
			cla.totalWords += wc
			if translatedSet[locale] {
				cla.translatedBlocks++
				cla.translatedWords += wc
			}
		}
	}

	// Build per-item stats (preserve item order from ListItems).
	itemStats := make([]store.ItemTranslationStats, 0, len(items))
	for _, item := range items {
		ia := itemAggs[item.Name]
		itemLocales := make([]store.LocaleTranslationStats, 0, len(targetLocales))
		for _, l := range targetLocales {
			var ila *localeAgg
			if ia != nil {
				ila = ia.locales[l]
			}
			if ila == nil {
				ila = &localeAgg{}
			}
			pct := 0.0
			if ila.totalWords > 0 {
				pct = float64(ila.translatedWords) / float64(ila.totalWords) * 100
			}
			itemLocales = append(itemLocales, store.LocaleTranslationStats{
				Locale:           l,
				DisplayName:      locale.DisplayName(model.LocaleID(l)),
				TranslatedBlocks: ila.translatedBlocks,
				TotalBlocks:      ila.totalBlocks,
				TranslatedWords:  ila.translatedWords,
				TotalWords:       ila.totalWords,
				Percentage:       pct,
			})
		}
		bc, wc := 0, 0
		if ia != nil {
			bc, wc = ia.blockCount, ia.wordCount
		}
		itemStats = append(itemStats, store.ItemTranslationStats{
			ItemName:     item.Name,
			ItemID:       item.ID,
			Format:       item.Format,
			CollectionID: item.CollectionID,
			BlockCount:   bc,
			WordCount:    wc,
			Locales:      itemLocales,
		})
	}

	// Build global locale stats.
	localeStats := make([]store.LocaleTranslationStats, 0, len(targetLocales))
	for _, l := range targetLocales {
		la := globalLocaleAggs[l]
		pct := 0.0
		if la.totalWords > 0 {
			pct = float64(la.translatedWords) / float64(la.totalWords) * 100
		}
		localeStats = append(localeStats, store.LocaleTranslationStats{
			Locale:           l,
			DisplayName:      locale.DisplayName(model.LocaleID(l)),
			TranslatedBlocks: la.translatedBlocks,
			TotalBlocks:      la.totalBlocks,
			TranslatedWords:  la.translatedWords,
			TotalWords:       la.totalWords,
			Percentage:       pct,
		})
	}

	// Build collection stats.
	collStats := make([]store.CollectionTranslationStats, 0, len(collAggs))
	for cid, ca := range collAggs {
		cls := make([]store.LocaleTranslationStats, 0, len(targetLocales))
		for _, l := range targetLocales {
			cla := ca.locales[l]
			pct := 0.0
			if cla.totalWords > 0 {
				pct = float64(cla.translatedWords) / float64(cla.totalWords) * 100
			}
			cls = append(cls, store.LocaleTranslationStats{
				Locale:           l,
				DisplayName:      locale.DisplayName(model.LocaleID(l)),
				TranslatedBlocks: cla.translatedBlocks,
				TotalBlocks:      cla.totalBlocks,
				TranslatedWords:  cla.translatedWords,
				TotalWords:       cla.totalWords,
				Percentage:       pct,
			})
		}
		collStats = append(collStats, store.CollectionTranslationStats{
			CollectionID:   cid,
			CollectionName: collMap[cid],
			ItemCount:      len(ca.itemSet),
			BlockCount:     ca.blockCount,
			WordCount:      ca.wordCount,
			Locales:        cls,
		})
	}

	return &store.TranslationDashboardStats{
		LocaleStats:        localeStats,
		ItemStats:          itemStats,
		CollectionStats:    collStats,
		TotalBlocks:        totalBlocks,
		TranslatableBlocks: translatableBlocks,
		TotalSourceWords:   totalSourceWords,
	}, nil
}
