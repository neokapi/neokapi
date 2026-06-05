package sievepen

import (
	"cmp"
	"context"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"golang.org/x/text/unicode/norm"
)

// DefaultMaxEntries is the default maximum number of entries in an InMemoryTM.
// A value of 0 means unlimited.
const DefaultMaxEntries = 0

// variantKeys caches the pre-computed normalized text forms for one variant
// of an entry, avoiding repeated NormalizeText calls during lookup scans.
type variantKeys struct {
	plain       string
	structural  string
	generalized string
}

// storedEntry is an entry plus pre-computed per-variant keys.
type storedEntry struct {
	entry TMEntry
	keys  map[model.LocaleID]variantKeys
}

// InMemoryTM is a thread-safe, in-memory implementation of TMStore with
// multilingual entries and tiered content-aware matching.
type InMemoryTM struct {
	mu         sync.RWMutex
	entries    []storedEntry
	byID       map[string]int
	sessions   map[string]ImportSession
	maxEntries int
}

// InMemoryTMOption configures an InMemoryTM instance.
type InMemoryTMOption func(*InMemoryTM)

// WithMaxEntries sets the maximum number of entries; 0 = unlimited.
func WithMaxEntries(max int) InMemoryTMOption {
	return func(tm *InMemoryTM) { tm.maxEntries = max }
}

// NewInMemoryTM creates a new empty in-memory translation memory.
func NewInMemoryTM(opts ...InMemoryTMOption) *InMemoryTM {
	tm := &InMemoryTM{
		byID:       make(map[string]int),
		sessions:   make(map[string]ImportSession),
		maxEntries: DefaultMaxEntries,
	}
	for _, opt := range opts {
		opt(tm)
	}
	return tm
}

// MaxEntries returns the configured max entry count (0 = unlimited).
func (tm *InMemoryTM) MaxEntries() int { return tm.maxEntries }

// Close is a no-op for InMemoryTM.
func (tm *InMemoryTM) Close() error { return nil }

// Count returns the total number of entries.
func (tm *InMemoryTM) Count(_ context.Context) (int, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return len(tm.entries), nil
}

// Add inserts or updates a TM entry.
func (tm *InMemoryTM) Add(ctx context.Context, entry TMEntry) error {
	return tm.AddWithStream(ctx, entry, "")
}

// AddWithStream inserts or updates a TM entry. InMemoryTM ignores the
// stream parameter — it's a persistence concern for on-disk backends only.
func (tm *InMemoryTM) AddWithStream(_ context.Context, entry TMEntry, _ string) error {
	if entry.ID == "" {
		return ErrEntryIDRequired
	}
	if len(entry.Variants) == 0 {
		return ErrEntryNoVariants
	}
	now := time.Now()
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = now
	}
	if entry.UpdatedAt.IsZero() {
		entry.UpdatedAt = now
	}

	stored := storedEntry{entry: entry, keys: buildVariantKeys(entry)}

	tm.mu.Lock()
	defer tm.mu.Unlock()

	if idx, exists := tm.byID[entry.ID]; exists {
		tm.entries[idx] = stored
		return nil
	}
	if tm.maxEntries > 0 && len(tm.entries) >= tm.maxEntries {
		tm.evictOldest()
	}
	tm.byID[entry.ID] = len(tm.entries)
	tm.entries = append(tm.entries, stored)
	return nil
}

func buildVariantKeys(entry TMEntry) map[model.LocaleID]variantKeys {
	keys := make(map[model.LocaleID]variantKeys, len(entry.Variants))
	for loc, runs := range entry.Variants {
		if len(runs) == 0 {
			continue
		}
		keys[loc] = variantKeys{
			plain:       NormalizeText(model.FlattenRuns(runs)),
			structural:  NormalizeText(model.RunsStructuralText(runs)),
			generalized: NormalizeText(model.RunsGeneralizedText(runs)),
		}
	}
	return keys
}

func (tm *InMemoryTM) evictOldest() {
	if len(tm.entries) == 0 {
		return
	}
	oldest := tm.entries[0]
	delete(tm.byID, oldest.entry.ID)
	copy(tm.entries, tm.entries[1:])
	tm.entries = tm.entries[:len(tm.entries)-1]
	for id, idx := range tm.byID {
		tm.byID[id] = idx - 1
	}
}

