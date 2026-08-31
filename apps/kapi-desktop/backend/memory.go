package backend

import (
	"cmp"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/locale"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/terms"
)

// --- DTOs ---

// ResourceInfo describes a named resource (content memory or terms) in KAPI_HOME.
type ResourceInfo struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Modified string `json:"modified"` // ISO 8601
}

// OriginDTO is the frontend-facing content-memory entry origin (provenance).
type OriginDTO struct {
	Source    string `json:"source"`
	Key       string `json:"key,omitempty"`
	Reference string `json:"reference,omitempty"`
	AddedAt   string `json:"added_at"`
	AddedBy   string `json:"added_by,omitempty"`
	SessionID string `json:"session_id,omitempty"`
}

// VariantDTO is a single language variant of a multilingual content-memory entry. Inline
// markup travels as an RFC 0001 Run sequence; Text is the flattened plain form.
type VariantDTO struct {
	Locale string      `json:"locale"`
	Text   string      `json:"text"`
	Runs   []model.Run `json:"runs"`
}

// EntityValueDTO is the frontend-facing value+position of an entity mapping
// for a single locale.
type EntityValueDTO struct {
	Text  string `json:"text"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

// EntityMappingDTO is a multilingual entity mapping.
type EntityMappingDTO struct {
	PlaceholderID string                    `json:"placeholder_id"`
	Type          string                    `json:"type"`
	Values        map[string]EntityValueDTO `json:"values"`
	ConceptID     string                    `json:"concept_id,omitempty"` // optional terms cross-reference
}

// MemoryEntryDTO is the frontend-facing multilingual content-memory entry.
type MemoryEntryDTO struct {
	ID          string                `json:"id"`
	ProjectID   string                `json:"project_id"`
	Variants    map[string]VariantDTO `json:"variants"`
	HintSrcLang string                `json:"hint_src_lang"`
	Entities    []EntityMappingDTO    `json:"entities,omitempty"`
	Properties  map[string]string     `json:"properties,omitempty"`
	Note        string                `json:"note,omitempty"`
	Origins     []OriginDTO           `json:"origins,omitempty"`
	// Unit is the block this answer was approved for, and Point the coordinate
	// it was approved at. An entry the whole project agrees on carries a unit
	// and no point: a point is recorded when one source has been answered
	// differently at two of them, which is the disagreement the field exists to
	// keep straight.
	Unit      string    `json:"unit,omitempty"`
	Point     *PointDTO `json:"point,omitempty"`
	CreatedAt string    `json:"created_at"`
	UpdatedAt string    `json:"updated_at"`
}

// PointDTO is a context point as its rungs, coarsest first.
//
// The stored form joins them with a unit separator and is opaque by design —
// the corpus holds no recipe, so it cannot say how a point should read. Split
// here rather than passed through, because a raw point rendered into a UI is a
// string with control characters in it.
type PointDTO struct {
	Profile    string `json:"profile,omitempty"`
	Channel    string `json:"channel,omitempty"`
	Collection string `json:"collection,omitempty"`
}

func pointToDTO(point string) *PointDTO {
	if point == "" {
		return nil
	}
	return &PointDTO{
		Profile:    memory.PointRung(point, 0),
		Channel:    memory.PointRung(point, 1),
		Collection: memory.PointRung(point, 2),
	}
}

// MemorySearchResult is the paginated result from SearchMemoryEntries.
type MemorySearchResult struct {
	Entries    []MemoryEntryDTO `json:"entries"`
	TotalCount int              `json:"total_count"`
}

// MemoryStats is the stats response for an open content memory.
type MemoryStats struct {
	Count int    `json:"count"`
	Path  string `json:"path"`
	Size  int64  `json:"size"`
}

// MemoryMatchDTO is a single match from entity-aware content-memory lookup.
type MemoryMatchDTO struct {
	Entry             MemoryEntryDTO        `json:"entry"`
	Score             float64               `json:"score"`
	MatchType         string                `json:"match_type"`
	EntityAdaptations []EntityAdaptationDTO `json:"entity_adaptations,omitempty"`
}

// EntityAdaptationDTO describes how to substitute an entity value.
type EntityAdaptationDTO struct {
	PlaceholderID string `json:"placeholder_id"`
	Type          string `json:"type"`
	StoredValue   string `json:"stored_value"`
	CurrentValue  string `json:"current_value"`
}

// LookupMemoryRequest is the request for entity-aware content-memory lookup.
type LookupMemoryRequest struct {
	Text         string                `json:"text"`
	Entities     []EntityAnnotationDTO `json:"entities"`
	SourceLocale string                `json:"source_locale"`
	TargetLocale string                `json:"target_locale"`
	MinScore     float64               `json:"min_score"`
	MaxResults   int                   `json:"max_results"`
}

// EntityAnnotationDTO is a single entity annotation from the frontend.
type EntityAnnotationDTO struct {
	Text  string `json:"text"`
	Type  string `json:"type"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

// AddMemoryEntryRequest is the request to add a new multilingual content-memory entry.
// Callers populate Variants with one VariantInput per locale; the server uses
// each variant's Run sequence, falling back to plain Text when Runs is empty.
type AddMemoryEntryRequest struct {
	Variants    map[string]VariantInputDTO `json:"variants"`
	HintSrcLang string                     `json:"hint_src_lang"`
	ProjectID   string                     `json:"project_id"`
	Note        string                     `json:"note,omitempty"`
	Origins     []OriginDTO                `json:"origins,omitempty"`
}

// VariantInputDTO is how the frontend submits a single variant on add/update.
// Runs carries the inline content; Text is a plain-text fallback used when
// Runs is empty.
type VariantInputDTO struct {
	Text string      `json:"text"`
	Runs []model.Run `json:"runs,omitempty"`
}

// UpdateMemoryEntryRequest is the request to update a content-memory entry. The Variants map
// replaces the stored variants wholesale.
type UpdateMemoryEntryRequest struct {
	EntryID     string                     `json:"entry_id"`
	Variants    map[string]VariantInputDTO `json:"variants"`
	HintSrcLang string                     `json:"hint_src_lang"`
	ProjectID   string                     `json:"project_id"`
	Note        string                     `json:"note,omitempty"`
	Origins     []OriginDTO                `json:"origins,omitempty"`
}

// AnnotateEntitiesRequest is the request to batch-annotate entities on content-memory entries.
type AnnotateEntitiesRequest struct {
	EntryIDs    []string               `json:"entry_ids"`
	Patterns    []EntityPatternRequest `json:"patterns"`
	TermsHandle string                 `json:"termbase_handle,omitempty"` // optional: cross-ref entities against this terms
}

// EntityPatternRequest defines a text→entity mapping for batch annotation.
type EntityPatternRequest struct {
	Text          string `json:"text"`
	EntityType    string `json:"entity_type"`
	CaseSensitive bool   `json:"case_sensitive"`
}

// AnnotateResult reports the outcome of a batch entity annotation.
type AnnotateResult struct {
	EntriesUpdated int `json:"entries_updated"`
	EntitiesAdded  int `json:"entities_added"`
}

// MemoryFacets is the frontend-facing facet data for the sidebar.
type MemoryFacets struct {
	Locales        []LocaleFacetDTO        `json:"locales"`
	Projects       []ProjectFacetDTO       `json:"projects"`
	EntityTypes    []EntityTypeFacetDTO    `json:"entity_types"`
	ImportSessions []ImportSessionFacetDTO `json:"import_sessions"`
	HasCodes       int                     `json:"has_codes"`
	NoCodes        int                     `json:"no_codes"`
}

// LocaleFacetDTO is a single-locale entry count.
type LocaleFacetDTO struct {
	Locale string `json:"locale"`
	Count  int    `json:"count"`
}

// ProjectFacetDTO is a project ID with its entry count.
type ProjectFacetDTO struct {
	ProjectID string `json:"project_id"`
	Count     int    `json:"count"`
}

// EntityTypeFacetDTO is an entity type with its entry count.
type EntityTypeFacetDTO struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

// ImportSessionFacetDTO is an import session as a facet option.
type ImportSessionFacetDTO struct {
	SessionID  string `json:"session_id"`
	FileKey    string `json:"file_key"`
	ToolName   string `json:"tool_name,omitempty"`
	ImportedAt string `json:"imported_at"`
	Count      int    `json:"count"`
}

// ImportSessionDTO is the full import-session record for the sessions panel.
type ImportSessionDTO struct {
	ID               string            `json:"id"`
	FileKey          string            `json:"file_key"`
	FileHash         string            `json:"file_hash"`
	FileSizeBytes    int64             `json:"file_size_bytes"`
	ImportedAt       string            `json:"imported_at"`
	ImportedBy       string            `json:"imported_by"`
	ToolName         string            `json:"tool_name"`
	ToolVersion      string            `json:"tool_version"`
	SegType          string            `json:"seg_type"`
	AdminLang        string            `json:"admin_lang"`
	SrcLang          string            `json:"src_lang"`
	DataType         string            `json:"data_type"`
	OriginalFormat   string            `json:"original_format"`
	OriginalEncoding string            `json:"original_encoding"`
	EntryCount       int               `json:"entry_count"`
	Properties       map[string]string `json:"properties,omitempty"`
}

// MemorySearchFilter is the frontend-facing search filter.
type MemorySearchFilter struct {
	ProjectID    string              `json:"project_id,omitempty"`
	Locale       string              `json:"locale,omitempty"` // require this locale variant
	SessionIDs   []string            `json:"session_ids,omitempty"`
	EntityTypes  []string            `json:"entity_types,omitempty"`
	EntityValues []EntityValueFilter `json:"entity_values,omitempty"`
	HasCodes     *bool               `json:"has_codes,omitempty"`
}

// EntityValueFilter is a single entity value+type pair for search filtering.
type EntityValueFilter struct {
	Value string `json:"value"`
	Type  string `json:"type"`
}

// --- Conversion helpers ---

// runsFromVariantInput builds a Run sequence from the frontend variant input.
// It uses the submitted Run sequence directly; when Runs is empty it falls back
// to a single TextRun built from Text (or nil for empty input).
func runsFromVariantInput(in VariantInputDTO) []model.Run {
	if len(in.Runs) > 0 {
		return in.Runs
	}
	if in.Text == "" {
		return nil
	}
	return []model.Run{{Text: &model.TextRun{Text: in.Text}}}
}

// runsToVariantDTO converts a Run sequence into the frontend shape, carrying
// the runs verbatim plus the flattened plain text.
func runsToVariantDTO(locale model.LocaleID, runs []model.Run) VariantDTO {
	if len(runs) == 0 {
		return VariantDTO{Locale: string(locale)}
	}
	return VariantDTO{
		Locale: string(locale),
		Text:   model.FlattenRuns(runs),
		Runs:   runs,
	}
}

// originsToDTO converts memory.Origin values to OriginDTO for the frontend.
func originsToDTO(in []memory.Origin) []OriginDTO {
	if len(in) == 0 {
		return nil
	}
	out := make([]OriginDTO, 0, len(in))
	for _, o := range in {
		out = append(out, OriginDTO{
			Source:    o.Source,
			Key:       o.Key,
			Reference: o.Reference,
			AddedAt:   o.AddedAt.Format(time.RFC3339),
			AddedBy:   o.AddedBy,
			SessionID: o.SessionID,
		})
	}
	return out
}

// originsFromDTO converts request OriginDTOs to memory.Origin values,
// defaulting AddedAt to time.Now() when not supplied.
func originsFromDTO(in []OriginDTO) []memory.Origin {
	if len(in) == 0 {
		return nil
	}
	out := make([]memory.Origin, 0, len(in))
	now := time.Now()
	for _, o := range in {
		addedAt, _ := time.Parse(time.RFC3339, o.AddedAt)
		if addedAt.IsZero() {
			addedAt = now
		}
		out = append(out, memory.Origin{
			Source:    o.Source,
			Key:       o.Key,
			Reference: o.Reference,
			AddedAt:   addedAt,
			AddedBy:   o.AddedBy,
			SessionID: o.SessionID,
		})
	}
	return out
}

// entitiesToDTO converts stored entities to the frontend shape.
func entitiesToDTO(in []memory.EntityMapping) []EntityMappingDTO {
	if len(in) == 0 {
		return nil
	}
	out := make([]EntityMappingDTO, 0, len(in))
	for _, em := range in {
		values := make(map[string]EntityValueDTO, len(em.Values))
		for loc, v := range em.Values {
			values[string(loc)] = EntityValueDTO{Text: v.Text, Start: v.Start, End: v.End}
		}
		out = append(out, EntityMappingDTO{
			PlaceholderID: em.PlaceholderID,
			Type:          string(em.Type),
			Values:        values,
			ConceptID:     em.ConceptID,
		})
	}
	return out
}

func memoryEntryToDTO(entry memory.Entry) MemoryEntryDTO {
	variants := make(map[string]VariantDTO, len(entry.Variants))
	for loc, runs := range entry.Variants {
		variants[string(loc)] = runsToVariantDTO(loc, runs)
	}
	return MemoryEntryDTO{
		ID:          entry.ID,
		ProjectID:   entry.ProjectID,
		Variants:    variants,
		HintSrcLang: string(entry.HintSrcLang),
		Entities:    entitiesToDTO(entry.Entities),
		Properties:  entry.Properties,
		Note:        entry.Note,
		Origins:     originsToDTO(entry.Origins),
		Unit:        entry.Unit,
		Point:       pointToDTO(entry.Point),
		CreatedAt:   entry.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   entry.UpdatedAt.Format(time.RFC3339),
	}
}

// canonicalLocale turns a locale as the frontend spells it into the form the
// stores key on.
//
// A locale arrives from a text field, a URL, a POSIX environment or another
// tool, so "nb_NO", "NB-no" and "nb-NO" all name one language and only the last
// one matches a stored variant. Canonicalizing at the method boundary is what
// keeps a search for "nb_NO" from quietly finding nothing.
//
// This is an ingress boundary, so it goes through locale.Canonical rather than
// locale.Parse: the app renders a pseudo-locale ("qps-Ploc") whose subtags CLDR
// does not know, and the stricter function rejects it.
//
// An empty locale is a real answer — unscoped — rather than a bad one.
func canonicalLocale(s string) (model.LocaleID, error) {
	if s == "" {
		return "", nil
	}
	return locale.Canonical(s)
}

// canonicalSearchLocales canonicalizes the pair of locales a search or lookup
// is scoped by, reporting false when either is not a locale.
//
// These methods answer with a result rather than an error, so a locale nothing
// could name is answered the way an unknown handle is: with nothing found. The
// alternative is searching for the literal string the caller typed, which finds
// nothing anyway and says so less clearly.
func canonicalSearchLocales(a, b string) (string, string, bool) {
	first, err := canonicalLocale(a)
	if err != nil {
		return "", "", false
	}
	second, err := canonicalLocale(b)
	if err != nil {
		return "", "", false
	}
	return string(first), string(second), true
}

func variantsFromInput(in map[string]VariantInputDTO) (map[model.LocaleID][]model.Run, error) {
	out := make(map[model.LocaleID][]model.Run, len(in))
	for loc, v := range in {
		if loc == "" {
			continue
		}
		canonical, err := canonicalLocale(loc)
		if err != nil {
			return nil, err
		}
		out[canonical] = runsFromVariantInput(v)
	}
	return out, nil
}

// --- Resource discovery ---

func (a *App) ListNamedMemories() []ResourceInfo {
	return listNamedResources("memory")
}

func listNamedResources(kind string) []ResourceInfo {
	dir := namedResourceDir(kind)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var result []ResourceInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".db") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".db")
		result = append(result, ResourceInfo{
			Name:     name,
			Path:     filepath.Join(dir, e.Name()),
			Size:     info.Size(),
			Modified: info.ModTime().Format(time.RFC3339),
		})
	}
	slices.SortFunc(result, func(a, b ResourceInfo) int {
		return cmp.Compare(b.Modified, a.Modified)
	})
	return result
}

