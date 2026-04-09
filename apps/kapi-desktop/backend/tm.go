package backend

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/sievepen"
)

// --- DTOs ---

// ResourceInfo describes a named resource (TM or termbase) in KAPI_HOME.
type ResourceInfo struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Modified string `json:"modified"` // ISO 8601
}

// TMEntryDTO is the frontend-facing TM entry.
type TMEntryDTO struct {
	ID           string            `json:"id"`
	SourceText   string            `json:"source_text"`
	TargetText   string            `json:"target_text"`
	SourceCoded  string            `json:"source_coded"`
	TargetCoded  string            `json:"target_coded"`
	SourceSpans  []SpanDTO         `json:"source_spans"`
	TargetSpans  []SpanDTO         `json:"target_spans"`
	SourceLocale string            `json:"source_locale"`
	TargetLocale string            `json:"target_locale"`
	ProjectID    string            `json:"project_id"`
	Properties   map[string]string `json:"properties,omitempty"`
	CreatedAt    string            `json:"created_at"`
	UpdatedAt    string            `json:"updated_at"`
}

// SpanDTO is the frontend-facing inline span, matching the SpanInfo TypeScript type.
type SpanDTO struct {
	SpanType    string `json:"span_type"`              // "opening"|"closing"|"placeholder"
	Type        string `json:"type"`                   // "fmt:bold", "entity:person"
	Data        string `json:"data"`                   // raw markup or entity value
	DisplayText string `json:"display_text,omitempty"` // optional label override
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
	Type  string `json:"type"` // "entity:person" etc.
	Start int    `json:"start"`
	End   int    `json:"end"`
}

// AddTMEntryRequest is the request to add a new TM entry.
type AddTMEntryRequest struct {
	Source       string    `json:"source"`
	Target       string    `json:"target"`
	SourceCoded  string    `json:"source_coded,omitempty"`
	TargetCoded  string    `json:"target_coded,omitempty"`
	SourceSpans  []SpanDTO `json:"source_spans,omitempty"`
	TargetSpans  []SpanDTO `json:"target_spans,omitempty"`
	SourceLocale string    `json:"source_locale"`
	TargetLocale string    `json:"target_locale"`
	ProjectID    string    `json:"project_id"`
}

// UpdateTMEntryRequest is the request to update a TM entry.
type UpdateTMEntryRequest struct {
	EntryID      string    `json:"entry_id"`
	Source       string    `json:"source"`
	Target       string    `json:"target"`
	TargetCoded  string    `json:"target_coded,omitempty"`
	TargetSpans  []SpanDTO `json:"target_spans,omitempty"`
	SourceLocale string    `json:"source_locale"`
	TargetLocale string    `json:"target_locale"`
	ProjectID    string    `json:"project_id"`
}

// ImportResult reports the outcome of an import operation.
type ImportResult struct {
	Count int `json:"count"`
}

// AnnotateEntitiesRequest is the request to batch-annotate entities on TM entries.
type AnnotateEntitiesRequest struct {
	EntryIDs []string               `json:"entry_ids"`
	Patterns []EntityPatternRequest `json:"patterns"`
}

// EntityPatternRequest defines a text→entity mapping for batch annotation.
type EntityPatternRequest struct {
	Text          string `json:"text"`
	EntityType    string `json:"entity_type"` // "entity:person" etc.
	CaseSensitive bool   `json:"case_sensitive"`
}

// AnnotateResult reports the outcome of a batch entity annotation.
type AnnotateResult struct {
	EntriesUpdated int `json:"entries_updated"`
	EntitiesAdded  int `json:"entities_added"`
}

// TMFacets is the frontend-facing facet data for the sidebar.
type TMFacets struct {
	LocalePairs []LocalePairFacetDTO `json:"locale_pairs"`
	Projects    []ProjectFacetDTO    `json:"projects"`
	EntityTypes []EntityTypeFacetDTO `json:"entity_types"`
	HasCodes    int                  `json:"has_codes"`
	NoCodes     int                  `json:"no_codes"`
}