// Delete removes an entry by ID.
func (tm *InMemoryTM) Delete(_ context.Context, id string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	idx, ok := tm.byID[id]
	if !ok {
		return ErrImportSessionMiss // reuse "not found" style error — matches sqlite behavior
	}
	lastIdx := len(tm.entries) - 1
	if idx != lastIdx {
		tm.entries[idx] = tm.entries[lastIdx]
		tm.byID[tm.entries[idx].entry.ID] = idx
	}
	tm.entries = tm.entries[:lastIdx]
	delete(tm.byID, id)
	return nil
}

// --- Lookup ---

// Lookup searches for matches using tiered matching against the source-locale
// variant of each stored entry. Entries lacking the target-locale variant
// are skipped. Returns TMMatch results ordered by match priority and score.
func (tm *InMemoryTM) Lookup(_ context.Context, source *model.Block, sourceLocale, targetLocale model.LocaleID, opts LookupOptions) ([]TMMatch, error) {
	if source == nil {
		return nil, nil
	}
	opts = ApplyDefaults(opts)
	runs := source.Source
	if len(runs) == 0 {
		return nil, nil
	}
	plainKey := NormalizeText(model.FlattenRuns(runs))
	structKey := NormalizeText(model.RunsStructuralText(runs))
	generalKey := NormalizeText(model.RunsGeneralizedText(runs))
	entityAnnotations := ExtractEntityAnnotations(source)
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.tieredLookup(plainKey, structKey, generalKey, entityAnnotations, sourceLocale, targetLocale, opts), nil
}

// LookupSegment searches for matches against a specific segment of the
// source block. See TranslationMemory.LookupSegment for the contract.
func (tm *InMemoryTM) LookupSegment(_ context.Context, source *model.Block, segmentIdx int, sourceLocale, targetLocale model.LocaleID, opts LookupOptions) ([]TMMatch, error) {
	if source == nil {
		return nil, nil
	}
	runs := source.SourceSegmentRuns(segmentIdx)
	if len(runs) == 0 {
		return nil, nil
	}
	opts = ApplyDefaults(opts)
	plainKey := NormalizeText(model.FlattenRuns(runs))
	structKey := NormalizeText(model.RunsStructuralText(runs))
	generalKey := NormalizeText(model.RunsGeneralizedText(runs))
	entityAnnotations := ExtractEntityAnnotations(source)
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.tieredLookup(plainKey, structKey, generalKey, entityAnnotations, sourceLocale, targetLocale, opts), nil
}

// LookupText searches for matches using plain text only.
func (tm *InMemoryTM) LookupText(_ context.Context, source string, sourceLocale, targetLocale model.LocaleID, opts LookupOptions) ([]TMMatch, error) {
	opts = ApplyDefaults(opts)
	opts.MatchModes = []MatchMode{MatchModePlain}
	normalized := NormalizeText(source)
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.tieredLookup(normalized, normalized, normalized, nil, sourceLocale, targetLocale, opts), nil
}