func namedResourceDir(kind string) string {
	return filepath.Join(kapiConfigDir(), kind)
}

// --- Recovery ---

// RecoverResource moves an unopenable store aside so a fresh one can take its
// place, and returns where the old one went. The caller creates the replacement.
//
// Two kinds of file arrive here and they part company in what "aside" costs. A
// STANDALONE store — one the user opened by path, or a named store under
// `~/.config/kapi` — is one content memory or one terms store, and moving it
// aside loses exactly that. A PROJECT store is `.kapi/work/store.db`, where the
// content memory, terms, block cache and unit working set share one file: moving
// it aside takes all four. That is the documented trade of merging them, and it
// is affordable because every one of those is a projection rebuilt from
// committed sources — the exception being decisions staged and not yet
// committed, which a store this process cannot open was not going to give back
// either.
//
// A project store also needs the handle released first: renaming a file under an
// open pool leaves the pool on the moved inode, and the replacement would be
// written by a second one.
func (a *App) RecoverResource(path string) (string, error) {
	if root, ok := projectStoreRoot(path); ok {
		a.releaseTabsFor(root)
		if err := a.hostEngine().CloseProjectDB(root); err != nil {
			return "", fmt.Errorf("release project store %q: %w", path, err)
		}
	}
	bakPath := path + ".bak"
	_ = os.Remove(bakPath)
	if err := os.Rename(path, bakPath); err != nil {
		return "", fmt.Errorf("backup %q: %w", path, err)
	}
	return bakPath, nil
}

