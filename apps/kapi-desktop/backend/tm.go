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
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/sievepen"
	"github.com/neokapi/neokapi/termbase"
)

// --- DTOs ---

// ResourceInfo describes a named resource (TM or termbase) in KAPI_HOME.
type ResourceInfo struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Modified string `json:"modified"` // ISO 8601
}

// OriginDTO is the frontend-facing TM entry origin (provenance).
type OriginDTO struct {
	Source    string `json:"source"`
	Key       string `json:"key,omitempty"`
	Reference string `json:"reference,omitempty"`
	AddedAt   string `json:"added_at"`
	AddedBy   string `json:"added_by,omitempty"`
	SessionID string `json:"session_id,omitempty"`
}

// VariantDTO is a single language variant of a multilingual TM entry. Inline
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
	ConceptID     string                    `json:"concept_id,omitempty"` // optional termbase cross-reference
}

// TMEntryDTO is the frontend-facing multilingual TM entry.
type TMEntryDTO struct {
	ID          string                `json:"id"`
	ProjectID   string                `json:"project_id"`
	Variants    map[string]VariantDTO `json:"variants"`
	HintSrcLang string                `json:"hint_src_lang"`
	Entities    []EntityMappingDTO    `json:"entities,omitempty"`
	Properties  map[string]string     `json:"properties,omitempty"`
	Note        string                `json:"note,omitempty"`
	Origins     []OriginDTO           `json:"origins,omitempty"`
	CreatedAt   string                `json:"created_at"`
	UpdatedAt   string                `json:"updated_at"`
}

// TMSearchResult is the paginated result from SearchTMEntries.
type TMSearchResult struct {
	Entries    []TMEntryDTO `json:"entries"`
	TotalCount int          `json:"total_count"`
}

// TMStats is the stats response for an open TM.
type TMStats struct {
	Count int    `json:"count"`
	Path  string `json:"path"`
	Size  int64  `json:"size"`
}