func (tm *InMemoryTM) tieredLookup(plainKey, structKey, generalKey string, entityAnnotations []*model.EntityAnnotation, sourceLocale, targetLocale model.LocaleID, opts LookupOptions) []TMMatch {
	var matches []TMMatch
	seen := make(map[string]bool)
	modeEnabled := MatchModesEnabled(opts.MatchModes)

	// Helper: check entry has both source and target variants.
	eligible := func(se *storedEntry) (variantKeys, bool) {
		k, ok := se.keys[sourceLocale]
		if !ok {
			return variantKeys{}, false
		}
		if !se.entry.HasLocale(targetLocale) {
			return variantKeys{}, false
		}
		if !projectAllowed(se.entry.ProjectID, opts) {
			return variantKeys{}, false
		}
		return k, true
	}

	// Tier 1-3: exact matches.
	for tierType, mode := range []struct {
		modeFlag MatchMode
		mt       MatchType
	}{
		{MatchModeGeneralized, MatchGeneralizedExact},
		{MatchModeStructural, MatchStructuralExact},
		{MatchModePlain, MatchExact},
	} {
		_ = tierType
		if !modeEnabled[mode.modeFlag] {
			continue
		}
		for i := range tm.entries {
			se := &tm.entries[i]
			if seen[se.entry.ID] {
				continue
			}
			k, ok := eligible(se)
			if !ok {
				continue
			}
			var storedKey, lookupKey string
			switch mode.modeFlag {
			case MatchModeGeneralized:
				storedKey, lookupKey = k.generalized, generalKey
			case MatchModeStructural:
				storedKey, lookupKey = k.structural, structKey
			case MatchModePlain:
				storedKey, lookupKey = k.plain, plainKey
			}
			if storedKey == lookupKey {
				seen[se.entry.ID] = true
				var adaptations []EntityAdaptation
				if mode.mt == MatchGeneralizedExact {
					adaptations = ComputeEntityAdaptations(se.entry, sourceLocale, targetLocale, entityAnnotations)
				}
				matches = append(matches, TMMatch{
					Entry:             se.entry,
					Score:             1.0,
					MatchType:         mode.mt,
					ProjectID:         se.entry.ProjectID,
					EntityAdaptations: adaptations,
				})
			}
		}
	}

	if len(matches) > 0 && opts.MinScore >= 1.0 {
		return LimitResults(matches, opts.MaxResults)
	}

	// Tier 4-6: fuzzy.
	for i := range tm.entries {
		se := &tm.entries[i]
		if seen[se.entry.ID] {
			continue
		}
		k, ok := eligible(se)
		if !ok {
			continue
		}
		var bestScore float64
		var bestType MatchType
		if modeEnabled[MatchModeGeneralized] {
			s := LevenshteinRatio(generalKey, k.generalized)
			if s >= opts.MinScore && s > bestScore {
				bestScore = s
				bestType = MatchGeneralizedFuzzy
			}
		}
		if modeEnabled[MatchModeStructural] {
			s := LevenshteinRatio(structKey, k.structural)
			if s >= opts.MinScore && s > bestScore {
				bestScore = s
				bestType = MatchStructuralFuzzy
			}
		}
		if modeEnabled[MatchModePlain] {
			s := LevenshteinRatio(plainKey, k.plain)
			if s >= opts.MinScore && s > bestScore {
				bestScore = s
				bestType = MatchFuzzy
			}
		}
		if bestScore < opts.MinScore {
			continue
		}
		if opts.ProjectID != "" && se.entry.ProjectID == opts.ProjectID && bestScore < 1.0 {
			bestScore += 0.03
			if bestScore > 1.0 {
				bestScore = 1.0
			}
		}
		seen[se.entry.ID] = true
		var adaptations []EntityAdaptation
		if bestType == MatchGeneralizedFuzzy {
			adaptations = ComputeEntityAdaptations(se.entry, sourceLocale, targetLocale, entityAnnotations)
		}
		matches = append(matches, TMMatch{
			Entry:             se.entry,
			Score:             bestScore,
			MatchType:         bestType,
			ProjectID:         se.entry.ProjectID,
			EntityAdaptations: adaptations,
		})
	}

	slices.SortFunc(matches, func(a, b TMMatch) int {
		pa := MatchTypePriority(a.MatchType)
		pb := MatchTypePriority(b.MatchType)
		if c := cmp.Compare(pa, pb); c != 0 {
			return c
		}
		return cmp.Compare(b.Score, a.Score)
	})
	return LimitResults(matches, opts.MaxResults)
}

func projectAllowed(entryProject string, opts LookupOptions) bool {
	switch opts.ProjectScope {
	case ProjectScopeAll:
		return true
	case ProjectScopeOnly:
		return entryProject == opts.ProjectID
	case ProjectScopeExclude:
		return entryProject != opts.ProjectID
	}
	return true
}

// --- Retrieval ---

// GetEntry fetches a single entry by ID.
func (tm *InMemoryTM) GetEntry(_ context.Context, id string) (TMEntry, bool, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	idx, ok := tm.byID[id]
	if !ok {
		return TMEntry{}, false, nil
	}
	return tm.entries[idx].entry, true, nil
}