// projectStoreRoot reports the project root when path names a project's own
// store — `<root>/.kapi/work/store.db` — rather than a standalone one.
func projectStoreRoot(path string) (string, bool) {
	if filepath.Base(path) != project.StoreFileName {
		return "", false
	}
	workDir := filepath.Dir(path)
	if filepath.Base(workDir) != project.WorkDirName {
		return "", false
	}
	stateDir := filepath.Dir(workDir)
	if filepath.Base(stateDir) != project.StateDirName {
		return "", false
	}
	return filepath.Dir(stateDir), true
}

// releaseTabsFor drops the borrowed content-memory and terms handles of every
// tab open on a project, so nothing keeps addressing a store that is about to be
// moved aside.
func (a *App) releaseTabsFor(root string) {
	a.mu.RLock()
	var affected []*openProject
	for _, op := range a.projects {
		if opRoot, ok := projectRoot(op); ok && opRoot == root {
			affected = append(affected, op)
		}
	}
	a.mu.RUnlock()
	for _, op := range affected {
		if op.memoryHandle != "" {
			a.memoryHandles.Close(op.memoryHandle)
			op.memoryHandle = ""
		}
		if op.tbHandle != "" {
			a.tbHandles.Close(op.tbHandle)
			op.tbHandle = ""
		}
	}
}

