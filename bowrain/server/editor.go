package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/labstack/echo/v4"

	"github.com/neokapi/neokapi/bowrain/billing"
	"github.com/neokapi/neokapi/bowrain/core/brandscope"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/credentials"
	"github.com/neokapi/neokapi/bowrain/jobs"
	sqltm "github.com/neokapi/neokapi/bowrain/memory"
	"github.com/neokapi/neokapi/bowrain/resilience/aiguard"
	"github.com/neokapi/neokapi/bowrain/storage"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	sqltb "github.com/neokapi/neokapi/bowrain/terms"
	"github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/editor"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/locale"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/memory"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"github.com/neokapi/neokapi/terms"
)

var (
	errNoPgDB = errors.New("PostgreSQL database not configured")
)

// ---------------------------------------------------------------------------
// Workspace content memory/TB management (persistent, PostgreSQL-backed)
// ---------------------------------------------------------------------------

// workspaceMemoryTerms holds workspace-scoped content memory and terminology stores.
type workspaceMemoryTerms struct {
	tm memory.Store
	tb terms.Store
}

// workspaceStores manages per-workspace content memory and terminology stores.
type workspaceStores struct {
	mu     sync.RWMutex
	stores map[string]*workspaceMemoryTerms
	pgDB   *storage.PgDB // PostgreSQL database (required in production)

	// memoryFactory and tbFactory are optional factory functions for creating
	// content memory/TB stores without PostgreSQL. Used by tests to inject in-memory stores.
	memoryFactory func() memory.Store
	tbFactory     func() terms.Store
}

func newWorkspaceStores() *workspaceStores {
	return &workspaceStores{
		stores: make(map[string]*workspaceMemoryTerms),
	}
}

func (ws *workspaceStores) getOrCreate(wsSlug string) *workspaceMemoryTerms {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	w, ok := ws.stores[wsSlug]
	if !ok {
		w = &workspaceMemoryTerms{}
		ws.stores[wsSlug] = w
	}
	return w
}

func (ws *workspaceStores) getMemory(wsSlug string) (memory.Store, error) {
	w := ws.getOrCreate(wsSlug)
	if w.tm != nil {
		return w.tm, nil
	}

	if ws.pgDB == nil {
		if ws.memoryFactory != nil {
			w.tm = ws.memoryFactory()
			return w.tm, nil
		}
		return nil, errNoPgDB
	}

	tm, err := sqltm.NewPostgresStoreFromDB(ws.pgDB, wsSlug)
	if err != nil {
		return nil, err
	}
	w.tm = tm
	return tm, nil
}

func (ws *workspaceStores) getTB(wsSlug string) (terms.Store, error) {
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

	tb, err := sqltb.NewPostgresStoreFromDB(ws.pgDB, wsSlug)
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
	// Type is the project-type rollup over its collections' origins plus the
	// project-level source-connector signal: "connected" (all connector-sourced
	// — read-only source), "managed" (UI-native — editable), or "hybrid" (a
	// mix). Editable is the derived project-level gate. Both are filled by
	// annotateProjectOrigin on the project-detail read; the summary list view
	// leaves them empty.
	Type     string `json:"type,omitempty"`
	Editable bool   `json:"editable"`
	// Precomputed aggregates so list consumers (dashboard cards, stats bar)
	// never need the embedded arrays: total items, total blocks, translatable
	// source words, and stream count. The workspace project list serves these
	// in its (default) summary view with Items left empty; the single-project
	// response carries them alongside the full arrays.
	ItemCount   int `json:"item_count"`
	BlockCount  int `json:"block_count"`
	WordCount   int `json:"word_count"`
	StreamCount int `json:"stream_count"`
	// Skipped lists uploaded files that were NOT imported and why (unsupported
	// extension, no reader, parse failure). Only set on the upload responses;
	// absent/empty means every file imported.
	Skipped    []SkippedFileResponse `json:"skipped,omitempty"`
	CreatedAt  string                `json:"created_at"`
	ModifiedAt string                `json:"modified_at"`
}

// SkippedFileResponse names one uploaded file that was not imported and the
// reason it was skipped.
type SkippedFileResponse struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
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
//
// Targets carries, per locale, the committed target's plain text AND its
// per-locale review status (model.Target.Status — the ladder the convergence /
// coverage engine consumes), so the editor reads status as
// block.targets[locale].status. Blocks written before per-locale status carry
// only the legacy block-global Properties["translation-status"], which readers
// use as a fallback when targets[locale].status is empty.
type BlockInfoResponse struct {
	ID             string                     `json:"id"`
	Source         string                     `json:"source"`
	SourceRuns     []model.Run                `json:"source_runs,omitempty"`
	Targets        map[string]BlockTargetInfo `json:"targets"`
	TargetsRuns    map[string][]model.Run     `json:"targets_runs,omitempty"`
	Translatable   bool                       `json:"translatable"`
	HasInlineCodes bool                       `json:"has_inline_codes"`
	Properties     map[string]string          `json:"properties"`
	Entities       []EntityInfoResponse       `json:"entities,omitempty"`
}

