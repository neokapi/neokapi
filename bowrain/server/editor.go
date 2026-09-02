package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/url"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/neokapi/neokapi/bowrain/billing"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/core/voicescope"
	"github.com/neokapi/neokapi/bowrain/credentials"
	"github.com/neokapi/neokapi/bowrain/jobs"
	sqlmemory "github.com/neokapi/neokapi/bowrain/memory"
	"github.com/neokapi/neokapi/bowrain/resilience/aiguard"
	"github.com/neokapi/neokapi/bowrain/storage"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	sqlterms "github.com/neokapi/neokapi/bowrain/terms"
	"github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/editor"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/locale"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	libtools "github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/memory/leverage"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"github.com/neokapi/neokapi/terms"
)

var (
	errNoPgDB = errors.New("PostgreSQL database not configured")
)

// ---------------------------------------------------------------------------
// Workspace content memory / terms management (persistent, PostgreSQL-backed)
// ---------------------------------------------------------------------------

// workspaceMemoryTerms holds one workspace's content memory and terms stores.
//
// Both are opened lazily, on the first request that needs them, and the entry
// then caches them for the process's lifetime. mu guards that opening: it is
// held across the nil check and the assignment, so two concurrent requests for
// the same workspace open one store between them rather than each opening its
// own and racing to publish it.
//
// The lock is per entry rather than the map's own lock because opening a
// PostgreSQL-backed store runs migrations — holding workspaceStores.mu for that
// would stall every other workspace's lookup behind one workspace's first use.
type workspaceMemoryTerms struct {
	mu     sync.Mutex
	memory memory.Store
	terms  terms.Store
}

// workspaceStores manages per-workspace content memory and terms stores.
type workspaceStores struct {
	// mu guards stores, the slug → entry map. It is released before the entry
	// itself is filled; see workspaceMemoryTerms.
	mu     sync.RWMutex
	stores map[string]*workspaceMemoryTerms
	pgDB   *storage.PgDB // PostgreSQL database (required in production)

	// memoryFactory and termsFactory are optional factory functions for
	// creating content memory / terms stores without PostgreSQL. Used by tests
	// to inject in-memory stores. They are installed during setup, before the
	// server serves, and never written afterwards.
	memoryFactory func() memory.Store
	termsFactory  func() terms.Store
}

func newWorkspaceStores() *workspaceStores {
	return &workspaceStores{
		stores: make(map[string]*workspaceMemoryTerms),
	}
}

func (ws *workspaceStores) getOrCreate(wsSlug string) *workspaceMemoryTerms {
	ws.mu.RLock()
	w, ok := ws.stores[wsSlug]
	ws.mu.RUnlock()
	if ok {
		return w
	}

	ws.mu.Lock()
	defer ws.mu.Unlock()
	// Another caller may have created the entry between the two locks; the
	// entry has to be unique per slug or the caching below is per-caller.
	if w, ok := ws.stores[wsSlug]; ok {
		return w
	}
	w = &workspaceMemoryTerms{}
	ws.stores[wsSlug] = w
	return w
}

func (ws *workspaceStores) getMemory(wsSlug string) (memory.Store, error) {
	w := ws.getOrCreate(wsSlug)
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.memory != nil {
		return w.memory, nil
	}

	if ws.pgDB == nil {
		if ws.memoryFactory != nil {
			w.memory = ws.memoryFactory()
			return w.memory, nil
		}
		return nil, errNoPgDB
	}

	opened, err := sqlmemory.NewPostgresStoreFromDB(ws.pgDB, wsSlug)
	if err != nil {
		return nil, err
	}
	w.memory = opened
	return opened, nil
}