// --- Lifecycle ---

func (a *App) OpenMemory(path string) (string, error) {
	tm, err := memory.NewSQLiteStore(path)
	if err != nil {
		return "", fmt.Errorf("open Memory %q: %w", path, err)
	}
	return a.memoryHandles.Open(tm), nil
}

func (a *App) OpenMemoryDialog() (string, error) {
	if a.app == nil {
		return "", nil
	}
	path, err := a.app.Dialog.OpenFile().
		AddFilter("Content Memory", "*.db").
		AddFilter("All Files", "*").
		PromptForSingleSelection()
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", nil
	}
	return a.OpenMemory(path)
}

func (a *App) CreateMemory(path string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create directory: %w", err)
	}
	return a.OpenMemory(path)
}

func (a *App) CreateNamedMemory(name string) (string, error) {
	dir := namedResourceDir("memory")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create Memory directory: %w", err)
	}
	path := filepath.Join(dir, name+".db")
	return a.OpenMemory(path)
}

func (a *App) CloseMemory(handle string) {
	_ = a.memoryHandles.Close(handle)
}

func (a *App) GetMemoryStats(handle string) *MemoryStats {
	tm, ok := a.memoryHandles.Get(handle)
	if !ok {
		return nil
	}
	count, err := tm.Count(context.Background())
	if err != nil {
		return nil
	}
	return &MemoryStats{Count: count}
}

// GetMemoryActivityStats returns daily entry counts over time.
func (a *App) GetMemoryActivityStats(handle string) []memory.ActivityStat {
	tm, ok := a.memoryHandles.Get(handle)
	if !ok {
		return nil
	}
	stats, err := tm.ActivityStats(context.Background())
	if err != nil {
		return nil
	}
	return stats
}

// GetMemoryLocaleStats returns per-locale entry counts. The legacy API name is
// preserved for frontend compatibility; the response is now a flat list of
// single-locale counts, not locale pairs.
func (a *App) GetMemoryLocaleStats(handle string) []memory.LocaleFacet {
	tm, ok := a.memoryHandles.Get(handle)
	if !ok {
		return nil
	}
	stats, err := tm.LocaleStats(context.Background())
	if err != nil {
		return nil
	}
	return stats
}

// --- CRUD ---

// SearchMemoryEntries searches content-memory entries by query with pagination.
// anyLocale restricts the text search to entries with a variant in that
// locale; requireLocale additionally requires that variant to exist.
func (a *App) SearchMemoryEntries(handle, query, anyLocale, requireLocale string, offset, limit int) *MemorySearchResult {
	tm, ok := a.memoryHandles.Get(handle)
	if !ok {
		return &MemorySearchResult{}
	}
	anyLocale, requireLocale, named := canonicalSearchLocales(anyLocale, requireLocale)
	if !named {
		return &MemorySearchResult{}
	}
	entries, total, err := tm.SearchEntries(context.Background(), memory.SearchParams{
		Query:         query,
		AnyLocale:     anyLocale,
		RequireLocale: requireLocale,
		Offset:        offset,
		Limit:         limit,
	})
	if err != nil {
		return &MemorySearchResult{}
	}
	dtos := make([]MemoryEntryDTO, 0, len(entries))
	for _, e := range entries {
		dtos = append(dtos, memoryEntryToDTO(e))
	}
	return &MemorySearchResult{Entries: dtos, TotalCount: total}
}