// TMMatchDTO is a single match from entity-aware TM lookup.
type TMMatchDTO struct {
	Entry             TMEntryDTO            `json:"entry"`
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

// LookupTMRequest is the request for entity-aware TM lookup.
type LookupTMRequest struct {
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

// AddTMEntryRequest is the request to add a new multilingual TM entry.
// Callers populate Variants with one VariantInput per locale; the server uses
// each variant's Run sequence, falling back to plain Text when Runs is empty.
type AddTMEntryRequest struct {
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

// UpdateTMEntryRequest is the request to update a TM entry. The Variants map
// replaces the stored variants wholesale.
type UpdateTMEntryRequest struct {
	EntryID     string                     `json:"entry_id"`
	Variants    map[string]VariantInputDTO `json:"variants"`
	HintSrcLang string                     `json:"hint_src_lang"`
	ProjectID   string                     `json:"project_id"`
	Note        string                     `json:"note,omitempty"`
	Origins     []OriginDTO                `json:"origins,omitempty"`
}

// ImportResult reports the outcome of an import operation.
type ImportResult struct {
	SessionID string `json:"session_id"`
	Count     int    `json:"count"`
}

// AnnotateEntitiesRequest is the request to batch-annotate entities on TM entries.
type AnnotateEntitiesRequest struct {
	EntryIDs       []string               `json:"entry_ids"`
	Patterns       []EntityPatternRequest `json:"patterns"`
	TermbaseHandle string                 `json:"termbase_handle,omitempty"` // optional: cross-ref entities against this termbase
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

// TMFacets is the frontend-facing facet data for the sidebar.
type TMFacets struct {
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

// TMSearchFilter is the frontend-facing search filter.
type TMSearchFilter struct {
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

// originsToDTO converts sievepen.Origin values to OriginDTO for the frontend.
func originsToDTO(in []sievepen.Origin) []OriginDTO {
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

// originsFromDTO converts request OriginDTOs to sievepen.Origin values,
// defaulting AddedAt to time.Now() when not supplied.
func originsFromDTO(in []OriginDTO) []sievepen.Origin {
	if len(in) == 0 {
		return nil
	}
	out := make([]sievepen.Origin, 0, len(in))
	now := time.Now()
	for _, o := range in {
		addedAt, _ := time.Parse(time.RFC3339, o.AddedAt)
		if addedAt.IsZero() {
			addedAt = now
		}
		out = append(out, sievepen.Origin{
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
func entitiesToDTO(in []sievepen.EntityMapping) []EntityMappingDTO {
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

func tmEntryToDTO(entry sievepen.TMEntry) TMEntryDTO {
	variants := make(map[string]VariantDTO, len(entry.Variants))
	for loc, runs := range entry.Variants {
		variants[string(loc)] = runsToVariantDTO(loc, runs)
	}
	return TMEntryDTO{
		ID:          entry.ID,
		ProjectID:   entry.ProjectID,
		Variants:    variants,
		HintSrcLang: string(entry.HintSrcLang),
		Entities:    entitiesToDTO(entry.Entities),
		Properties:  entry.Properties,
		Note:        entry.Note,
		Origins:     originsToDTO(entry.Origins),
		CreatedAt:   entry.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   entry.UpdatedAt.Format(time.RFC3339),
	}
}

func variantsFromInput(in map[string]VariantInputDTO) map[model.LocaleID][]model.Run {
	out := make(map[model.LocaleID][]model.Run, len(in))
	for loc, v := range in {
		if loc == "" {
			continue
		}
		out[model.LocaleID(loc)] = runsFromVariantInput(v)
	}
	return out
}

// --- Resource discovery ---

func (a *App) ListNamedTMs() []ResourceInfo {
	return listNamedResources("tm")
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

func (a *App) RecoverResource(path string) (string, error) {
	bakPath := path + ".bak"
	_ = os.Remove(bakPath)
	if err := os.Rename(path, bakPath); err != nil {
		return "", fmt.Errorf("backup %q: %w", path, err)
	}
	return bakPath, nil
}

// --- Lifecycle ---

func (a *App) OpenTM(path string) (string, error) {
	tm, err := sievepen.NewSQLiteTM(path)
	if err != nil {
		return "", fmt.Errorf("open TM %q: %w", path, err)
	}
	return a.tmHandles.Open(tm), nil
}

func (a *App) OpenTMDialog() (string, error) {
	if a.app == nil {
		return "", nil
	}
	path, err := a.app.Dialog.OpenFile().
		AddFilter("Translation Memory", "*.db").
		AddFilter("All Files", "*").
		PromptForSingleSelection()
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", nil
	}
	return a.OpenTM(path)
}

func (a *App) CreateTM(path string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create directory: %w", err)
	}
	return a.OpenTM(path)
}

func (a *App) CreateNamedTM(name string) (string, error) {
	dir := namedResourceDir("tm")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create TM directory: %w", err)
	}
	path := filepath.Join(dir, name+".db")
	return a.OpenTM(path)
}

func (a *App) CloseTM(handle string) {
	_ = a.tmHandles.Close(handle)
}

func (a *App) GetTMStats(handle string) *TMStats {
	tm, ok := a.tmHandles.Get(handle)
	if !ok {
		return nil
	}
	count, err := tm.Count(context.Background())
	if err != nil {
		return nil
	}
	return &TMStats{Count: count}
}

// GetTMActivityStats returns daily entry counts over time.
func (a *App) GetTMActivityStats(handle string) []sievepen.ActivityStat {
	tm, ok := a.tmHandles.Get(handle)
	if !ok {
		return nil
	}
	stats, err := tm.ActivityStats(context.Background())
	if err != nil {
		return nil
	}
	return stats
}

// GetTMLocaleStats returns per-locale entry counts. The legacy API name is
// preserved for frontend compatibility; the response is now a flat list of
// single-locale counts, not locale pairs.
func (a *App) GetTMLocaleStats(handle string) []sievepen.LocaleFacet {
	tm, ok := a.tmHandles.Get(handle)
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

// SearchTMEntries searches TM entries by query with pagination.
// anyLocale restricts the text search to entries with a variant in that
// locale; requireLocale additionally requires that variant to exist.
func (a *App) SearchTMEntries(handle, query, anyLocale, requireLocale string, offset, limit int) *TMSearchResult {
	tm, ok := a.tmHandles.Get(handle)
	if !ok {
		return &TMSearchResult{}
	}
	entries, total, err := tm.SearchEntries(context.Background(), sievepen.SearchParams{
		Query:         query,
		AnyLocale:     anyLocale,
		RequireLocale: requireLocale,
		Offset:        offset,
		Limit:         limit,
	})
	if err != nil {
		return &TMSearchResult{}
	}
	dtos := make([]TMEntryDTO, 0, len(entries))
	for _, e := range entries {
		dtos = append(dtos, tmEntryToDTO(e))
	}
	return &TMSearchResult{Entries: dtos, TotalCount: total}
}

// SearchTMEntriesFiltered searches TM entries with facet filters.
func (a *App) SearchTMEntriesFiltered(handle, query, anyLocale, requireLocale string, filter TMSearchFilter, offset, limit int) *TMSearchResult {
	tm, ok := a.tmHandles.Get(handle)
	if !ok {
		return &TMSearchResult{}
	}
	entries, total, err := tm.SearchEntriesFiltered(context.Background(), sievepen.SearchParams{
		Query:         query,
		AnyLocale:     anyLocale,
		RequireLocale: requireLocale,
		Filter:        toSearchFilter(filter),
		Offset:        offset,
		Limit:         limit,
	})
	if err != nil {
		return &TMSearchResult{}
	}
	dtos := make([]TMEntryDTO, 0, len(entries))
	for _, e := range entries {
		dtos = append(dtos, tmEntryToDTO(e))
	}
	return &TMSearchResult{Entries: dtos, TotalCount: total}
}

// GetTMEntry returns a single TM entry by ID.
func (a *App) GetTMEntry(handle, entryID string) *TMEntryDTO {
	tm, ok := a.tmHandles.Get(handle)
	if !ok {
		return nil
	}
	entry, found, err := tm.GetEntry(context.Background(), entryID)
	if err != nil || !found {
		return nil
	}
	dto := tmEntryToDTO(entry)
	return &dto
}

// AddTMEntry adds a new multilingual TM entry.
func (a *App) AddTMEntry(handle string, req AddTMEntryRequest) error {
	tm, ok := a.tmHandles.Get(handle)
	if !ok {
		return fmt.Errorf("TM handle %q not found", handle)
	}
	now := time.Now()
	entry := sievepen.TMEntry{
		ID:          id.New(),
		ProjectID:   req.ProjectID,
		Variants:    variantsFromInput(req.Variants),
		HintSrcLang: model.LocaleID(req.HintSrcLang),
		Note:        req.Note,
		Origins:     originsFromDTO(req.Origins),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	return tm.Add(context.Background(), entry)
}

// UpdateTMEntry updates an existing multilingual TM entry.
func (a *App) UpdateTMEntry(handle string, req UpdateTMEntryRequest) error {
	tm, ok := a.tmHandles.Get(handle)
	if !ok {
		return fmt.Errorf("TM handle %q not found", handle)
	}
	existing, found, err := tm.GetEntry(context.Background(), req.EntryID)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("entry %q not found", req.EntryID)
	}
	existing.Variants = variantsFromInput(req.Variants)
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

// DeleteTMEntry deletes a single TM entry.
func (a *App) DeleteTMEntry(handle, entryID string) error {
	tm, ok := a.tmHandles.Get(handle)
	if !ok {
		return fmt.Errorf("TM handle %q not found", handle)
	}
	return tm.Delete(context.Background(), entryID)
}

// DeleteTMEntries deletes multiple TM entries.
func (a *App) DeleteTMEntries(handle string, entryIDs []string) error {
	tm, ok := a.tmHandles.Get(handle)
	if !ok {
		return fmt.Errorf("TM handle %q not found", handle)
	}
	for _, eid := range entryIDs {
		if err := tm.Delete(context.Background(), eid); err != nil {
			return err
		}
	}
	return nil
}

// --- Entity-aware lookup ---

// LookupTM performs entity-aware TM lookup using the full tiered matching pipeline.
func (a *App) LookupTM(handle string, req LookupTMRequest) []TMMatchDTO {
	tm, ok := a.tmHandles.Get(handle)
	if !ok {
		return nil
	}

	runs := buildRunsWithEntities(req.Text, req.Entities)
	block := &model.Block{
		ID:           "lookup",
		Translatable: true,
		Source:       runs,
		Annotations:  make(map[string]any),
	}
	for i, ea := range req.Entities {
		block.SetAnno(fmt.Sprintf("entity:%d", i), &model.EntityAnnotation{
			Text:     ea.Text,
			Type:     model.EntityType(ea.Type),
			Position: model.RunRangeForBytes(block.Source, ea.Start, ea.End),
			Source:   model.ExtractionSourceManual,
		})
	}

	opts := sievepen.LookupOptions{
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
		a.logger.Printf("TM lookup error: %v", err)
		return nil
	}

	result := make([]TMMatchDTO, 0, len(matches))
	for _, m := range matches {
		dto := TMMatchDTO{
			Entry:     tmEntryToDTO(m.Entry),
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

// --- Import / Export ---

// ImportTMXDialog shows a file dialog and imports a TMX file into the TM.
// The importer creates one multilingual entry per TU with all TUVs as
// variants. A new ImportSession row is created for each invocation.
func (a *App) ImportTMXDialog(handle string) (*ImportResult, error) {
	if a.app == nil {
		return nil, nil
	}
	tm, ok := a.tmHandles.Get(handle)
	if !ok {
		return nil, fmt.Errorf("TM handle %q not found", handle)
	}

	path, err := a.app.Dialog.OpenFile().
		AddFilter("TMX Files", "*.tmx").
		AddFilter("All Files", "*").
		PromptForSingleSelection()
	if err != nil {
		return nil, err
	}
	if path == "" {
		return nil, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open TMX: %w", err)
	}
	defer f.Close()

	sid, count, err := sievepen.ImportTMXSession(context.Background(), tm, f, sievepen.ImportTMXOptions{
		OriginKey:     filepath.Base(path),
		OriginAddedBy: "tmx-import",
	})
	if err != nil {
		return nil, fmt.Errorf("import TMX: %w", err)
	}
	return &ImportResult{SessionID: sid, Count: count}, nil
}

// ExportTMXDialog shows a save dialog and exports the TM as TMX. When
// locales is empty, every variant present on each entry is emitted.
func (a *App) ExportTMXDialog(handle string, locales []string) error {
	if a.app == nil {
		return nil
	}
	tm, ok := a.tmHandles.Get(handle)
	if !ok {
		return fmt.Errorf("TM handle %q not found", handle)
	}

	path, err := a.app.Dialog.SaveFile().
		AddFilter("TMX Files", "*.tmx").
		SetFilename("export.tmx").
		PromptForSingleSelection()
	if err != nil {
		return err
	}
	if path == "" {
		return nil
	}
	if !strings.HasSuffix(strings.ToLower(path), ".tmx") {
		path += ".tmx"
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create TMX: %w", err)
	}
	defer f.Close()

	localeIDs := make([]model.LocaleID, 0, len(locales))
	for _, l := range locales {
		localeIDs = append(localeIDs, model.LocaleID(l))
	}
	return sievepen.ExportTMX(context.Background(), tm, f, localeIDs)
}

// --- Facets ---

func (a *App) GetTMFacets(handle string) *TMFacets {
	return a.GetTMFacetsFiltered(handle, "", "", "", TMSearchFilter{})
}

func (a *App) GetTMFacetsFiltered(handle, query, anyLocale, requireLocale string, filter TMSearchFilter) *TMFacets {
	tm, ok := a.tmHandles.Get(handle)
	if !ok {
		return nil
	}
	data, err := tm.FacetStatsFiltered(context.Background(), sievepen.SearchParams{
		Query:         query,
		AnyLocale:     anyLocale,
		RequireLocale: requireLocale,
		Filter:        toSearchFilter(filter),
	})
	if err != nil {
		return nil
	}
	return buildTMFacetsDTO(data)
}

func buildTMFacetsDTO(data sievepen.FacetData) *TMFacets {
	result := &TMFacets{HasCodes: data.HasCodes, NoCodes: data.NoCodes}
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

// toSearchFilter converts the frontend DTO to the sievepen filter type.
func toSearchFilter(f TMSearchFilter) sievepen.SearchFilter {
	sf := sievepen.SearchFilter{
		ProjectID:   f.ProjectID,
		SessionIDs:  f.SessionIDs,
		EntityTypes: f.EntityTypes,
		HasCodes:    f.HasCodes,
	}
	if len(f.EntityValues) > 0 {
		sf.EntityValues = make([]sievepen.EntityValueFilter, len(f.EntityValues))
		for i, ev := range f.EntityValues {
			sf.EntityValues[i] = sievepen.EntityValueFilter{Value: ev.Value, Type: ev.Type}
		}
	}
	return sf
}

// --- Import session CRUD ---

// ListTMImportSessions returns every session row in imported_at DESC order.
func (a *App) ListTMImportSessions(handle string) []ImportSessionDTO {
	tm, ok := a.tmHandles.Get(handle)
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

// GetTMImportSession fetches a single session by ID.
func (a *App) GetTMImportSession(handle, sessionID string) *ImportSessionDTO {
	tm, ok := a.tmHandles.Get(handle)
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

// DeleteTMImportSession removes a session; its origins keep pointing at
// empty session_id (see sievepen.DeleteImportSession).
func (a *App) DeleteTMImportSession(handle, sessionID string) error {
	tm, ok := a.tmHandles.Get(handle)
	if !ok {
		return fmt.Errorf("TM handle %q not found", handle)
	}
	return tm.DeleteImportSession(context.Background(), sessionID)
}

func importSessionToDTO(s sievepen.ImportSession) ImportSessionDTO {
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

// AnnotateEntities applies entity annotations to selected TM entries. The
// patterns are searched across every variant's plain text and entity spans
// are inserted where matches are found. Entity values are populated per
// locale from the matching variant.
//
// When TermbaseHandle is set, each new entity's text is looked up in the
// termbase; if a concept matches, its ID is stored on the EntityMapping
// so the TM entry cross-references the termbase.
func (a *App) AnnotateEntities(handle string, req AnnotateEntitiesRequest) (*AnnotateResult, error) {
	tm, ok := a.tmHandles.Get(handle)
	if !ok {
		return nil, fmt.Errorf("TM handle %q not found", handle)
	}

	// Optionally resolve concept IDs from the termbase.
	var tb *termbase.SQLiteTermBase
	if req.TermbaseHandle != "" {
		tb, _ = a.tbHandles.Get(req.TermbaseHandle)
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

// ResolveEntityConcepts re-links entities on TM entries to termbase concepts.
// Useful after a termbase import or when entities were created without a
// termbase available. Entries whose entities already have a ConceptID are
// skipped unless force is true.
func (a *App) ResolveEntityConcepts(tmHandle, tbHandle string, entryIDs []string, force bool) (int, error) {
	tm, ok := a.tmHandles.Get(tmHandle)
	if !ok {
		return 0, fmt.Errorf("TM handle %q not found", tmHandle)
	}
	tb, ok := a.tbHandles.Get(tbHandle)
	if !ok {
		return 0, fmt.Errorf("termbase handle %q not found", tbHandle)
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
func buildEntityMappingsFromVariantRuns(variants map[model.LocaleID][]model.Run) []sievepen.EntityMapping {
	if len(variants) == 0 {
		return nil
	}
	type entKey struct {
		id    string
		eType string
	}
	byKey := make(map[entKey]*sievepen.EntityMapping)
	var order []entKey
	for loc, runs := range variants {
		for _, r := range runs {
			if r.Ph == nil || !model.IsEntityTypeString(r.Ph.Type) {
				continue
			}
			key := entKey{id: r.Ph.ID, eType: r.Ph.Type}
			em, ok := byKey[key]
			if !ok {
				em = &sievepen.EntityMapping{
					PlaceholderID: r.Ph.ID,
					Type:          model.EntityType(r.Ph.Type),
					Values:        make(map[model.LocaleID]sievepen.EntityValue),
				}
				byKey[key] = em
				order = append(order, key)
			}
			em.Values[loc] = sievepen.EntityValue{Text: r.Ph.Data}
		}
	}
	out := make([]sievepen.EntityMapping, 0, len(order))
	for _, k := range order {
		out = append(out, *byKey[k])
	}
	return out
}

// resolveConceptIDs looks up each entity mapping's text in the termbase
// and sets ConceptID when a concept matches. Looks up the first locale
// value that returns a hit.
func resolveConceptIDs(entities []sievepen.EntityMapping, tb *termbase.SQLiteTermBase) {
	for i := range entities {
		resolveOneConceptID(&entities[i], tb)
	}
}

// resolveOneConceptID looks up one entity mapping's text values in the
// termbase and sets ConceptID if a concept with a matching term is found.
func resolveOneConceptID(em *sievepen.EntityMapping, tb *termbase.SQLiteTermBase) {
	if tb == nil {
		return
	}
	// Try each locale's entity value text against the termbase.
	for loc, val := range em.Values {
		if val.Text == "" {
			continue
		}
		matches, err := tb.Lookup(context.Background(), val.Text, termbase.LookupOptions{
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