// BlockTargetInfo is one locale's committed target in the blocks payload:
// plain text plus the target's lifecycle status ("" | draft | translated |
// reviewed | signed-off, model.TargetStatus).
type BlockTargetInfo struct {
	Text   string `json:"text"`
	Status string `json:"status,omitempty"`
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

// MemoryMatchInfoResponse is a content-memory match result.
type MemoryMatchInfoResponse struct {
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

// --- content memory types ---

// MemoryEntryInfoResponse is the API response for a content-memory entry.
type MemoryEntryInfoResponse struct {
	ID             string `json:"id"`
	Source         string `json:"source"`
	Target         string `json:"target"`
	SourceLanguage string `json:"source_language"`
	TargetLanguage string `json:"target_language"`
	ProjectID      string `json:"project_id,omitempty"`
	UpdatedAt      string `json:"updated_at"`
}

// MemorySearchResponse holds a page of content-memory search results.
type MemorySearchResponse struct {
	Entries    []MemoryEntryInfoResponse `json:"entries"`
	TotalCount int                       `json:"total_count"`
}

// MemoryAddRequest holds parameters for adding a content-memory entry.
type MemoryAddRequest struct {
	Source       string `json:"source"`
	Target       string `json:"target"`
	SourceLocale string `json:"source_locale"`
	TargetLocale string `json:"target_locale"`
	ProjectID    string `json:"project_id"` // which project to associate with
}

// MemoryUpdateRequest holds parameters for updating a content-memory entry.
type MemoryUpdateRequest struct {
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

// maxItemNameLen bounds an item name. Real names are repository-relative paths;
// anything approaching this is not one.
const maxItemNameLen = 1024

// safeItemName reports whether name may be stored as an item name.
//
// An item name is not just a label: it is a repository-relative path that the
// delivery side joins onto a checkout root to decide which file to write. It
// therefore has to be a path that stays inside the tree it is relative to.
// Nested names are ordinary and stay allowed — "locales/en/messages.json" is
// the common case, so filepath.Base would be wrong — but the name must be
// relative, canonical, and free of parent references.
//
// The rule is expressed in slash terms rather than the host's, because an item
// name means the same thing on every platform: it is stored on one OS and may
// be delivered on another. Requiring path.Clean to be a fixed point is what
// rejects "..", ".", doubled separators and trailing slashes in one test.
func safeItemName(name string) bool {
	if name == "" || len(name) > maxItemNameLen {
		return false
	}
	if strings.ContainsRune(name, 0) {
		return false
	}
	// A backslash is a separator on Windows and an ordinary character
	// elsewhere; refusing it keeps one stored name from meaning two things.
	if strings.ContainsRune(name, '\\') {
		return false
	}
	if path.IsAbs(name) || filepath.IsAbs(name) || filepath.VolumeName(name) != "" {
		return false
	}
	// Canonical form only: this is what rejects "./x", "a//b" and "a/b/".
	if path.Clean(name) != name {
		return false
	}
	// Clean keeps a leading ".." on a relative path — "../x" is already
	// canonical — so the escape has to be rejected on its own.
	return name != "." && name != ".." && !strings.HasPrefix(name, "../")
}

// editorAddFiles parses uploaded files, stores items and blocks in ContentStore.
// Files that cannot be imported (unknown extension, no reader, parse failure,
// or a name that is not a contained relative path) are reported per-file in the
// response's Skipped list rather than silently dropped or failing the whole
// batch; the importable files still land.
func editorAddFiles(ctx context.Context, cs store.ContentStore, formatReg *registry.FormatRegistry, projectID, stream string, files map[string][]byte) (*ProjectInfoResponse, error) {
	return editorAddFilesInternal(ctx, cs, formatReg, projectID, stream, "", files)
}

// editorAddFilesToCollection parses uploaded files and stores them in a specific
// collection. Per-file import failures are reported like editorAddFiles.
func editorAddFilesToCollection(ctx context.Context, cs store.ContentStore, formatReg *registry.FormatRegistry, projectID, stream, collectionID string, files map[string][]byte) (*ProjectInfoResponse, error) {
	return editorAddFilesInternal(ctx, cs, formatReg, projectID, stream, collectionID, files)
}

// editorAddFilesInternal is the shared upload path: it parses each file,
// stores item + blocks (tagged with collectionID when non-empty), collects
// per-file skip reasons, and returns the project info with Skipped set.
// Storage errors stay hard errors — they are infrastructure failures, not
// per-file input problems.
func editorAddFilesInternal(ctx context.Context, cs store.ContentStore, formatReg *registry.FormatRegistry, projectID, stream, collectionID string, files map[string][]byte) (*ProjectInfoResponse, error) {
	proj, err := cs.GetProject(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}

	var skipped []SkippedFileResponse
	skip := func(name, reason string) {
		skipped = append(skipped, SkippedFileResponse{Name: name, Reason: reason})
	}

	for itemName, data := range files {
		if !safeItemName(itemName) {
			skip(itemName, "invalid file name")
			continue
		}

		ext := filepath.Ext(itemName)
		fmtName, err := formatReg.Detector().DetectByExtension(ext)
		if err != nil {
			skip(itemName, fmt.Sprintf("unsupported file type %q: %s", ext, err))
			continue
		}

		reader, err := formatReg.NewReader(registry.FormatID(fmtName))
		if err != nil {
			skip(itemName, fmt.Sprintf("no reader for format %q: %s", fmtName, err))
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
			skip(itemName, fmt.Sprintf("parse as %s failed: %s", fmtName, err))
			continue
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

	info, err := editorBuildProjectInfo(ctx, cs, proj, stream)
	if err != nil {
		return nil, err
	}
	// Map iteration is unordered; sort for a deterministic response.
	sort.Slice(skipped, func(i, j int) bool { return skipped[i].Name < skipped[j].Name })
	info.Skipped = skipped
	return info, nil
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

	loc := model.LocaleID(req.TargetLocale)
	oldRuns := sb.Block.TargetRuns(loc)
	sb.Block.SetTargetText(loc, req.Text)
	demoteStaleReviewOnEdit(sb.Block, loc, oldRuns)

	return cs.StoreBlocks(ctx, projectID, stream, []*model.Block{sb.Block})
}

// editorUpdateBlockTargetRuns loads a block, updates its target with the given
// Run sequence, and stores it back.
func editorUpdateBlockTargetRuns(ctx context.Context, cs store.ContentStore, projectID, stream, blockID string, req UpdateBlockTargetRunsRequest) error {
	sb, err := cs.GetBlock(ctx, projectID, stream, blockID)
	if err != nil {
		return err
	}

	loc := model.LocaleID(req.TargetLocale)
	oldRuns := sb.Block.TargetRuns(loc)
	sb.Block.SetTargetRuns(loc, req.Runs)
	demoteStaleReviewOnEdit(sb.Block, loc, oldRuns)

	return cs.StoreBlocks(ctx, projectID, stream, []*model.Block{sb.Block})
}

// demoteStaleReviewOnEdit drops a reviewed/signed-off Target.Status back to
// translated when an edit actually changed the target's content. A review
// decision judges ONE specific translation, so rewriting the text invalidates
// the stale approval — the host review model binds every decision to the
// content hash of the translation it judges for exactly this reason
// (host/convergereport.go). Without the demotion, an edited-after-approval
// target would keep counting as reviewed in convergence/coverage and ship
// gates forever. Statuses at or below translated are left alone: they are not
// review decisions, and SetTargetText/SetTargetRuns deliberately preserve
// provenance.
func demoteStaleReviewOnEdit(b *model.Block, locale model.LocaleID, oldRuns []model.Run) {
	t := b.Target(locale)
	if t == nil {
		return
	}
	if t.Status != model.TargetStatusReviewed && t.Status != model.TargetStatusSignedOff {
		return
	}
	if reflect.DeepEqual(oldRuns, t.Runs) {
		return
	}
	t.Status = model.TargetStatusTranslated
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
	pseudoTool.Produce = func(v tool.VariantView) error {
		if !v.Translatable() {
			return nil
		}
		locale := model.LocaleID(targetLocale)
		runs := v.SourceRuns()
		if runsHaveInlineCodes(runs) {
			v.SetTargetRuns(locale, editorPseudoRuns(runs))
		} else {
			v.SetTargetText(locale, "["+editorPseudoAccent(v.SourceText())+"]")
		}
		return nil
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

// editorBrandContext bundles the optional stores the synchronous editor
// translate path uses to bind the project's standing brand context — the brand
// voice profile and the terminology glossary — mirroring the worker's
// WorkerDeps brand fields (jobs/brand_context.go). Every field is optional: a
// zero value translates bare, exactly as before.
type editorBrandContext struct {
	// Brand reads brand voice profiles. Nil translates without brand voice.
	Brand coreprofile.BrandStore
	// WorkspaceDefault resolves the workspace-level default brand-voice
	// profile — the base rung of the brandscope resolution ladder. Nil skips
	// the workspace rung.
	WorkspaceDefault brandscope.WorkspaceDefault
	// Stores yields the per-workspace terms the glossary derives from. Nil
	// translates without a glossary.
	Stores *workspaceStores
}

// editorTranslateConfig builds the AI translate tool config for the
// interactive editor path, binding the same standing brand context the worker
// binds via jobTranslateConfig (jobs/worker.go): the brand voice profile
// resolved through the brandscope ladder, the per-locale terminology glossary
// from the workspace terms, and the project's do-not-translate terms from
// settings. All bindings are best-effort — absence or a resolution failure
// leaves the field unset and the translation runs bare, never fails.
func editorTranslateConfig(
	ctx context.Context,
	cs store.ContentStore,
	brandCtx editorBrandContext,
	proj *store.Project,
	projectID, stream, workspaceID, workspaceSlug string,
	req TranslateRequest,
) tools.AITranslateConfig {
	cfg := tools.AITranslateConfig{
		SourceLocale:     proj.DefaultSourceLanguage,
		TargetLocale:     model.LocaleID(req.TargetLocale),
		BatchSize:        req.BatchSize,
		BatchConcurrency: req.Concurrency,
	}
	// Brand voice → prompt guidance, resolved through the platform's binding
	// ladder (collection → stream → project → workspace default).
	cfg.Profile = editorBrandProfile(ctx, cs, brandCtx, workspaceID, projectID, stream, cfg.TargetLocale)
	// Terminology → the advisory glossary section of the prompt, so the model
	// is told the mandated renderings at generation time instead of
	// term-enforce only flagging them afterwards.
	cfg.Glossary = editorGlossary(ctx, brandCtx, workspaceSlug, projectID, cfg.SourceLocale, cfg.TargetLocale)
	// Do-not-translate terms are ENFORCED, not merely prompted: the tool masks
	// each protected span before the model and restores it verbatim after, so a
	// product name / trademark / code identifier cannot be translated. Sourced
	// from project settings via the same derivation the worker uses
	// (jobs.ProjectDNTTerms), so an interactive translation protects exactly
	// the terms a batch job does. Empty settings simply leave the field unset.
	cfg.DNT = jobs.ProjectDNTTerms(proj)
	return cfg
}

// editorBrandProfile resolves the brand voice profile an editor translation
// should carry via the platform's hierarchical binding ladder — the same
// resolution the worker runs in resolveJobBrandProfile, scoped to the
// request's actual stream rather than the job model's fixed "main". Returns
// nil (and logs) on any resolution failure: brand voice must never fail an
// interactive translation.
func editorBrandProfile(
	ctx context.Context,
	cs store.ContentStore,
	brandCtx editorBrandContext,
	workspaceID, projectID, stream string,
	targetLocale model.LocaleID,
) *coreprofile.VoiceProfile {
	if brandCtx.Brand == nil {
		return nil
	}
	profile, err := brandscope.Resolve(ctx, cs, brandCtx.WorkspaceDefault, brandCtx.Brand, brandscope.Scope{
		WorkspaceID: workspaceID,
		ProjectID:   projectID,
		Stream:      stream,
		Locale:      targetLocale,
	})
	if err != nil {
		slog.WarnContext(ctx, "brand profile resolution failed; translating without brand voice",
			"project_id", projectID, "error", err)
		return nil
	}
	return profile
}

// editorGlossary builds the source→target glossary for an editor translation
// from the workspace terms, sharing the derivation with the worker
// (jobs.GlossaryFromTerms) so both surfaces mandate identical renderings.
// Returns nil (and logs) when no terms resolves or a read fails:
// terminology must never fail an interactive translation.
func editorGlossary(
	ctx context.Context,
	brandCtx editorBrandContext,
	workspaceSlug, projectID string,
	sourceLocale, targetLocale model.LocaleID,
) map[string]string {
	if brandCtx.Stores == nil || workspaceSlug == "" || sourceLocale == "" || targetLocale == "" {
		return nil
	}
	tb, err := brandCtx.Stores.getTB(workspaceSlug)
	if err != nil || tb == nil {
		if err != nil {
			slog.WarnContext(ctx, "terms resolution failed; translating without glossary",
				"workspace", workspaceSlug, "error", err)
		}
		return nil
	}
	glossary, err := jobs.GlossaryFromTerms(ctx, tb, projectID, sourceLocale, targetLocale)
	if err != nil {
		slog.WarnContext(ctx, "terms read failed; translating without glossary",
			"workspace", workspaceSlug, "error", err)
		return nil
	}
	return glossary
}

// editorAITranslate translates blocks using an AI provider.
//
// Provider resolution mirrors the worker (Epic 004): a saved provider_config_id
// resolves from the per-workspace Postgres ProviderStore, scoped to the caller's
// durable workspace id (never the keychain, which is empty in a headless
// container); an inline api_key/provider builds a one-off provider. Both the
// platform path and any bring-your-own path RECORD ai_usage (the monthly abuse
// cap must see all traffic), while only the platform path DEDUCTS credits.
//
// The translate config carries the project's standing brand context (voice
// profile + terminology glossary) via editorTranslateConfig, so an interactive
// per-block translation gets the same guidance an async worker job does.
func editorAITranslate(
	ctx context.Context,
	cs store.ContentStore,
	providerStore *bstore.ProviderConfigStore,
	quotaStore jobs.QuotaStore,
	projectID, stream, itemName string,
	req TranslateRequest,
	billingHooks *billing.UsageHooks,
	workspaceID, workspaceSlug string,
	platform jobs.PlatformProviderConfig,
	brandCtx editorBrandContext,
) (*TranslationStatsResponse, error) {
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

	// A saved provider_config_id is a bring-your-own key; an inline api_key is
	// too. Everything else is the platform path (metered in credits).
	byoSaved := req.ProviderConfigID != "" && req.ProviderConfigID != "platform"
	byo := byoSaved || req.APIKey != ""

	var prov aiprovider.LLMProvider
	if byoSaved {
		if providerStore == nil {
			return nil, errors.New("resolve provider config: provider store not configured")
		}
		cfg, cerr := providerStore.Resolve(ctx, workspaceID, req.ProviderConfigID)
		if cerr != nil {
			return nil, fmt.Errorf("resolve provider config: %w", cerr)
		}
		m := cfg.Model
		if m == "" {
			m = req.Model
		}
		prov, err = aiguard.NewProvider(aiprovider.ProviderID(cfg.Type), aiprovider.Config{
			APIKey:  cfg.APIKey,
			Model:   m,
			BaseURL: cfg.BaseURL,
		})
		if err != nil {
			return nil, fmt.Errorf("build provider %q: %w", cfg.Type, err)
		}
	} else if req.APIKey != "" {
		// Inline bring-your-own key.
		prov = editorCreateProvider(req.Provider, req.APIKey, req.Model)
	} else if platform.Provider != "" {
		// Platform path: the server chooses the AI backend (the hosted cloud's
		// Bedrock provider via the task role, no API key), ignoring any
		// client-named provider/model — the platform default wins, exactly as it
		// does for async jobs. Usage is metered in credits below.
		prov, _, err = platform.Build(req.Model)
		if err != nil {
			return nil, fmt.Errorf("build platform provider: %w", err)
		}
	} else {
		// No platform provider configured (self-hosted with no platform key):
		// fall back to the client-named provider, which may be a keyless local
		// backend (Ollama) or carry no key at all (demo/mock).
		prov = editorCreateProvider(req.Provider, req.APIKey, req.Model)
	}

	translateTool := tools.NewAITranslateTool(prov,
		editorTranslateConfig(ctx, cs, brandCtx, proj, projectID, stream, workspaceID, workspaceSlug, req))

	outParts, err := runToolOnParts(ctx, translateTool, parts)
	if err != nil {
		return nil, fmt.Errorf("AI translate: %w", err)
	}

	usage := translateTool.TotalUsage()

	// Record ai_usage for BOTH platform and BYO (mirror the worker): the monthly
	// DefaultMonthlyQuota abuse cap must see every AI call, including BYO ones
	// that burn no credits. Without this, the synchronous editor path was a blind
	// spot in the cap.
	if quotaStore != nil && usage.TotalTokens() > 0 {
		_ = quotaStore.RecordUsage(ctx, jobs.AIUsageRecord{
			WorkspaceSlug: workspaceSlug,
			WorkspaceID:   workspaceID,
			ProjectID:     projectID,
			Model:         req.Model,
			Operation:     "translate",
			PromptTokens:  usage.InputTokens,
			OutputTokens:  usage.OutputTokens,
			TotalTokens:   usage.TotalTokens(),
		})
	}

	// Deduct billing credits based on actual token usage from the provider — but
	// only for the platform-held key. A workspace bring-your-own key (an inline
	// api_key or a saved provider_config_id) burns NO credits (Epic 004): it is
	// capped via ai_usage above, not charged here. The reference id is unique per
	// deduction (project+item+locale+nonce) so distinct editor translations never
	// collapse into one Stripe meter event.
	if usage.TotalTokens() > 0 && !byo {
		refID := fmt.Sprintf("%s:%s:%s:%s", projectID, itemName, req.TargetLocale, id.New())
		billingHooks.DeductTokens(ctx, workspaceID, usage.TotalTokens(), "ai_translation", refID)
	}

	blocks := partsToBlocks(outParts)
	if len(blocks) > 0 {
		if err := cs.StoreBlocks(ctx, projectID, stream, blocks); err != nil {
			return nil, fmt.Errorf("store blocks: %w", err)
		}
	}

	return editorComputeStats(outParts, req.TargetLocale), nil
}

// editorMemoryTranslate leverages content memory to translate blocks.
func editorMemoryTranslate(ctx context.Context, cs store.ContentStore, wsStores *workspaceStores, ws, projectID, stream, itemName, targetLocale string) (*TranslationStatsResponse, error) {
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

	tm, err := wsStores.getMemory(ws)
	if err != nil {
		return nil, fmt.Errorf("init content memory: %w", err)
	}

	parts := storedBlocksToParts(storedBlocks)

	memoryTool := memory.NewMemoryLeverageTool(tm, memory.MemoryLeverageConfig{
		MinScore:     0.7,
		MaxResults:   5,
		SourceLocale: proj.DefaultSourceLanguage,
		TargetLocale: model.LocaleID(targetLocale),
	})

	outParts, err := runToolOnParts(ctx, memoryTool, parts)
	if err != nil {
		return nil, fmt.Errorf("content memory translate: %w", err)
	}

	blocks := partsToBlocks(outParts)
	if len(blocks) > 0 {
		if err := cs.StoreBlocks(ctx, projectID, stream, blocks); err != nil {
			return nil, fmt.Errorf("store blocks: %w", err)
		}
	}

	return editorComputeStats(outParts, targetLocale), nil
}

// TermEnforceResultResponse represents a terminology violation in a block —
// the API shape for POST /:ws/:id/actions/:ref/term-enforce. The server owns
// the check by running the framework terms.TermEnforceTool, so the desktop
// no longer hand-reimplements the matching logic.
type TermEnforceResultResponse struct {
	BlockID      string   `json:"block_id"`
	SourceTerm   string   `json:"source_term"`
	ConceptID    string   `json:"concept_id"`
	Expected     []string `json:"expected"`
	SourceText   string   `json:"source_text"`
	TargetText   string   `json:"target_text"`
	SourceLocale string   `json:"source_locale"`
	TargetLocale string   `json:"target_locale"`
}

// editorTermEnforce runs terminology enforcement over an item's blocks using
// the framework term-enforce tool and returns the violations found. It is a
// read-only check: blocks are annotated in memory but not persisted.
func editorTermEnforce(ctx context.Context, cs store.ContentStore, wsStores *workspaceStores, ws, projectID, stream, itemName, targetLocale string) ([]TermEnforceResultResponse, error) {
	proj, err := cs.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}

	tb, err := wsStores.getTB(ws)
	if err != nil {
		return nil, fmt.Errorf("init terms: %w", err)
	}
	if count, cerr := tb.Count(ctx); cerr != nil {
		return nil, cerr
	} else if count == 0 {
		return nil, nil
	}

	storedBlocks, err := cs.GetBlocks(ctx, store.BlockQuery{
		ProjectID: projectID,
		Stream:    stream,
		ItemName:  itemName,
	})
	if err != nil {
		return nil, err
	}

	srcLocale := proj.DefaultSourceLanguage
	tgtLocale := model.LocaleID(targetLocale)

	parts := storedBlocksToParts(storedBlocks)
	enforceTool := terms.NewTermEnforceTool(tb, terms.TermEnforceConfig{
		SourceLocale: srcLocale,
		TargetLocale: tgtLocale,
	})
	outParts, err := runToolOnParts(ctx, enforceTool, parts)
	if err != nil {
		return nil, fmt.Errorf("term-enforce: %w", err)
	}

	var results []TermEnforceResultResponse
	for _, block := range partsToBlocks(outParts) {
		for _, v := range terms.ViolationsFromBlock(block) {
			results = append(results, TermEnforceResultResponse{
				BlockID:      block.ID,
				SourceTerm:   v.SourceTerm,
				ConceptID:    v.ConceptID,
				Expected:     v.Expected,
				SourceText:   block.SourceText(),
				TargetText:   block.TargetText(tgtLocale),
				SourceLocale: string(srcLocale),
				TargetLocale: string(tgtLocale),
			})
		}
	}
	return results, nil
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

// editorLookupMemoryForBlock looks up content-memory matches for a specific block.
func editorLookupMemoryForBlock(ctx context.Context, cs store.ContentStore, wsStores *workspaceStores, ws, projectID, stream, blockID, targetLocale string) ([]MemoryMatchInfoResponse, error) {
	proj, err := cs.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}

	tm, err := wsStores.getMemory(ws)
	if err != nil {
		return nil, fmt.Errorf("init content memory: %w", err)
	}
	if count, err := tm.Count(ctx); err != nil {
		return nil, err
	} else if count == 0 {
		return nil, nil
	}

	sb, err := cs.GetBlock(ctx, projectID, stream, blockID)
	if err != nil {
		return nil, err
	}

	opts := memory.DefaultLookupOptions()
	opts.MaxResults = 5
	opts.ProjectID = projectID // for scoring boost
	matches, err := tm.Lookup(ctx, sb.Block, proj.DefaultSourceLanguage, model.LocaleID(targetLocale), opts)
	if err != nil {
		return nil, err
	}

	srcLoc := proj.DefaultSourceLanguage
	tgtLoc := model.LocaleID(targetLocale)
	result := make([]MemoryMatchInfoResponse, len(matches))
	for i, m := range matches {
		result[i] = MemoryMatchInfoResponse{
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
		return nil, fmt.Errorf("init terms: %w", err)
	}
	if count, err := tb.Count(ctx); err != nil {
		return nil, err
	} else if count == 0 {
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

	matches, err := tb.LookupAll(ctx, sourceText, terms.LookupOptions{
		SourceLocale: proj.DefaultSourceLanguage,
		TargetLocale: model.LocaleID(targetLocale),
		ProjectID:    projectID,
	})
	if err != nil {
		return nil, err
	}

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
// editorBuildProjectItems loads a project's items with per-item block and
// word counts, plus the per-collection item tally. Shared by the single-project
// info builder and the workspace project list (the dashboard cards need the
// same counts).
func editorBuildProjectItems(ctx context.Context, cs store.ContentStore, projID, stream string) ([]ProjectItemResponse, map[string]int, error) {
	items, err := cs.ListItems(ctx, projID, stream)
	if err != nil {
		return nil, nil, fmt.Errorf("list items: %w", err)
	}

	itemCounts := map[string]int{}
	out := make([]ProjectItemResponse, 0, len(items))

	for _, item := range items {
		blocks, err := cs.GetBlocks(ctx, store.BlockQuery{
			ProjectID: projID,
			Stream:    stream,
			ItemName:  item.Name,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("get blocks for %q: %w", item.Name, err)
		}

		wordCount := 0
		for _, sb := range blocks {
			if sb.Block.Translatable {
				wordCount += editorCountWords(sb.Block.SourceText())
			}
		}

		out = append(out, ProjectItemResponse{
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
	return out, itemCounts, nil
}

// editorBuildProjectSummary computes the per-project aggregates for the
// workspace project list without materializing (or shipping) the per-item
// arrays: item count, total block count, and translatable source words. It
// uses GetBlockStats — the same lightweight projection the translation
// dashboard uses — instead of a full GetBlocks per item, so no targets,
// properties, or overlays are deserialized.
func editorBuildProjectSummary(ctx context.Context, cs store.ContentStore, projID, stream string) (itemCount, blockCount, wordCount int, err error) {
	items, err := cs.ListItems(ctx, projID, stream)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("list items: %w", err)
	}
	stats, err := cs.GetBlockStats(ctx, projID, stream)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("get block stats: %w", err)
	}
	for _, bs := range stats {
		blockCount++
		if bs.Translatable {
			wordCount += bs.SourceWords
		}
	}
	return len(items), blockCount, wordCount, nil
}

func editorBuildProjectInfo(ctx context.Context, cs store.ContentStore, proj *store.Project, stream string) (*ProjectInfoResponse, error) {
	info := projectToInfoResponse(proj)

	items, itemCounts, err := editorBuildProjectItems(ctx, cs, proj.ID, stream)
	if err != nil {
		return nil, err
	}
	info.Items = items
	info.ItemCount = len(items)
	for _, it := range items {
		info.BlockCount += it.BlockCount
		info.WordCount += it.WordCount
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
	info.StreamCount = len(streams)
	info.ActiveStream = stream

	return info, nil
}

// storedBlockToInfoResponse converts a StoredBlock to a BlockInfoResponse.
func storedBlockToInfoResponse(sb *store.StoredBlock, targetLocales []string) BlockInfoResponse {
	targets := make(map[string]BlockTargetInfo, len(targetLocales))
	for _, locale := range targetLocales {
		loc := model.LocaleID(locale)
		text := sb.Block.TargetText(loc)
		status := ""
		if t := sb.Block.Target(loc); t != nil {
			status = string(t.Status)
		}
		if text != "" || status != "" {
			targets[locale] = BlockTargetInfo{Text: text, Status: status}
		}
	}

	props := make(map[string]string, len(sb.Block.Properties))
	maps.Copy(props, sb.Block.Properties)

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
	srcRuns := block.SourceRuns()
	if len(srcRuns) == 0 {
		return
	}
	if !runsHaveInlineCodes(srcRuns) {
		// Plain-text blocks carry their content in Source/Targets already;
		// only blocks with inline markup need the Run sequences.
		return
	}

	bi.HasInlineCodes = true
	bi.SourceRuns = srcRuns

	bi.TargetsRuns = make(map[string][]model.Run, len(targetLocales))
	for _, locale := range targetLocales {
		runs := block.TargetRuns(model.LocaleID(locale))
		if len(runs) == 0 {
			continue
		}
		bi.TargetsRuns[locale] = runs
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

// enrichBlockEntities extracts entity annotations from a block's entity overlay
// (positional spans: span.Range is the position, span.ID the identity).
func enrichBlockEntities(bi *BlockInfoResponse, block *model.Block) {
	f := block.OverlayOf(model.OverlayEntity)
	if f == nil {
		return
	}
	for _, span := range f.Spans {
		a, ok := span.Value.(*model.EntityAnnotation)
		if !ok {
			continue
		}
		start, end := span.Range.ByteSpan(block.Source)
		bi.Entities = append(bi.Entities, EntityInfoResponse{
			Key:    span.ID,
			Text:   a.Text,
			Type:   string(a.Type),
			Start:  start,
			End:    end,
			DNT:    a.DNT,
			Source: string(a.Source),
			Locale: string(a.Locale),
		})
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
//
// Process runs in its own goroutine while the caller drains the output channel
// concurrently, so a fan-out tool that emits more parts than it consumes cannot
// deadlock on a bounded buffer.
func runToolOnParts(ctx context.Context, t tool.Tool, parts []*model.Part) ([]*model.Part, error) {
	in := make(chan *model.Part, len(parts))
	out := make(chan *model.Part, len(parts))
	for _, pt := range parts {
		in <- pt
	}
	close(in)

	errCh := make(chan error, 1)
	go func() {
		err := t.Process(ctx, in, out)
		close(out)
		errCh <- err
	}()

	var result []*model.Part
	for pt := range out {
		result = append(result, pt)
	}
	if err := <-errCh; err != nil {
		return nil, err
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

// platformProviderConfig returns the server's configured platform AI backend for
// the synchronous editor platform path, mirroring the worker's BOWRAIN_PLATFORM_*
// selection so interactive and async translation resolve the same upstream. The
// zero value (Provider == "") means no platform provider is configured.
//
// Values are read from the instance-wide platform_config service (ctrl-managed),
// which falls back to the BOWRAIN_PLATFORM_* env bootstrap defaults when unset —
// so an admin switching provider/model in ctrl takes effect on the next request
// without a redeploy, while an un-provisioned instance behaves exactly as before.
func (s *Server) platformProviderConfig() jobs.PlatformProviderConfig {
	return jobs.PlatformProviderConfig{
		Provider: s.PlatformConfig.AIProvider(),
		Model:    s.PlatformConfig.AIDefaultModel(),
		BaseURL:  s.PlatformConfig.AIBaseURL(),
	}
}

// platformProviderConfigForWorkspace is platformProviderConfig with the
// workspace's customer-chosen model applied when the admin has opened model
// choice to customers. The provider and base URL stay platform-owned; only the
// model may vary per workspace, and only to a model the admin has enabled. Any
// lookup failure or ineligible selection silently falls back to the platform
// default, so translation never breaks on a stale or disallowed preference.
func (s *Server) platformProviderConfigForWorkspace(ctx context.Context, wsID string) jobs.PlatformProviderConfig {
	cfg := s.platformProviderConfig()
	if wsID == "" || s.AuthStore == nil || !s.PlatformConfig.AICustomerChoice() {
		return cfg
	}
	w, err := s.AuthStore.GetWorkspace(ctx, wsID)
	if err != nil || w == nil {
		return cfg
	}
	cfg.Model = s.PlatformConfig.ResolveWorkspaceModel(w.PreferredModel)
	return cfg
}

// editorEntryToInfo projects a multilingual Entry onto a bilingual
// response view for the requested (src, tgt) locale pair. When the source
// is empty, it falls back to the entry's HintSrcLang. When the target is
// empty, it picks any other variant on the entry.
func editorEntryToInfo(e memory.Entry, sourceLocale, targetLocale string) MemoryEntryInfoResponse {
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
	return MemoryEntryInfoResponse{
		ID:             e.ID,
		Source:         e.VariantText(srcLoc),
		Target:         e.VariantText(tgtLoc),
		SourceLanguage: string(srcLoc),
		TargetLanguage: string(tgtLoc),
		ProjectID:      e.ProjectID,
		UpdatedAt:      e.UpdatedAt.Format(time.RFC3339),
	}
}

func editorConceptToInfo(c terms.Concept) ConceptInfoResponse {
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

func editorTermsFromInfo(infos []TermInfoResponse) []terms.Term {
	result := make([]terms.Term, len(infos))
	for i, t := range infos {
		result[i] = terms.Term{
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
		approvedBlocks   int
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

		// Build sets of translated / review-approved locales for this block
		translatedSet := make(map[string]bool, len(bs.TargetLocales))
		for _, l := range bs.TargetLocales {
			translatedSet[l] = true
		}
		approvedSet := make(map[string]bool, len(bs.ApprovedLocales))
		for _, l := range bs.ApprovedLocales {
			approvedSet[l] = true
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
			if approvedSet[locale] {
				gla.approvedBlocks++
				ila.approvedBlocks++
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
			if approvedSet[locale] {
				cla.approvedBlocks++
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
				ApprovedBlocks:   ila.approvedBlocks,
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
			ApprovedBlocks:   la.approvedBlocks,
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
				ApprovedBlocks:   cla.approvedBlocks,
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
		ItemTotal:          len(itemStats),
		CollectionStats:    collStats,
		TotalBlocks:        totalBlocks,
		TranslatableBlocks: translatableBlocks,
		TotalSourceWords:   totalSourceWords,
	}, nil
}

// dashboardItemSortKey returns the sortable value(s) for one item under the
// given column: name (case-insensitive), words (word_count), or completion
// (average percentage across the project's target locales — the same average
// the file-progress table displays).
func dashboardItemCompletion(it store.ItemTranslationStats) float64 {
	if len(it.Locales) == 0 {
		return 0
	}
	sum := 0.0
	for _, l := range it.Locales {
		sum += l.Percentage
	}
	return sum / float64(len(it.Locales))
}

// sortDashboardItems sorts a copy-safe slice of item stats by the requested
// column and direction. Ties fall back to the item name so paging is stable.
func sortDashboardItems(items []store.ItemTranslationStats, field, dir string) {
	desc := dir == "desc"
	less := func(a, b store.ItemTranslationStats) bool {
		switch field {
		case "words":
			if a.WordCount != b.WordCount {
				return a.WordCount < b.WordCount
			}
		case "completion":
			ca, cb := dashboardItemCompletion(a), dashboardItemCompletion(b)
			if ca != cb {
				return ca < cb
			}
		}
		return strings.ToLower(a.ItemName) < strings.ToLower(b.ItemName)
	}
	sort.SliceStable(items, func(i, j int) bool {
		if desc {
			return less(items[j], items[i])
		}
		return less(items[i], items[j])
	})
}

// pageDashboardStats returns a shallow copy of stats with ItemStats sorted and
// sliced to the requested window. The input (typically the cached full result)
// is never mutated. limit <= 0 with an empty sort keeps the legacy full,
// ListItems-ordered response.
func pageDashboardStats(stats *store.TranslationDashboardStats, limit, offset int, sortField, dir string) *store.TranslationDashboardStats {
	if limit <= 0 && sortField == "" {
		return stats
	}
	resp := *stats
	items := make([]store.ItemTranslationStats, len(stats.ItemStats))
	copy(items, stats.ItemStats)
	if sortField == "" {
		sortField = "name"
	}
	sortDashboardItems(items, sortField, dir)
	if offset < 0 {
		offset = 0
	}
	if offset > len(items) {
		offset = len(items)
	}
	items = items[offset:]
	if limit > 0 && limit < len(items) {
		items = items[:limit]
	}
	resp.ItemStats = items
	return &resp
}