// SearchMemoryEntriesFiltered searches content-memory entries with facet filters.
func (a *App) SearchMemoryEntriesFiltered(handle, query, anyLocale, requireLocale string, filter MemorySearchFilter, offset, limit int) *MemorySearchResult {
	tm, ok := a.memoryHandles.Get(handle)
	if !ok {
		return &MemorySearchResult{}
	}
	anyLocale, requireLocale, named := canonicalSearchLocales(anyLocale, requireLocale)
	if !named {
		return &MemorySearchResult{}
	}
	entries, total, err := tm.SearchEntriesFiltered(context.Background(), memory.SearchParams{
		Query:         query,
		AnyLocale:     anyLocale,
		RequireLocale: requireLocale,
		Filter:        toSearchFilter(filter),
		Offset:        offset,
		Limit:         limit,
	})
	if err != nil {
		return &MemorySearchResult{}
	}
	dtos := make([]MemoryEntryDTO, 0, len(entries))
	for _, e := range entries {
		dtos = append(dtos, memoryEntryToDTO(e))
	}
	return &MemorySearchResult{Entries: dtos, TotalCount: total}
}

// GetMemoryEntry returns a single content-memory entry by ID.
func (a *App) GetMemoryEntry(handle, entryID string) *MemoryEntryDTO {
	tm, ok := a.memoryHandles.Get(handle)
	if !ok {
		return nil
	}
	entry, found, err := tm.GetEntry(context.Background(), entryID)
	if err != nil || !found {
		return nil
	}
	dto := memoryEntryToDTO(entry)
	return &dto
}

// AddMemoryEntry adds a new multilingual content-memory entry.
func (a *App) AddMemoryEntry(handle string, req AddMemoryEntryRequest) error {
	tm, ok := a.memoryHandles.Get(handle)
	if !ok {
		return fmt.Errorf("content-memory handle %q not found", handle)
	}
	variants, err := variantsFromInput(req.Variants)
	if err != nil {
		return err
	}
	hint, err := canonicalLocale(req.HintSrcLang)
	if err != nil {
		return err
	}
	now := time.Now()
	entry := memory.Entry{
		ID:          id.New(),
		ProjectID:   req.ProjectID,
		Variants:    variants,
		HintSrcLang: hint,
		Note:        req.Note,
		Origins:     originsFromDTO(req.Origins),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	return tm.Add(context.Background(), entry)
}

// UpdateMemoryEntry updates an existing multilingual content-memory entry.
func (a *App) UpdateMemoryEntry(handle string, req UpdateMemoryEntryRequest) error {
	tm, ok := a.memoryHandles.Get(handle)
	if !ok {
		return fmt.Errorf("content-memory handle %q not found", handle)
	}
	existing, found, err := tm.GetEntry(context.Background(), req.EntryID)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("entry %q not found", req.EntryID)
	}
	// A write replaces the locales it names and leaves the rest standing, so an
	// edit that removes a language has to say so: the removed locale is named
	// with no runs, which retires it. Handing the store only the surviving
	// locales would leave the dropped one in place and the editor showing a
	// language the user just deleted.
	next, err := variantsFromInput(req.Variants)
	if err != nil {
		return err
	}
	for locale := range existing.Variants {
		if _, kept := next[locale]; !kept {
			next[locale] = nil
		}
	}
	existing.Variants = next
	if req.HintSrcLang != "" {
		existing.HintSrcLang = model.LocaleID(req.HintSrcLang)
	}
	if req.ProjectID != "" {
		existing.ProjectID = req.ProjectID
	}
	existing.Note = req.Note
	if req.Origins != nil {
		existing.Origins = originsFromDTO(req.Origins)
	}
	existing.UpdatedAt = time.Now()
	return tm.Add(context.Background(), existing)
}

// DeleteMemoryEntry deletes a single content-memory entry.
func (a *App) DeleteMemoryEntry(handle, entryID string) error {
	tm, ok := a.memoryHandles.Get(handle)
	if !ok {
		return fmt.Errorf("content-memory handle %q not found", handle)
	}
	return tm.Delete(context.Background(), entryID)
}

// DeleteMemoryEntries deletes multiple content-memory entries.
func (a *App) DeleteMemoryEntries(handle string, entryIDs []string) error {
	tm, ok := a.memoryHandles.Get(handle)
	if !ok {
		return fmt.Errorf("content-memory handle %q not found", handle)
	}
	for _, eid := range entryIDs {
		if err := tm.Delete(context.Background(), eid); err != nil {
			return err
		}
	}
	return nil
}

// --- Entity-aware lookup ---