// Entries returns a snapshot of all entries.
func (tm *InMemoryTM) Entries(_ context.Context) ([]TMEntry, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	out := make([]TMEntry, len(tm.entries))
	for i, se := range tm.entries {
		out[i] = se.entry
	}
	return out, nil
}

// --- Search ---

// SearchEntries performs a case-insensitive substring search.
func (tm *InMemoryTM) SearchEntries(ctx context.Context, params SearchParams) ([]TMEntry, int, error) {
	return tm.searchInternal(ctx, params)
}

// SearchEntriesFiltered applies additional facet filters.
func (tm *InMemoryTM) SearchEntriesFiltered(ctx context.Context, params SearchParams) ([]TMEntry, int, error) {
	return tm.searchInternal(ctx, params)
}

// SearchEntriesForStream performs a search with stream priority ordering.
func (tm *InMemoryTM) SearchEntriesForStream(ctx context.Context, params SearchParams) ([]TMEntry, int, error) {
	return tm.searchInternal(ctx, params)
}

func (tm *InMemoryTM) searchInternal(_ context.Context, params SearchParams) ([]TMEntry, int, error) {
	query := params.Query
	anyLocale := params.AnyLocale
	requireLocale := params.RequireLocale
	filter := params.Filter
	offset := params.Offset
	limit := params.Limit

	tm.mu.RLock()
	defer tm.mu.RUnlock()

	lowerQuery := strings.ToLower(query)
	var matched []TMEntry
	for _, se := range tm.entries {
		e := se.entry
		if !matchesSearchFilter(e, filter) {
			continue
		}
		if requireLocale != "" && !e.HasLocale(model.LocaleID(requireLocale)) {
			continue
		}
		if query != "" {
			if !variantTextContains(e, anyLocale, lowerQuery) {
				continue
			}
		} else if anyLocale != "" && !e.HasLocale(model.LocaleID(anyLocale)) {
			continue
		}
		matched = append(matched, e)
	}
	// Sort by updated_at DESC for stable pagination.
	sort.SliceStable(matched, func(i, j int) bool {
		return matched[i].UpdatedAt.After(matched[j].UpdatedAt)
	})
	total := len(matched)
	if offset >= total {
		return nil, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return matched[offset:end], total, nil
}

// variantTextContains checks if any variant's plain text contains the
// (already lowercased) needle. When anyLocale is set, only that locale's
// variant is consulted.
func variantTextContains(e TMEntry, anyLocale, needle string) bool {
	if anyLocale != "" {
		runs := e.Variant(model.LocaleID(anyLocale))
		if runs == nil {
			return false
		}
		return strings.Contains(strings.ToLower(model.FlattenRuns(runs)), needle)
	}
	for _, runs := range e.Variants {
		if len(runs) == 0 {
			continue
		}
		if strings.Contains(strings.ToLower(model.FlattenRuns(runs)), needle) {
			return true
		}
	}
	return false
}

// matchesSearchFilter reports whether entry passes the facet filter.
func matchesSearchFilter(entry TMEntry, filter SearchFilter) bool {
	if filter.ProjectID != "" && entry.ProjectID != filter.ProjectID {
		return false
	}
	if len(filter.SessionIDs) > 0 {
		found := false
		for _, sid := range filter.SessionIDs {
			for _, o := range entry.Origins {
				if o.SessionID == sid {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			return false
		}
	}
	if len(filter.EntityTypes) > 0 {
		found := false
		for _, et := range filter.EntityTypes {
			for _, em := range entry.Entities {
				if string(em.Type) == et {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			return false
		}
	}
	if len(filter.EntityValues) > 0 {
		found := false
		for _, ev := range filter.EntityValues {
			for _, em := range entry.Entities {
				if string(em.Type) != ev.Type {
					continue
				}
				for _, val := range em.Values {
					if val.Text == ev.Value {
						found = true
						break
					}
				}
				if found {
					break
				}
			}
			if found {
				break
			}
		}
		if !found {
			return false
		}
	}
	if filter.HasCodes != nil {
		has := entryHasCodes(entry)
		if *filter.HasCodes != has {
			return false
		}
	}
	return true
}

func entryHasCodes(entry TMEntry) bool {
	for _, runs := range entry.Variants {
		for _, r := range runs {
			if r.Text == nil {
				return true
			}
		}
	}
	return false
}

// --- Facets & stats ---

// FacetStats returns aggregated facet data for filtering UI.
func (tm *InMemoryTM) FacetStats(ctx context.Context) (FacetData, error) {
	return tm.FacetStatsFiltered(ctx, SearchParams{})
}

// FacetStatsFiltered returns facet counts scoped to entries matching the
// given search query and filter.
func (tm *InMemoryTM) FacetStatsFiltered(_ context.Context, params SearchParams) (FacetData, error) {
	query := params.Query
	anyLocale := params.AnyLocale
	requireLocale := params.RequireLocale
	filter := params.Filter

	tm.mu.RLock()
	defer tm.mu.RUnlock()
	lowerQuery := strings.ToLower(query)

	localeCounts := make(map[string]int)
	projCounts := make(map[string]int)
	entityCounts := make(map[string]int)
	sessionCounts := make(map[string]int)
	sessionInfo := make(map[string]ImportSession)
	var hasCodes, noCodes int

	for _, se := range tm.entries {
		e := se.entry
		if !matchesSearchFilter(e, filter) {
			continue
		}
		if requireLocale != "" && !e.HasLocale(model.LocaleID(requireLocale)) {
			continue
		}
		if query != "" {
			if !variantTextContains(e, anyLocale, lowerQuery) {
				continue
			}
		} else if anyLocale != "" && !e.HasLocale(model.LocaleID(anyLocale)) {
			continue
		}

		for loc := range e.Variants {
			localeCounts[string(loc)]++
		}
		projCounts[e.ProjectID]++
		entityTypesCounted := make(map[string]bool)
		for _, em := range e.Entities {
			t := string(em.Type)
			if !entityTypesCounted[t] {
				entityTypesCounted[t] = true
				entityCounts[t]++
			}
		}
		sessionCounted := make(map[string]bool)
		for _, o := range e.Origins {
			if o.SessionID == "" || sessionCounted[o.SessionID] {
				continue
			}
			sessionCounted[o.SessionID] = true
			sessionCounts[o.SessionID]++
			if _, ok := sessionInfo[o.SessionID]; !ok {
				if s, ok := tm.sessions[o.SessionID]; ok {
					sessionInfo[o.SessionID] = s
				}
			}
		}
		if entryHasCodes(e) {
			hasCodes++
		} else {
			noCodes++
		}
	}

	data := FacetData{HasCodes: hasCodes, NoCodes: noCodes}
	for loc, count := range localeCounts {
		data.Locales = append(data.Locales, LocaleFacet{Locale: loc, Count: count})
	}
	sort.Slice(data.Locales, func(i, j int) bool {
		if data.Locales[i].Count != data.Locales[j].Count {
			return data.Locales[i].Count > data.Locales[j].Count
		}
		return data.Locales[i].Locale < data.Locales[j].Locale
	})
	for pid, count := range projCounts {
		data.Projects = append(data.Projects, ProjectFacet{ProjectID: pid, Count: count})
	}
	sort.Slice(data.Projects, func(i, j int) bool { return data.Projects[i].Count > data.Projects[j].Count })
	for et, count := range entityCounts {
		data.EntityTypes = append(data.EntityTypes, EntityTypeFacet{Type: et, Count: count})
	}
	sort.Slice(data.EntityTypes, func(i, j int) bool { return data.EntityTypes[i].Count > data.EntityTypes[j].Count })
	for sid, count := range sessionCounts {
		info := sessionInfo[sid]
		data.ImportSessions = append(data.ImportSessions, ImportSessionFacet{
			SessionID:  sid,
			FileKey:    info.FileKey,
			ToolName:   info.ToolName,
			ImportedAt: info.ImportedAt,
			Count:      count,
		})
	}
	sort.Slice(data.ImportSessions, func(i, j int) bool {
		return data.ImportSessions[i].Count > data.ImportSessions[j].Count
	})
	return data, nil
}

// LocaleStats returns per-locale entry counts across the full TM.
func (tm *InMemoryTM) LocaleStats(_ context.Context) ([]LocaleFacet, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	counts := make(map[string]int)
	for _, se := range tm.entries {
		for loc := range se.entry.Variants {
			counts[string(loc)]++
		}
	}
	out := make([]LocaleFacet, 0, len(counts))
	for loc, count := range counts {
		out = append(out, LocaleFacet{Locale: loc, Count: count})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Locale < out[j].Locale
	})
	return out, nil
}

// ActivityStats returns daily entry counts over time based on CreatedAt.
func (tm *InMemoryTM) ActivityStats(_ context.Context) ([]ActivityStat, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	counts := make(map[string]int)
	for _, se := range tm.entries {
		day := se.entry.CreatedAt.Format("2006-01-02")
		counts[day]++
	}
	out := make([]ActivityStat, 0, len(counts))
	for d, c := range counts {
		out = append(out, ActivityStat{Date: d, Count: c})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out, nil
}

// --- Import session CRUD ---

// CreateImportSession inserts a new session.
func (tm *InMemoryTM) CreateImportSession(_ context.Context, session ImportSession) error {
	if session.ID == "" {
		return ErrSessionIDRequired
	}
	if session.FileKey == "" {
		return ErrSessionFileKey
	}
	if session.ImportedAt.IsZero() {
		session.ImportedAt = time.Now()
	}
	tm.mu.Lock()
	defer tm.mu.Unlock()
	copySession := session
	if session.Properties != nil {
		copySession.Properties = make(map[string]string, len(session.Properties))
		for k, v := range session.Properties {
			copySession.Properties[k] = v
		}
	}
	tm.sessions[session.ID] = copySession
	return nil
}

// GetImportSession fetches a session by ID.
func (tm *InMemoryTM) GetImportSession(_ context.Context, id string) (ImportSession, bool, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	s, ok := tm.sessions[id]
	return s, ok, nil
}

// FindImportSessionByHash returns the most recent session matching the hash.
func (tm *InMemoryTM) FindImportSessionByHash(_ context.Context, hash string) (ImportSession, bool, error) {
	if hash == "" {
		return ImportSession{}, false, nil
	}
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	var best *ImportSession
	for _, s := range tm.sessions {
		if s.FileHash != hash {
			continue
		}
		if best == nil || s.ImportedAt.After(best.ImportedAt) {
			best = &s
		}
	}
	if best == nil {
		return ImportSession{}, false, nil
	}
	return *best, true, nil
}

// ListImportSessions returns all sessions ordered by imported_at DESC.
func (tm *InMemoryTM) ListImportSessions(_ context.Context) ([]ImportSession, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	out := make([]ImportSession, 0, len(tm.sessions))
	for _, s := range tm.sessions {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ImportedAt.After(out[j].ImportedAt) })
	return out, nil
}

// UpdateImportSessionCount sets the entry_count on a session.
func (tm *InMemoryTM) UpdateImportSessionCount(_ context.Context, id string, count int) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	s, ok := tm.sessions[id]
	if !ok {
		return ErrImportSessionMiss
	}
	s.EntryCount = count
	tm.sessions[id] = s
	return nil
}

// DeleteImportSession removes a session; any origins referencing it have
// their session_id cleared.
func (tm *InMemoryTM) DeleteImportSession(_ context.Context, id string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	if _, ok := tm.sessions[id]; !ok {
		return ErrImportSessionMiss
	}
	delete(tm.sessions, id)
	for i := range tm.entries {
		for j := range tm.entries[i].entry.Origins {
			if tm.entries[i].entry.Origins[j].SessionID == id {
				tm.entries[i].entry.Origins[j].SessionID = ""
			}
		}
	}
	return nil
}

// --- helpers ---

// ApplyDefaults fills in sensible defaults for LookupOptions.
func ApplyDefaults(opts LookupOptions) LookupOptions {
	if opts.MinScore <= 0 {
		opts.MinScore = 0.7
	}
	if opts.MaxResults <= 0 {
		opts.MaxResults = 10
	}
	return opts
}

// MatchModesEnabled returns a set of enabled match modes; empty means all.
func MatchModesEnabled(modes []MatchMode) map[MatchMode]bool {
	if len(modes) == 0 {
		return map[MatchMode]bool{
			MatchModeGeneralized: true,
			MatchModeStructural:  true,
			MatchModePlain:       true,
		}
	}
	m := make(map[MatchMode]bool)
	for _, mode := range modes {
		m[mode] = true
	}
	return m
}

// MatchTypePriority returns the sort priority (lower = better) for a match type.
func MatchTypePriority(mt MatchType) int {
	switch mt {
	case MatchGeneralizedExact:
		return 0
	case MatchStructuralExact:
		return 1
	case MatchExact:
		return 2
	case MatchGeneralizedFuzzy:
		return 3
	case MatchStructuralFuzzy:
		return 4
	case MatchFuzzy:
		return 5
	default:
		return 6
	}
}

// LimitResults truncates a match list to max entries.
func LimitResults(matches []TMMatch, max int) []TMMatch {
	if len(matches) > max {
		return matches[:max]
	}
	return matches
}

// ExtractEntityAnnotations pulls EntityAnnotation instances from a Block's
// annotations map.
func ExtractEntityAnnotations(block *model.Block) []*model.EntityAnnotation {
	if block == nil || block.Annotations == nil {
		return nil
	}
	var entities []*model.EntityAnnotation
	for _, ann := range block.Annotations {
		if ea, ok := ann.(*model.EntityAnnotation); ok {
			entities = append(entities, ea)
		}
	}
	return entities
}

// ComputeEntityAdaptations computes how to adapt entity values from a stored
// TM entry's target variant to match the current source content.
// sourceLocale is the locale of currentEntities; targetLocale is the variant
// whose entity values should be rewritten.
func ComputeEntityAdaptations(entry TMEntry, sourceLocale, targetLocale model.LocaleID, currentEntities []*model.EntityAnnotation) []EntityAdaptation {
	if len(entry.Entities) == 0 || len(currentEntities) == 0 {
		return nil
	}
	typeQueues := make(map[model.EntityType][]*model.EntityAnnotation)
	for _, ea := range currentEntities {
		typeQueues[ea.Type] = append(typeQueues[ea.Type], ea)
	}
	typeIdx := make(map[model.EntityType]int)
	var adaptations []EntityAdaptation
	for _, em := range entry.Entities {
		queue := typeQueues[em.Type]
		idx := typeIdx[em.Type]
		if idx >= len(queue) {
			continue
		}
		current := queue[idx]
		typeIdx[em.Type] = idx + 1
		tv, ok := em.Values[targetLocale]
		if !ok {
			continue
		}
		if tv.Text == current.Text {
			continue
		}
		adaptations = append(adaptations, EntityAdaptation{
			PlaceholderID: em.PlaceholderID,
			Type:          em.Type,
			StoredValue:   tv.Text,
			CurrentValue:  current.Text,
			TargetPos:     model.TextRange{Start: tv.Start, End: tv.End},
		})
	}
	return adaptations
}

// NormalizeText normalizes text for comparison by applying Unicode NFC
// normalization, trimming whitespace, and collapsing internal whitespace
// to single spaces.
func NormalizeText(s string) string {
	s = norm.NFC.String(s)
	if !needsWhitespaceNormalization(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	inSpace := true
	for _, r := range s {
		if isWhitespace(r) {
			inSpace = true
		} else {
			if inSpace && b.Len() > 0 {
				b.WriteByte(' ')
			}
			inSpace = false
			b.WriteRune(r)
		}
	}
	return b.String()
}

func needsWhitespaceNormalization(s string) bool {
	if len(s) == 0 {
		return false
	}
	if isWhitespace(rune(s[0])) || isWhitespace(rune(s[len(s)-1])) {
		return true
	}
	prev := false
	for _, r := range s {
		ws := isWhitespace(r)
		if ws && prev {
			return true
		}
		if ws && r != ' ' {
			return true
		}
		prev = ws
	}
	return false
}

func isWhitespace(r rune) bool {
	switch r {
	case ' ', '\t', '\n', '\r', '\v', '\f':
		return true
	case '\u0085', '\u00A0', '\u1680',
		'\u2000', '\u2001', '\u2002', '\u2003', '\u2004', '\u2005', '\u2006', '\u2007', '\u2008', '\u2009', '\u200A',
		'\u2028', '\u2029', '\u202F', '\u205F', '\u3000':
		return true
	}
	return false
}