// LocalePairFacetDTO is a locale pair with its entry count.
type LocalePairFacetDTO struct {
	SourceLocale string `json:"source_locale"`
	TargetLocale string `json:"target_locale"`
	Count        int    `json:"count"`
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

// TMGroupedResult is a source text with all target translations.
type TMGroupedResult struct {
	SourceText   string        `json:"source_text"`
	SourceCoded  string        `json:"source_coded"`
	SourceSpans  []SpanDTO     `json:"source_spans"`
	SourceLocale string        `json:"source_locale"`
	Targets      []TMTargetDTO `json:"targets"`
}

// TMTargetDTO is a single target translation within a grouped result.
type TMTargetDTO struct {
	ID           string    `json:"id"`
	TargetText   string    `json:"target_text"`
	TargetCoded  string    `json:"target_coded"`
	TargetSpans  []SpanDTO `json:"target_spans"`
	TargetLocale string    `json:"target_locale"`
	ProjectID    string    `json:"project_id"`
	UpdatedAt    string    `json:"updated_at"`
}

// TMGroupedSearchResult is the paginated result from SearchTMEntriesGrouped.
type TMGroupedSearchResult struct {
	Groups     []TMGroupedResult `json:"groups"`
	TotalCount int               `json:"total_count"`
}

// TMSearchFilter is the frontend-facing search filter.
type TMSearchFilter struct {
	ProjectID   string   `json:"project_id,omitempty"`
	EntityTypes []string `json:"entity_types,omitempty"`
	HasCodes    *bool    `json:"has_codes,omitempty"`
}

// --- Conversion helpers ---

func spanTypeStr(st model.SpanType) string {
	switch st {
	case model.SpanOpening:
		return "opening"
	case model.SpanClosing:
		return "closing"
	default:
		return "placeholder"
	}
}

// parseSpanType converts a string span type to a model.SpanType.
func parseSpanType(s string) model.SpanType {
	switch s {
	case "opening":
		return model.SpanOpening
	case "closing":
		return model.SpanClosing
	default:
		return model.SpanPlaceholder
	}
}

// fragmentFromDTO builds a model.Fragment from coded text and span DTOs.
// If codedText is empty, it falls back to plain text via model.NewFragment.
func fragmentFromDTO(plainText, codedText string, spans []SpanDTO) *model.Fragment {
	if codedText == "" {
		return model.NewFragment(plainText)
	}
	frag := &model.Fragment{CodedText: codedText}
	for _, s := range spans {
		frag.Spans = append(frag.Spans, &model.Span{
			SpanType:    parseSpanType(s.SpanType),
			Type:        s.Type,
			Data:        s.Data,
			DisplayText: s.DisplayText,
		})
	}
	return frag
}

func fragmentToDTO(frag *model.Fragment) (codedText string, spans []SpanDTO) {
	if frag == nil {
		return "", nil
	}
	codedText = frag.CodedText
	spans = make([]SpanDTO, 0, len(frag.Spans))
	for _, s := range frag.Spans {
		spans = append(spans, SpanDTO{
			SpanType:    spanTypeStr(s.SpanType),
			Type:        s.Type,
			Data:        s.Data,
			DisplayText: s.DisplayText,
		})
	}
	return codedText, spans
}

func tmEntryToDTO(entry sievepen.TMEntry) TMEntryDTO {
	srcCoded, srcSpans := fragmentToDTO(entry.Source)
	tgtCoded, tgtSpans := fragmentToDTO(entry.Target)
	return TMEntryDTO{
		ID:           entry.ID,
		SourceText:   entry.SourceText(),
		TargetText:   entry.TargetText(),
		SourceCoded:  srcCoded,
		TargetCoded:  tgtCoded,
		SourceSpans:  srcSpans,
		TargetSpans:  tgtSpans,
		SourceLocale: string(entry.SourceLocale),
		TargetLocale: string(entry.TargetLocale),
		ProjectID:    entry.ProjectID,
		Properties:   entry.Properties,
		CreatedAt:    entry.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    entry.UpdatedAt.Format(time.RFC3339),
	}
}

// --- Resource discovery ---

// ListNamedTMs returns named TMs from KAPI_HOME/tm/.
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
		return cmp.Compare(b.Modified, a.Modified) // newest first
	})
	return result
}

func namedResourceDir(kind string) string {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(cfgDir, "kapi", kind)
}

// --- Recovery ---