// LookupMemory performs entity-aware content-memory lookup using the full tiered matching pipeline.
func (a *App) LookupMemory(handle string, req LookupMemoryRequest) []MemoryMatchDTO {
	tm, ok := a.memoryHandles.Get(handle)
	if !ok {
		return nil
	}

	runs := buildRunsWithEntities(req.Text, req.Entities)
	block := &model.Block{
		ID:           "lookup",
		Translatable: true,
		Source:       runs,
	}
	for i, ea := range req.Entities {
		block.AddOverlaySpan(model.OverlayEntity, model.Span{
			ID:    fmt.Sprintf("entity:%d", i),
			Range: model.RangeAnchorForBytes(block.Source, ea.Start, ea.End),
			Value: &model.EntityAnnotation{
				Text:   ea.Text,
				Type:   model.EntityType(ea.Type),
				Source: model.ExtractionSourceManual,
			},
		})
	}

	opts := memory.LookupOptions{
		MinScore:   req.MinScore,
		MaxResults: req.MaxResults,
	}
	if opts.MinScore == 0 {
		opts.MinScore = 0.7
	}
	if opts.MaxResults == 0 {
		opts.MaxResults = 10
	}

	matches, err := tm.Lookup(context.Background(), block, model.LocaleID(req.SourceLocale), model.LocaleID(req.TargetLocale), opts)
	if err != nil {
		a.logger.Printf("content-memory lookup error: %v", err)
		return nil
	}

	result := make([]MemoryMatchDTO, 0, len(matches))
	for _, m := range matches {
		dto := MemoryMatchDTO{
			Entry:     memoryEntryToDTO(m.Entry),
			Score:     m.Score,
			MatchType: string(m.MatchType),
		}
		for _, ea := range m.EntityAdaptations {
			dto.EntityAdaptations = append(dto.EntityAdaptations, EntityAdaptationDTO{
				PlaceholderID: ea.PlaceholderID,
				Type:          string(ea.Type),
				StoredValue:   ea.StoredValue,
				CurrentValue:  ea.CurrentValue,
			})
		}
		result = append(result, dto)
	}
	return result
}

// buildRunsWithEntities builds a Run sequence from plain text + entity
// annotations. Entity ranges become PlaceholderRuns; the surrounding
// text is split into TextRuns.
func buildRunsWithEntities(text string, entities []EntityAnnotationDTO) []model.Run {
	if len(entities) == 0 {
		if text == "" {
			return nil
		}
		return []model.Run{{Text: &model.TextRun{Text: text}}}
	}

	sorted := make([]EntityAnnotationDTO, len(entities))
	copy(sorted, entities)
	slices.SortFunc(sorted, func(a, b EntityAnnotationDTO) int {
		return cmp.Compare(a.Start, b.Start)
	})

	runes := []rune(text)
	var runs []model.Run
	pos := 0
	appendText := func(s string) {
		if s == "" {
			return
		}
		runs = append(runs, model.Run{Text: &model.TextRun{Text: s}})
	}

	for i, ea := range sorted {
		if ea.Start < pos || ea.Start >= len(runes) || ea.End > len(runes) {
			continue
		}
		if ea.Start > pos {
			appendText(string(runes[pos:ea.Start]))
		}
		runs = append(runs, model.Run{Ph: &model.PlaceholderRun{
			ID:   fmt.Sprintf("e%d", i+1),
			Type: ea.Type,
			Data: ea.Text,
		}})
		pos = ea.End
	}
	if pos < len(runes) {
		appendText(string(runes[pos:]))
	}
	return runs
}

// --- Facets ---

func (a *App) GetMemoryFacets(handle string) *MemoryFacets {
	return a.GetMemoryFacetsFiltered(handle, "", "", "", MemorySearchFilter{})
}

func (a *App) GetMemoryFacetsFiltered(handle, query, anyLocale, requireLocale string, filter MemorySearchFilter) *MemoryFacets {
	tm, ok := a.memoryHandles.Get(handle)
	if !ok {
		return nil
	}
	data, err := tm.FacetStatsFiltered(context.Background(), memory.SearchParams{
		Query:         query,
		AnyLocale:     anyLocale,
		RequireLocale: requireLocale,
		Filter:        toSearchFilter(filter),
	})
	if err != nil {
		return nil
	}
	return buildMemoryFacetsDTO(data)
}

func buildMemoryFacetsDTO(data memory.FacetData) *MemoryFacets {
	result := &MemoryFacets{HasCodes: data.HasCodes, NoCodes: data.NoCodes}
	for _, lf := range data.Locales {
		result.Locales = append(result.Locales, LocaleFacetDTO{Locale: lf.Locale, Count: lf.Count})
	}
	for _, p := range data.Projects {
		result.Projects = append(result.Projects, ProjectFacetDTO{ProjectID: p.ProjectID, Count: p.Count})
	}
	for _, et := range data.EntityTypes {
		result.EntityTypes = append(result.EntityTypes, EntityTypeFacetDTO{Type: et.Type, Count: et.Count})
	}
	for _, sf := range data.ImportSessions {
		result.ImportSessions = append(result.ImportSessions, ImportSessionFacetDTO{
			SessionID:  sf.SessionID,
			FileKey:    sf.FileKey,
			ToolName:   sf.ToolName,
			ImportedAt: sf.ImportedAt.Format(time.RFC3339),
			Count:      sf.Count,
		})
	}
	return result
}

// toSearchFilter converts the frontend DTO to the memory filter type.
func toSearchFilter(f MemorySearchFilter) memory.SearchFilter {
	sf := memory.SearchFilter{
		ProjectID:   f.ProjectID,
		SessionIDs:  f.SessionIDs,
		EntityTypes: f.EntityTypes,
		HasCodes:    f.HasCodes,
	}
	if len(f.EntityValues) > 0 {
		sf.EntityValues = make([]memory.EntityValueFilter, len(f.EntityValues))
		for i, ev := range f.EntityValues {
			sf.EntityValues[i] = memory.EntityValueFilter{Value: ev.Value, Type: ev.Type}
		}
	}
	return sf
}