func (ws *workspaceStores) getTerms(wsSlug string) (terms.Store, error) {
	w := ws.getOrCreate(wsSlug)
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.terms != nil {
		return w.terms, nil
	}

	if ws.pgDB == nil {
		if ws.termsFactory != nil {
			w.terms = ws.termsFactory()
			return w.terms, nil
		}
		return nil, errNoPgDB
	}

	opened, err := sqlterms.NewPostgresStoreFromDB(ws.pgDB, wsSlug)
	if err != nil {
		return nil, err
	}
	w.terms = opened
	return opened, nil
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
	ID     string `json:"id"`
	Name   string `json:"name"`
	Format string `json:"format"`
	// SourcePath is the file this item's content was lifted out of, when the
	// item is a generated catalog rather than the source itself. What a list
	// SHOWS; Name stays what everything ADDRESSES. Empty for an item that is
	// its own source, which is most of them.
	SourcePath   string `json:"source_path,omitempty"`
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
	ID string `json:"id"`
	// SourceID is the id the format reader gave this block inside its item —
	// the id the document itself carries. The store mints its own id on
	// ingest, so a surface holding a block and a surface holding the
	// document are naming the same content two different ways, and anything
	// addressed across that boundary has to translate. Empty for a block
	// stored without an item, where there is no document to disagree with.
	SourceID       string                     `json:"source_id,omitempty"`
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
//
// Path params are unescaped here because Echo hands them over as they appear
// in the URL. A ref with a slash — every `$auto` stream named after a
// git branch like fix/thing — MUST arrive percent-encoded to route at all, and
// taking it raw stored "fix%2Fthing" as a stream name.
func refParam(c echo.Context) string {
	if s := unescapeParam(c.Param("ref")); s != "" {
		return s
	}
	if s := unescapeParam(c.Param("stream")); s != "" {
		return s
	}
	if s := c.QueryParam("stream"); s != "" {
		return s
	}
	return "main"
}

// unescapeParam decodes a percent-encoded path parameter, returning the raw
// value when it is not valid percent-encoding (a literal "%" in a name must
// not make the request unroutable).
func unescapeParam(s string) string {
	if u, err := url.PathUnescape(s); err == nil {
		return u
	}
	return s
}

// streamParam is an alias for refParam (backward compatibility).
func streamParam(c echo.Context) string {
	return refParam(c)
}

// refParamWithProject extracts the ref from the request,
// falling back to the project's configured default stream before "main".
func refParamWithProject(c echo.Context, p *store.Project) string {
	if s := unescapeParam(c.Param("ref")); s != "" {
		return s
	}
	if s := unescapeParam(c.Param("stream")); s != "" {
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
func editorGetBlocks(ctx context.Context, cs store.ContentStore, projectID, stream, itemName string, targetLocales []string, limit, offset int) ([]BlockInfoResponse, error) {
	return editorQueryBlocks(ctx, cs, store.BlockQuery{
		ProjectID: projectID,
		Stream:    stream,
		ItemName:  itemName,
		Limit:     limit,
		Offset:    offset,
	}, targetLocales)
}

// editorQueryBlocks runs an arbitrary block query and formats the page for the
// API — the filtered form of editorGetBlocks, used by the surfaces that narrow
// by status, locale or search text.
func editorQueryBlocks(ctx context.Context, cs store.ContentStore, query store.BlockQuery, targetLocales []string) ([]BlockInfoResponse, error) {
	storedBlocks, err := cs.GetBlocks(ctx, query)
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

	applyTargetTextEdit(sb.Block, model.LocaleID(req.TargetLocale), req.Text)
	return cs.StoreBlocks(ctx, projectID, stream, []*model.Block{sb.Block})
}

// applyTargetTextEdit writes text as the block's target for locale, demoting a
// review the edit invalidates. Every path that edits target text in place goes
// through it, so none can skip the demotion.
func applyTargetTextEdit(b *model.Block, loc model.LocaleID, text string) {
	oldRuns := b.TargetRuns(loc)
	b.SetTargetText(loc, text)
	demoteStaleReviewOnEdit(b, loc, oldRuns)
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

	pseudoTool := libtools.NewPseudoTranslateTool(&libtools.PseudoConfig{
		TargetLocale: model.LocaleID(targetLocale),
	})

	outParts, err := tool.RunOnParts(ctx, pseudoTool, parts)
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

// editorVoiceContext bundles the optional stores the synchronous editor
// translate and check paths use to bind the project's standing context — the
// voice profile, the term rules and the content memory — mirroring the
// worker's WorkerDeps context fields (jobs/voice_context.go). Every field is
// optional: a zero value translates bare, exactly as before.
type editorVoiceContext struct {
	// Voice reads voice profiles. Nil translates without voice.
	Voice coreprofile.Store
	// WorkspaceDefault resolves the workspace-level default voice
	// profile — the base rung of the voicescope resolution ladder. Nil skips
	// the workspace rung.
	WorkspaceDefault voicescope.WorkspaceDefault
	// Stores yields the per-workspace terms and content memory. Nil
	// translates without terminology and without a block's history.
	Stores *workspaceStores
}

// editorVoiceContext binds the server's own stores — the same instances the
// voice and terminology surfaces use. Everything in it is optional: a missing
// store means a bare translation or an ungoverned check, never an error.
func (s *Server) editorVoiceContext() editorVoiceContext {
	ctx := editorVoiceContext{Voice: s.VoiceStore, Stores: s.wsStores}
	if s.AuthStore != nil {
		ctx.WorkspaceDefault = &mcpWorkspaceDefaultAdapter{auth: s.AuthStore}
	}
	return ctx
}

// editorTranslateConfig builds the AI translate tool config for the
// interactive editor path.
//
// The governing context is assembled by jobs.BuildTranslateConfig, the one
// assembly the worker's jobs also use, so a translation a person starts from
// the editor carries exactly what a queued one does: the voice profile
// resolved through the voicescope ladder, the per-locale term rules from the
// workspace terms, the project's do-not-translate terms, the workspace content
// memory, the surrounding blocks, and the point the item sits at. Every
// binding is best-effort — absence or a resolution failure leaves the field
// unset and the translation runs bare, never fails.
func editorTranslateConfig(
	ctx context.Context,
	cs store.ContentStore,
	voiceCtx editorVoiceContext,
	proj *store.Project,
	projectID, stream, itemName, workspaceID, workspaceSlug string,
	req TranslateRequest,
) tools.AITranslateConfig {
	b := jobs.TranslateBinding{
		Store:            cs,
		Voice:            voiceCtx.Voice,
		WorkspaceDefault: voiceCtx.WorkspaceDefault,
		Project:          proj,
		WorkspaceID:      workspaceID,
		ProjectID:        projectID,
		Stream:           stream,
		ItemName:         itemName,
		TargetLocale:     model.LocaleID(req.TargetLocale),
		BatchSize:        req.BatchSize,
		BatchConcurrency: req.Concurrency,
	}
	b.Terms = editorTerms(ctx, voiceCtx, workspaceSlug)
	b.Memory = editorMemory(ctx, voiceCtx, workspaceSlug)
	return jobs.BuildTranslateConfig(ctx, b)
}

// editorTerms returns the workspace terms an editor translation derives its
// terminology from, or nil (and logs) when the store cannot be opened:
// terminology must never fail an interactive translation.
func editorTerms(ctx context.Context, voiceCtx editorVoiceContext, workspaceSlug string) terms.Terminology {
	if voiceCtx.Stores == nil || workspaceSlug == "" {
		return nil
	}
	tb, err := voiceCtx.Stores.getTerms(workspaceSlug)
	if err != nil {
		slog.WarnContext(ctx, "terms resolution failed; translating without terminology",
			"workspace", workspaceSlug, "error", err)
		return nil
	}
	if tb == nil {
		return nil
	}
	return tb
}

// editorMemory returns the workspace content memory an editor translation
// reads a block's history from, or nil (and logs) when it cannot be opened: the
// content memory answers what a block said before, and a translation that
// cannot ask still translates.
func editorMemory(ctx context.Context, voiceCtx editorVoiceContext, workspaceSlug string) memory.ContentMemory {
	if voiceCtx.Stores == nil || workspaceSlug == "" {
		return nil
	}
	tm, err := voiceCtx.Stores.getMemory(workspaceSlug)
	if err != nil {
		slog.WarnContext(ctx, "content memory unavailable; translating without prior versions",
			"workspace", workspaceSlug, "error", err)
		return nil
	}
	if tm == nil {
		return nil
	}
	return tm
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
// The translate config carries the project's standing context via
// editorTranslateConfig, so an interactive translation gets the guidance an
// async worker job does: the voice profile, the term rules, the protected
// terms, the content memory, the surrounding blocks and the point. The tool is
// handed the item's blocks in document order, which is the neighbourhood the
// context window is measured over.
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
	voiceCtx editorVoiceContext,
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
		editorTranslateConfig(ctx, cs, voiceCtx, proj, projectID, stream, itemName, workspaceID, workspaceSlug, req))

	outParts, err := tool.RunOnParts(ctx, translateTool, parts)
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

	//nolint:contextcheck // the recycle tool threads its operation context through the tool VariantView, not this constructor
	memoryTool := leverage.NewTool(tm, proj.DefaultSourceLanguage, model.LocaleID(targetLocale), 0)

	outParts, err := tool.RunOnParts(ctx, memoryTool, parts)
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

	tb, err := wsStores.getTerms(ws)
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
	outParts, err := tool.RunOnParts(ctx, enforceTool, parts)
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
		result.SourceWords += model.CountWords(src)
		result.SourceChars += len([]rune(src))

		for _, locale := range targetLocales {
			t := sb.Block.TargetText(model.LocaleID(locale))
			if t != "" {
				result.TargetWords[locale] += model.CountWords(t)
				result.TargetChars[locale] += len([]rune(t))
			}
		}
	}

	return result, nil
}

// editorLookupMemoryForBlock looks up content-memory matches for a specific block.
func editorLookupMemoryForBlock(ctx context.Context, cs store.ContentStore, wsStores *workspaceStores, ws, projectID, stream, blockID, targetLocale string) ([]MemoryMatchInfoResponse, error) {
	lookup, err := newMemoryLookup(ctx, cs, wsStores, ws, projectID)
	if err != nil || lookup == nil {
		return nil, err
	}

	sb, err := cs.GetBlock(ctx, projectID, stream, blockID)
	if err != nil {
		return nil, err
	}
	return lookup.matches(ctx, sb.Block, targetLocale)
}

// memoryLookup binds a workspace's content memory to a project's source
// language once, so a run of blocks costs one lookup each rather than a fresh
// project read and store open per block.
type memoryLookup struct {
	tm           memory.Store
	projectID    string
	sourceLocale model.LocaleID
}

// newMemoryLookup resolves the project and its workspace memory. It returns a
// nil lookup (and no error) when the memory holds nothing to match against.
func newMemoryLookup(ctx context.Context, cs store.ContentStore, wsStores *workspaceStores, ws, projectID string) (*memoryLookup, error) {
	proj, err := cs.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	tm, err := wsStores.getMemory(ws)
	if err != nil {
		return nil, fmt.Errorf("init content memory: %w", err)
	}
	count, err := tm.Count(ctx)
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, nil
	}
	return &memoryLookup{tm: tm, projectID: projectID, sourceLocale: proj.DefaultSourceLanguage}, nil
}

// matches returns the block's content-memory matches, best score first.
func (m *memoryLookup) matches(ctx context.Context, b *model.Block, targetLocale string) ([]MemoryMatchInfoResponse, error) {
	opts := memory.DefaultLookupOptions()
	opts.MaxResults = 5
	opts.ProjectID = m.projectID // for scoring boost
	tgtLoc := model.LocaleID(targetLocale)
	matches, err := m.tm.Lookup(ctx, b, m.sourceLocale, tgtLoc, opts)
	if err != nil {
		return nil, err
	}

	result := make([]MemoryMatchInfoResponse, len(matches))
	for i, mt := range matches {
		result[i] = MemoryMatchInfoResponse{
			Source:    mt.Entry.VariantText(m.sourceLocale),
			Target:    mt.Entry.VariantText(tgtLoc),
			Score:     mt.Score,
			MatchType: string(mt.MatchType),
			ProjectID: mt.Entry.ProjectID,
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

	tb, err := wsStores.getTerms(ws)
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
				wordCount += model.CountWords(sb.Block.SourceText())
			}
		}

		out = append(out, ProjectItemResponse{
			ID:           item.ID,
			Name:         item.Name,
			Format:       item.Format,
			SourcePath:   item.Properties[store.ItemPropSourcePath],
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

// projectAggregates are the per-project totals every project response carries,
// plus the per-collection item tally the collection rows are numbered from.
type projectAggregates struct {
	itemCount  int
	blockCount int
	wordCount  int
	// itemCounts maps a collection id to how many of the project's items sit
	// in it; the collection-less bucket is keyed by the empty string.
	itemCounts map[string]int
}

// editorBuildProjectSummary computes a project's aggregates without
// materializing (or shipping) the per-item array: item count, total block
// count, translatable source words, and the per-collection tally. It uses
// GetBlockStats — the same lightweight projection the translation dashboard
// uses — instead of a full GetBlocks per item, so no targets, properties, or
// overlays are deserialized.
func editorBuildProjectSummary(ctx context.Context, cs store.ContentStore, projID, stream string) (projectAggregates, error) {
	items, err := cs.ListItems(ctx, projID, stream)
	if err != nil {
		return projectAggregates{}, fmt.Errorf("list items: %w", err)
	}
	agg := projectAggregates{itemCount: len(items), itemCounts: make(map[string]int, len(items))}
	for _, item := range items {
		agg.itemCounts[item.CollectionID]++
	}

	stats, err := cs.GetBlockStats(ctx, projID, stream)
	if err != nil {
		return projectAggregates{}, fmt.Errorf("get block stats: %w", err)
	}
	for _, bs := range stats {
		agg.blockCount++
		if bs.Translatable {
			agg.wordCount += bs.SourceWords
		}
	}
	return agg, nil
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

	editorAttachProjectShape(ctx, cs, proj.ID, stream, info, itemCounts)
	return info, nil
}

// editorBuildProjectSummaryInfo builds the project's metadata shape: the
// project's own fields, its collections and streams, and the aggregates —
// everything except the per-item array. It costs one item listing plus one
// block-stats projection, where the full builder costs a full block read per
// item, so a project's surfaces that only need its name, locales, streams or
// collections stay flat as the item count grows.
func editorBuildProjectSummaryInfo(ctx context.Context, cs store.ContentStore, proj *store.Project, stream string) (*ProjectInfoResponse, error) {
	info := projectToInfoResponse(proj)

	agg, err := editorBuildProjectSummary(ctx, cs, proj.ID, stream)
	if err != nil {
		return nil, err
	}
	info.ItemCount = agg.itemCount
	info.BlockCount = agg.blockCount
	info.WordCount = agg.wordCount

	editorAttachProjectShape(ctx, cs, proj.ID, stream, info, agg.itemCounts)
	return info, nil
}

// editorAttachProjectShape fills the parts of a project response that describe
// its shape rather than its content: the collections (each carrying the item
// tally the caller counted) and the streams. Shared by both builders so the
// summary and the full detail describe the same project.
func editorAttachProjectShape(ctx context.Context, cs store.ContentStore, projID, stream string, info *ProjectInfoResponse, itemCounts map[string]int) {
	colls, _ := cs.ListCollections(ctx, projID, stream)
	for _, coll := range colls {
		cr := collectionToResponse(coll)
		cr.ItemCount = itemCounts[coll.ID]
		info.Collections = append(info.Collections, cr)
	}

	streams, _ := cs.ListStreams(ctx, projID, false)
	if streams != nil {
		deref := make([]store.Stream, len(streams))
		for i, st := range streams {
			deref[i] = *st
		}
		info.Streams = deref
	}
	info.StreamCount = len(streams)
	info.ActiveStream = stream
}

// storedBlockToInfoResponse converts a StoredBlock to a BlockInfoResponse.
func storedBlockToInfoResponse(sb *venue.StoredBlock, targetLocales []string) BlockInfoResponse {
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
		SourceID:     sb.SourceID,
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
	if !model.RunsHaveInlineCodes(srcRuns) {
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
func storedBlocksToParts(storedBlocks []*venue.StoredBlock) []*model.Part {
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
		stats.WordCount += model.CountWords(block.SourceText())
		if block.TargetText(model.LocaleID(targetLocale)) != "" {
			stats.TranslatedBlocks++
		}
	}
	return stats
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
	collMap := make(map[string]*store.Collection, len(colls)) // id → row
	for _, c := range colls {
		collMap[c.ID] = c
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
		collName := ""
		if c := collMap[item.CollectionID]; c != nil {
			collName = c.Name
		}
		itemStats = append(itemStats, store.ItemTranslationStats{
			ItemName:       item.Name,
			ItemID:         item.ID,
			Format:         item.Format,
			SourcePath:     item.Properties[store.ItemPropSourcePath],
			CollectionID:   item.CollectionID,
			CollectionName: collName,
			BlockCount:     bc,
			WordCount:      wc,
			Locales:        itemLocales,
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
		stat := store.CollectionTranslationStats{
			CollectionID: cid,
			ItemCount:    len(ca.itemSet),
			BlockCount:   ca.blockCount,
			WordCount:    ca.wordCount,
			Locales:      cls,
		}
		// Items belonging to no collection aggregate under the ungrouped
		// bucket: the empty collection id, flagged so a consumer can label and
		// place it deliberately rather than inferring it from an empty string.
		if cid == "" {
			stat.Ungrouped = true
		}
		// Channel and coordinates are projected from the collection row the
		// context push wrote. A bucket whose id names no row (an item pointing
		// at a deleted collection) carries neither.
		if c := collMap[cid]; c != nil {
			stat.CollectionName = c.Name
			stat.Channel = c.ConnectorConfig[coreprofile.PropertyChannel]
			stat.Coordinates = maps.Clone(c.Context)
			stat.PreviewKind = c.PreviewKind
			stat.PreviewURL = c.PreviewURL
		}
		collStats = append(collStats, stat)
	}
	// collAggs is a map, so order it before it reaches the wire: by collection
	// name — falling back to the id for a bucket whose collection row is gone —
	// with the ungrouped bucket last.
	collSortKey := func(c store.CollectionTranslationStats) string {
		if c.CollectionName == "" {
			return strings.ToLower(c.CollectionID)
		}
		return strings.ToLower(c.CollectionName)
	}
	sort.SliceStable(collStats, func(i, j int) bool {
		a, b := collStats[i], collStats[j]
		if a.Ungrouped != b.Ungrouped {
			return b.Ungrouped
		}
		if ka, kb := collSortKey(a), collSortKey(b); ka != kb {
			return ka < kb
		}
		return a.CollectionID < b.CollectionID
	})

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

// dashboardItemWindow is the item-list request a dashboard read carries: which
// collection's items are wanted, then how that list is ordered and sliced. The
// zero value asks for every item in ListItems order — the legacy response.
type dashboardItemWindow struct {
	// collectionID scopes the list to one collection. Empty means every item;
	// the items belonging to no collection are asked for with ungrouped.
	collectionID string
	// ungrouped scopes the list to the items belonging to no collection — the
	// same bucket CollectionStats marks with Ungrouped. A collection id can
	// never select it, because that bucket's id is the empty string.
	ungrouped bool
	limit     int
	offset    int
	sortField string
	dir       string
}

// scoped reports whether the window narrows the item list to one collection.
func (w dashboardItemWindow) scoped() bool { return w.collectionID != "" || w.ungrouped }

// itemInScope reports whether one item belongs to the collection the window
// names.
func (w dashboardItemWindow) itemInScope(it store.ItemTranslationStats) bool {
	if w.ungrouped {
		return it.CollectionID == ""
	}
	return it.CollectionID == w.collectionID
}

// itemDisplayPath is the path an item READS as: the file it was extracted from
// when it is a generated catalog, otherwise the item itself.
//
// Mirrors itemDisplayPath in the UI's collections/itemBase.ts. Both surfaces
// show the same path, so both must trim the same one — a base computed over
// item names prefixes none of the source paths shown beside them, and every row
// would read whole.
func itemDisplayPath(it store.ItemTranslationStats) string {
	if it.SourcePath != "" {
		return it.SourcePath
	}
	return it.ItemName
}

// commonItemBase returns the directory prefix every item's displayed path
// shares, with a trailing slash, or "" when they share none.
//
// Whole segments only: "docs/api" and "docs/apps" share "docs/", not "docs/ap".
// A single item contributes its own directory, which is what makes a one-file
// collection read as the file rather than as the path to it.
func commonItemBase(items []store.ItemTranslationStats) string {
	if len(items) == 0 {
		return ""
	}
	dirOf := func(name string) []string {
		name = strings.TrimPrefix(filepath.ToSlash(name), "/")
		segs := strings.Split(name, "/")
		return segs[:len(segs)-1] // drop the file's own name
	}
	shared := dirOf(itemDisplayPath(items[0]))
	for _, it := range items[1:] {
		if len(shared) == 0 {
			return ""
		}
		segs := dirOf(itemDisplayPath(it))
		if len(segs) < len(shared) {
			shared = shared[:len(segs)]
		}
		for i := range shared {
			if segs[i] != shared[i] {
				shared = shared[:i]
				break
			}
		}
	}
	if len(shared) == 0 {
		return ""
	}
	return strings.Join(shared, "/") + "/"
}

// pageDashboardStats returns a shallow copy of stats with ItemStats filtered to
// the window's collection, sorted, and sliced to the requested page. The input
// (typically the cached full result) is never mutated. An unscoped window with
// no limit and no sort keeps the legacy full, ListItems-ordered response.
//
// ItemTotal follows the filter: a scoped read reports how many items the
// collection holds, so a paged consumer's "N of M" counts the list it is
// actually paging. An unscoped read leaves the project-wide total alone.
func pageDashboardStats(stats *store.TranslationDashboardStats, w dashboardItemWindow) *store.TranslationDashboardStats {
	if w.limit <= 0 && w.sortField == "" && !w.scoped() {
		resp := *stats
		resp.ItemBase = commonItemBase(stats.ItemStats)
		return &resp
	}
	resp := *stats
	items := make([]store.ItemTranslationStats, 0, len(stats.ItemStats))
	for _, it := range stats.ItemStats {
		if w.scoped() && !w.itemInScope(it) {
			continue
		}
		items = append(items, it)
	}
	if w.scoped() {
		resp.ItemTotal = len(items)
	}
	// Over the filtered list, before it is sliced: the base names the scope, so
	// it must not change when the reader asks for the next page of that scope.
	resp.ItemBase = commonItemBase(items)
	limit, offset, sortField, dir := w.limit, w.offset, w.sortField, w.dir
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