// RecoverResource backs up a corrupt .db file to .db.bak and returns the backup path.
// The caller should then create a fresh resource at the original path.
func (a *App) RecoverResource(path string) (string, error) {
	bakPath := path + ".bak"
	// Remove existing backup if present.
	_ = os.Remove(bakPath)
	if err := os.Rename(path, bakPath); err != nil {
		return "", fmt.Errorf("backup %q: %w", path, err)
	}
	return bakPath, nil
}

// --- Lifecycle ---

// OpenTM opens a SQLite TM file and returns a handle ID.
func (a *App) OpenTM(path string) (string, error) {
	tm, err := sievepen.NewSQLiteTM(path)
	if err != nil {
		return "", fmt.Errorf("open TM %q: %w", path, err)
	}
	return a.tmHandles.Open(tm), nil
}

// OpenTMDialog shows a native file dialog to open a TM.
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

// CreateTM creates a new empty TM at the given path.
func (a *App) CreateTM(path string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create directory: %w", err)
	}
	return a.OpenTM(path)
}

// CreateNamedTM creates a new named TM in KAPI_HOME/tm/.
func (a *App) CreateNamedTM(name string) (string, error) {
	dir := namedResourceDir("tm")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create TM directory: %w", err)
	}
	path := filepath.Join(dir, name+".db")
	return a.OpenTM(path)
}

// CloseTM closes an open TM by handle.
func (a *App) CloseTM(handle string) {
	_ = a.tmHandles.Close(handle)
}

// GetTMStats returns stats for an open TM.
func (a *App) GetTMStats(handle string) *TMStats {
	tm, ok := a.tmHandles.Get(handle)
	if !ok {
		return nil
	}
	return &TMStats{
		Count: tm.Count(),
	}
}

// GetTMActivityStats returns daily entry counts over time.
func (a *App) GetTMActivityStats(handle string) []sievepen.ActivityStat {
	tm, ok := a.tmHandles.Get(handle)
	if !ok {
		return nil
	}
	return tm.ActivityStats()
}

// GetTMLocaleStats returns entry counts grouped by locale pair.
func (a *App) GetTMLocaleStats(handle string) []sievepen.LocalePairStat {
	tm, ok := a.tmHandles.Get(handle)
	if !ok {
		return nil
	}
	return tm.LocalePairStats()
}

// --- CRUD ---

// SearchTMEntries searches TM entries by query with pagination.
func (a *App) SearchTMEntries(handle, query, srcLocale, tgtLocale string, offset, limit int) *TMSearchResult {
	tm, ok := a.tmHandles.Get(handle)
	if !ok {
		return &TMSearchResult{}
	}
	entries, total := tm.SearchEntries(query, srcLocale, tgtLocale, offset, limit)
	dtos := make([]TMEntryDTO, 0, len(entries))
	for _, e := range entries {
		dtos = append(dtos, tmEntryToDTO(e))
	}
	return &TMSearchResult{Entries: dtos, TotalCount: total}
}

// SearchTMEntriesFiltered searches TM entries with facet filters.
func (a *App) SearchTMEntriesFiltered(handle, query, srcLocale, tgtLocale string, filter TMSearchFilter, offset, limit int) *TMSearchResult {
	tm, ok := a.tmHandles.Get(handle)
	if !ok {
		return &TMSearchResult{}
	}
	entries, total := tm.SearchEntriesFiltered(query, srcLocale, tgtLocale, toSearchFilter(filter), offset, limit)
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
	entry, found := tm.GetEntry(entryID)
	if !found {
		return nil
	}
	dto := tmEntryToDTO(entry)
	return &dto
}