// --- Import session CRUD ---

// ListMemoryImportSessions returns every session row in imported_at DESC order.
func (a *App) ListMemoryImportSessions(handle string) []ImportSessionDTO {
	tm, ok := a.memoryHandles.Get(handle)
	if !ok {
		return nil
	}
	sessions, err := tm.ListImportSessions(context.Background())
	if err != nil {
		return nil
	}
	out := make([]ImportSessionDTO, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, importSessionToDTO(s))
	}
	return out
}

// GetMemoryImportSession fetches a single session by ID.
func (a *App) GetMemoryImportSession(handle, sessionID string) *ImportSessionDTO {
	tm, ok := a.memoryHandles.Get(handle)
	if !ok {
		return nil
	}
	s, found, err := tm.GetImportSession(context.Background(), sessionID)
	if err != nil || !found {
		return nil
	}
	dto := importSessionToDTO(s)
	return &dto
}

// DeleteMemoryImportSession removes a session; its origins keep pointing at
// empty session_id (see memory.DeleteImportSession).
func (a *App) DeleteMemoryImportSession(handle, sessionID string) error {
	tm, ok := a.memoryHandles.Get(handle)
	if !ok {
		return fmt.Errorf("content-memory handle %q not found", handle)
	}
	return tm.DeleteImportSession(context.Background(), sessionID)
}

func importSessionToDTO(s memory.ImportSession) ImportSessionDTO {
	return ImportSessionDTO{
		ID:               s.ID,
		FileKey:          s.FileKey,
		FileHash:         s.FileHash,
		FileSizeBytes:    s.FileSizeBytes,
		ImportedAt:       s.ImportedAt.Format(time.RFC3339),
		ImportedBy:       s.ImportedBy,
		ToolName:         s.ToolName,
		ToolVersion:      s.ToolVersion,
		SegType:          s.SegType,
		AdminLang:        s.AdminLang,
		SrcLang:          s.SrcLang,
		DataType:         s.DataType,
		OriginalFormat:   s.OriginalFormat,
		OriginalEncoding: s.OriginalEncoding,
		EntryCount:       s.EntryCount,
		Properties:       s.Properties,
	}
}

// --- Batch entity annotation ---

// AnnotateEntities applies entity annotations to selected content-memory entries. The
// patterns are searched across every variant's plain text and entity spans
// are inserted where matches are found. Entity values are populated per
// locale from the matching variant.
//
// When TermsHandle is set, each new entity's text is looked up in the
// terms; if a concept matches, its ID is stored on the EntityMapping
// so the content-memory entry cross-references the terms store.
func (a *App) AnnotateEntities(handle string, req AnnotateEntitiesRequest) (*AnnotateResult, error) {
	tm, ok := a.memoryHandles.Get(handle)
	if !ok {
		return nil, fmt.Errorf("content-memory handle %q not found", handle)
	}

	// Optionally resolve concept IDs from the terms store.
	var tb *terms.SQLiteStore
	if req.TermsHandle != "" {
		tb, _ = a.tbHandles.Get(req.TermsHandle)
	}

	var entriesUpdated, entitiesAdded int

	for _, eid := range req.EntryIDs {
		entry, found, err := tm.GetEntry(context.Background(), eid)
		if err != nil || !found {
			continue
		}

		anyHit := false
		newVariants := make(map[model.LocaleID][]model.Run, len(entry.Variants))
		perLocaleCounts := make(map[model.LocaleID]int)
		for loc, runs := range entry.Variants {
			if len(runs) == 0 {
				continue
			}
			newRuns, n := rebuildRunsWithEntities(runs, req.Patterns)
			newVariants[loc] = newRuns
			if n > 0 {
				anyHit = true
				perLocaleCounts[loc] = n
			}
		}
		if !anyHit {
			continue
		}
		entry.Variants = newVariants
		entry.Entities = buildEntityMappingsFromVariantRuns(entry.Variants)
		if tb != nil {
			resolveConceptIDs(entry.Entities, tb)
		}
		entry.UpdatedAt = time.Now()
		if err := tm.Add(context.Background(), entry); err != nil {
			return nil, fmt.Errorf("update entry %q: %w", eid, err)
		}
		entriesUpdated++
		for _, n := range perLocaleCounts {
			entitiesAdded += n
		}
	}

	return &AnnotateResult{EntriesUpdated: entriesUpdated, EntitiesAdded: entitiesAdded}, nil
}

// ResolveEntityConcepts re-links entities on content-memory entries to terms concepts.
// Useful after a terms store import or when entities were created without a
// terms available. Entries whose entities already have a ConceptID are
// skipped unless force is true.
func (a *App) ResolveEntityConcepts(memoryHandle, tbHandle string, entryIDs []string, force bool) (int, error) {
	tm, ok := a.memoryHandles.Get(memoryHandle)
	if !ok {
		return 0, fmt.Errorf("content-memory handle %q not found", memoryHandle)
	}
	tb, ok := a.tbHandles.Get(tbHandle)
	if !ok {
		return 0, fmt.Errorf("terms handle %q not found", tbHandle)
	}

	updated := 0
	for _, eid := range entryIDs {
		entry, found, err := tm.GetEntry(context.Background(), eid)
		if err != nil || !found || len(entry.Entities) == 0 {
			continue
		}
		changed := false
		for i := range entry.Entities {
			if entry.Entities[i].ConceptID != "" && !force {
				continue
			}
			old := entry.Entities[i].ConceptID
			resolveOneConceptID(&entry.Entities[i], tb)
			if entry.Entities[i].ConceptID != old {
				changed = true
			}
		}
		if !changed {
			continue
		}
		entry.UpdatedAt = time.Now()
		if err := tm.Add(context.Background(), entry); err != nil {
			return updated, fmt.Errorf("update entry %q: %w", eid, err)
		}
		updated++
	}
	return updated, nil
}

// rebuildRunsWithEntities walks a Run sequence's flat text, locates
// pattern occurrences, and emits a new Run sequence with PlaceholderRuns
// inserted at the matched ranges. Returns the new sequence and the
// number of entity hits inserted. Inline-code runs in the input are
// dropped — pattern matching is only meaningful against the textual
// projection.
func rebuildRunsWithEntities(runs []model.Run, patterns []EntityPatternRequest) ([]model.Run, int) {
	text := model.FlattenRuns(runs)

	type entityHit struct {
		start      int
		end        int
		entityType string
		text       string
	}
	runes := []rune(text)
	var hits []entityHit
	for _, p := range patterns {
		patLen := len([]rune(p.Text))
		for _, pos := range findPatternOccurrences(text, p.Text, p.CaseSensitive) {
			actualText := string(runes[pos : pos+patLen])
			hits = append(hits, entityHit{
				start:      pos,
				end:        pos + patLen,
				entityType: p.EntityType,
				text:       actualText,
			})
		}
	}
	slices.SortFunc(hits, func(a, b entityHit) int { return cmp.Compare(a.start, b.start) })
	var filtered []entityHit
	lastEnd := 0
	for _, h := range hits {
		if h.start >= lastEnd {
			filtered = append(filtered, h)
			lastEnd = h.end
		}
	}

	dtos := make([]EntityAnnotationDTO, len(filtered))
	for i, h := range filtered {
		dtos[i] = EntityAnnotationDTO{
			Text:  h.text,
			Type:  h.entityType,
			Start: h.start,
			End:   h.end,
		}
	}
	return buildRunsWithEntities(text, dtos), len(filtered)
}

// findPatternOccurrences returns rune positions of all non-overlapping occurrences.
func findPatternOccurrences(text, pattern string, caseSensitive bool) []int {
	if pattern == "" {
		return nil
	}
	searchText := text
	searchPattern := pattern
	if !caseSensitive {
		searchText = strings.ToLower(text)
		searchPattern = strings.ToLower(pattern)
	}

	var positions []int
	runes := []rune(searchText)
	patternRunes := []rune(searchPattern)
	patLen := len(patternRunes)

	for i := 0; i <= len(runes)-patLen; {
		if string(runes[i:i+patLen]) == string(patternRunes) {
			positions = append(positions, i)
			i += patLen
		} else {
			i++
		}
	}
	return positions
}

// buildEntityMappingsFromVariantRuns walks every variant's entity spans
// (materialised on demand from its Run sequence) and produces a unified
// EntityMapping list indexed by PlaceholderID. Values are populated per
// locale from the corresponding variant's entity span.
func buildEntityMappingsFromVariantRuns(variants map[model.LocaleID][]model.Run) []memory.EntityMapping {
	if len(variants) == 0 {
		return nil
	}
	type entKey struct {
		id    string
		eType string
	}
	byKey := make(map[entKey]*memory.EntityMapping)
	var order []entKey
	for loc, runs := range variants {
		for _, r := range runs {
			if r.Ph == nil || !model.IsEntityTypeString(r.Ph.Type) {
				continue
			}
			key := entKey{id: r.Ph.ID, eType: r.Ph.Type}
			em, ok := byKey[key]
			if !ok {
				em = &memory.EntityMapping{
					PlaceholderID: r.Ph.ID,
					Type:          model.EntityType(r.Ph.Type),
					Values:        make(map[model.LocaleID]memory.EntityValue),
				}
				byKey[key] = em
				order = append(order, key)
			}
			em.Values[loc] = memory.EntityValue{Text: r.Ph.Data}
		}
	}
	out := make([]memory.EntityMapping, 0, len(order))
	for _, k := range order {
		out = append(out, *byKey[k])
	}
	return out
}

// resolveConceptIDs looks up each entity mapping's text in the terms store
// and sets ConceptID when a concept matches. Looks up the first locale
// value that returns a hit.
func resolveConceptIDs(entities []memory.EntityMapping, tb *terms.SQLiteStore) {
	for i := range entities {
		resolveOneConceptID(&entities[i], tb)
	}
}

// resolveOneConceptID looks up one entity mapping's text values in the
// terms and sets ConceptID if a concept with a matching term is found.
func resolveOneConceptID(em *memory.EntityMapping, tb *terms.SQLiteStore) {
	if tb == nil {
		return
	}
	// Try each locale's entity value text against the terms store.
	for loc, val := range em.Values {
		if val.Text == "" {
			continue
		}
		matches, err := tb.Lookup(context.Background(), val.Text, terms.LookupOptions{
			SourceLocale:  loc,
			CaseSensitive: false,
			MinScore:      1.0, // exact or normalized match only
			MatchModes:    []model.MatchStrategy{model.MatchStrategyExact, model.MatchStrategyNormalized},
		})
		if err == nil && len(matches) > 0 {
			em.ConceptID = matches[0].Concept.ID
			return
		}
	}
}