// AddTMEntry adds a new TM entry.
func (a *App) AddTMEntry(handle string, req AddTMEntryRequest) error {
	tm, ok := a.tmHandles.Get(handle)
	if !ok {
		return fmt.Errorf("TM handle %q not found", handle)
	}
	entry := sievepen.TMEntry{
		ID:           id.New(),
		ProjectID:    req.ProjectID,
		Source:       fragmentFromDTO(req.Source, req.SourceCoded, req.SourceSpans),
		Target:       fragmentFromDTO(req.Target, req.TargetCoded, req.TargetSpans),
		SourceLocale: model.LocaleID(req.SourceLocale),
		TargetLocale: model.LocaleID(req.TargetLocale),
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	return tm.Add(entry)
}

// UpdateTMEntry updates an existing TM entry.
func (a *App) UpdateTMEntry(handle string, req UpdateTMEntryRequest) error {
	tm, ok := a.tmHandles.Get(handle)
	if !ok {
		return fmt.Errorf("TM handle %q not found", handle)
	}
	existing, found := tm.GetEntry(req.EntryID)
	if !found {
		return fmt.Errorf("entry %q not found", req.EntryID)
	}
	existing.Target = fragmentFromDTO(req.Target, req.TargetCoded, req.TargetSpans)
	existing.UpdatedAt = time.Now()
	return tm.Add(existing) // Add with same ID = update
}

// DeleteTMEntry deletes a single TM entry.
func (a *App) DeleteTMEntry(handle, entryID string) error {
	tm, ok := a.tmHandles.Get(handle)
	if !ok {
		return fmt.Errorf("TM handle %q not found", handle)
	}
	return tm.Delete(entryID)
}

// DeleteTMEntries deletes multiple TM entries.
func (a *App) DeleteTMEntries(handle string, entryIDs []string) error {
	tm, ok := a.tmHandles.Get(handle)
	if !ok {
		return fmt.Errorf("TM handle %q not found", handle)
	}
	for _, eid := range entryIDs {
		if err := tm.Delete(eid); err != nil {
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

	// Build a Fragment with entity spans from the annotated text.
	frag := buildFragmentWithEntities(req.Text, req.Entities)

	// Build a Block to carry the fragment and entity annotations.
	block := &model.Block{
		ID:           "lookup",
		Translatable: true,
		Source:       []*model.Segment{{ID: "s1", Content: frag}},
		Annotations:  make(map[string]model.Annotation),
	}

	// Add entity annotations to the block (used by the matching pipeline).
	for i, ea := range req.Entities {
		block.Annotations[fmt.Sprintf("entity:%d", i)] = &model.EntityAnnotation{
			Text:     ea.Text,
			Type:     model.EntityType(ea.Type),
			Position: model.TextRange{Start: ea.Start, End: ea.End},
			Source:   model.ExtractionSourceManual,
		}
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

	matches, err := tm.Lookup(block, model.LocaleID(req.SourceLocale), model.LocaleID(req.TargetLocale), opts)
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

// buildFragmentWithEntities constructs a Fragment from plain text + entity annotations.
// Entity annotations are converted to placeholder spans in the coded text.
func buildFragmentWithEntities(text string, entities []EntityAnnotationDTO) *model.Fragment {
	if len(entities) == 0 {
		return model.NewFragment(text)
	}

	// Sort entities by start position.
	sorted := make([]EntityAnnotationDTO, len(entities))
	copy(sorted, entities)
	slices.SortFunc(sorted, func(a, b EntityAnnotationDTO) int {
		return cmp.Compare(a.Start, b.Start)
	})

	runes := []rune(text)
	frag := &model.Fragment{}
	pos := 0

	for i, ea := range sorted {
		if ea.Start < pos || ea.Start >= len(runes) || ea.End > len(runes) {
			continue // skip invalid or overlapping
		}
		// Add text before this entity.
		if ea.Start > pos {
			frag.AppendText(string(runes[pos:ea.Start]))
		}
		// Add entity span.
		frag.AppendSpan(&model.Span{
			SpanType: model.SpanPlaceholder,
			Type:     ea.Type,
			ID:       fmt.Sprintf("e%d", i+1),
			Data:     ea.Text,
		})
		pos = ea.End
	}

	// Add remaining text.
	if pos < len(runes) {
		frag.AppendText(string(runes[pos:]))
	}

	return frag
}

// --- Import / Export ---

// ImportTMXDialog shows a file dialog and imports a TMX file into the TM.
func (a *App) ImportTMXDialog(handle, srcLocale, tgtLocale string) (*ImportResult, error) {
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

	count, err := sievepen.ImportTMX(tm, f, model.LocaleID(srcLocale), model.LocaleID(tgtLocale))
	if err != nil {
		return nil, fmt.Errorf("import TMX: %w", err)
	}
	return &ImportResult{Count: count}, nil
}

// ExportTMXDialog shows a save dialog and exports the TM as TMX.
func (a *App) ExportTMXDialog(handle, srcLocale, tgtLocale string) error {
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

	return sievepen.ExportTMX(tm, f, model.LocaleID(srcLocale), model.LocaleID(tgtLocale))
}

// --- Grouped search & facets ---

// GetTMFacets returns facet data for the TM sidebar.
func (a *App) GetTMFacets(handle string) *TMFacets {
	tm, ok := a.tmHandles.Get(handle)
	if !ok {
		return nil
	}
	data := tm.FacetStats()
	result := &TMFacets{HasCodes: data.HasCodes, NoCodes: data.NoCodes}
	for _, lp := range data.LocalePairs {
		result.LocalePairs = append(result.LocalePairs, LocalePairFacetDTO{
			SourceLocale: lp.SourceLocale,
			TargetLocale: lp.TargetLocale,
			Count:        lp.Count,
		})
	}
	for _, p := range data.Projects {
		result.Projects = append(result.Projects, ProjectFacetDTO{
			ProjectID: p.ProjectID,
			Count:     p.Count,
		})
	}
	for _, et := range data.EntityTypes {
		result.EntityTypes = append(result.EntityTypes, EntityTypeFacetDTO{
			Type:  et.Type,
			Count: et.Count,
		})
	}
	return result
}

// SearchTMEntriesGrouped searches TM entries grouped by source text.
func (a *App) SearchTMEntriesGrouped(handle, query, srcLocale string, offset, limit int) *TMGroupedSearchResult {
	tm, ok := a.tmHandles.Get(handle)
	if !ok {
		return &TMGroupedSearchResult{}
	}
	groups, total := tm.SearchEntriesGrouped(query, srcLocale, offset, limit)
	result := &TMGroupedSearchResult{TotalCount: total}
	for _, g := range groups {
		srcCoded, srcSpans := fragmentToDTO(g.Source)
		gr := TMGroupedResult{
			SourceText:   g.SourceText,
			SourceCoded:  srcCoded,
			SourceSpans:  srcSpans,
			SourceLocale: string(g.SourceLocale),
		}
		for _, t := range g.Targets {
			tgtCoded, tgtSpans := fragmentToDTO(t.Target)
			gr.Targets = append(gr.Targets, TMTargetDTO{
				ID:           t.ID,
				TargetText:   t.TargetText(),
				TargetCoded:  tgtCoded,
				TargetSpans:  tgtSpans,
				TargetLocale: string(t.TargetLocale),
				ProjectID:    t.ProjectID,
				UpdatedAt:    t.UpdatedAt.Format(time.RFC3339),
			})
		}
		result.Groups = append(result.Groups, gr)
	}
	return result
}

// SearchTMEntriesGroupedFiltered searches TM entries grouped by source text with facet filters.
func (a *App) SearchTMEntriesGroupedFiltered(handle, query, srcLocale string, filter TMSearchFilter, offset, limit int) *TMGroupedSearchResult {
	tm, ok := a.tmHandles.Get(handle)
	if !ok {
		return &TMGroupedSearchResult{}
	}
	groups, total := tm.SearchEntriesGroupedFiltered(query, srcLocale, toSearchFilter(filter), offset, limit)
	result := &TMGroupedSearchResult{TotalCount: total}
	for _, g := range groups {
		srcCoded, srcSpans := fragmentToDTO(g.Source)
		gr := TMGroupedResult{
			SourceText:   g.SourceText,
			SourceCoded:  srcCoded,
			SourceSpans:  srcSpans,
			SourceLocale: string(g.SourceLocale),
		}
		for _, t := range g.Targets {
			tgtCoded, tgtSpans := fragmentToDTO(t.Target)
			gr.Targets = append(gr.Targets, TMTargetDTO{
				ID:           t.ID,
				TargetText:   t.TargetText(),
				TargetCoded:  tgtCoded,
				TargetSpans:  tgtSpans,
				TargetLocale: string(t.TargetLocale),
				ProjectID:    t.ProjectID,
				UpdatedAt:    t.UpdatedAt.Format(time.RFC3339),
			})
		}
		result.Groups = append(result.Groups, gr)
	}
	return result
}

// toSearchFilter converts the frontend DTO to the sievepen filter type.
func toSearchFilter(f TMSearchFilter) sievepen.SearchFilter {
	return sievepen.SearchFilter{
		ProjectID:   f.ProjectID,
		EntityTypes: f.EntityTypes,
		HasCodes:    f.HasCodes,
	}
}

// --- Batch entity annotation ---

// AnnotateEntities applies entity annotations to selected TM entries,
// converting plain text occurrences to entity spans. This enables
// generalized matching (e.g., "Bob" → {PERSON} allows matching with "Jane").
func (a *App) AnnotateEntities(handle string, req AnnotateEntitiesRequest) (*AnnotateResult, error) {
	tm, ok := a.tmHandles.Get(handle)
	if !ok {
		return nil, fmt.Errorf("TM handle %q not found", handle)
	}

	var entriesUpdated, entitiesAdded int

	for _, eid := range req.EntryIDs {
		entry, found := tm.GetEntry(eid)
		if !found {
			continue
		}

		srcHas, _ := annotateFragment(entry.Source, req.Patterns)
		tgtHas, _ := annotateFragment(entry.Target, req.Patterns)

		if srcHas || tgtHas {
			var srcCount, tgtCount int
			if srcHas {
				entry.Source, srcCount = rebuildWithEntities(entry.Source, req.Patterns)
			}
			if tgtHas {
				entry.Target, tgtCount = rebuildWithEntities(entry.Target, req.Patterns)
			}
			// Rebuild entity mappings from the annotated fragments.
			entry.Entities = buildEntityMappings(entry.Source, entry.Target)
			entry.UpdatedAt = time.Now()
			if err := tm.Add(entry); err != nil {
				return nil, fmt.Errorf("update entry %q: %w", eid, err)
			}
			entriesUpdated++
			entitiesAdded += srcCount + tgtCount
		}
	}

	return &AnnotateResult{
		EntriesUpdated: entriesUpdated,
		EntitiesAdded:  entitiesAdded,
	}, nil
}

// annotateFragment checks if a fragment's plain text contains any pattern matches.
// Returns whether updates are needed and the count of matches found.
func annotateFragment(frag *model.Fragment, patterns []EntityPatternRequest) (bool, int) {
	if frag == nil {
		return false, 0
	}
	text := frag.Text()
	count := 0
	for _, p := range patterns {
		matches := findPatternOccurrences(text, p.Text, p.CaseSensitive)
		count += len(matches)
	}
	return count > 0, count
}

// rebuildWithEntities creates a new Fragment with entity spans inserted for pattern matches.
// Returns the new fragment and the actual number of entities inserted (after overlap filtering).
func rebuildWithEntities(frag *model.Fragment, patterns []EntityPatternRequest) (*model.Fragment, int) {
	if frag == nil {
		return nil, 0
	}

	text := frag.Text()

	// Collect all entity positions from patterns.
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
			// Use the actual text from the source (preserves original casing).
			actualText := string(runes[pos : pos+patLen])
			hits = append(hits, entityHit{
				start:      pos,
				end:        pos + patLen,
				entityType: p.EntityType,
				text:       actualText,
			})
		}
	}

	// Sort by position, remove overlaps.
	slices.SortFunc(hits, func(a, b entityHit) int { return cmp.Compare(a.start, b.start) })
	var filtered []entityHit
	lastEnd := 0
	for _, h := range hits {
		if h.start >= lastEnd {
			filtered = append(filtered, h)
			lastEnd = h.end
		}
	}

	// Convert to EntityAnnotationDTO and use buildFragmentWithEntities.
	dtos := make([]EntityAnnotationDTO, len(filtered))
	for i, h := range filtered {
		dtos[i] = EntityAnnotationDTO{
			Text:  h.text,
			Type:  h.entityType,
			Start: h.start,
			End:   h.end,
		}
	}

	return buildFragmentWithEntities(text, dtos), len(filtered)
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
			i += patLen // skip past this match
		} else {
			i++
		}
	}
	return positions
}

// buildEntityMappings creates EntityMapping entries from entity spans in source and target.
func buildEntityMappings(source, target *model.Fragment) []sievepen.EntityMapping {
	if source == nil {
		return nil
	}
	srcEntities := source.EntitySpans()
	srcValues := source.EntityValues()
	tgtValues := map[string]string{}
	if target != nil {
		tgtValues = target.EntityValues()
	}

	var mappings []sievepen.EntityMapping
	for _, s := range srcEntities {
		mappings = append(mappings, sievepen.EntityMapping{
			PlaceholderID: s.ID,
			Type:          model.EntityType(s.Type),
			SourceValue:   srcValues[s.ID],
			TargetValue:   tgtValues[s.ID],
		})
	}
	return mappings
}
