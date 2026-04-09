package sievepen

// TMStore extends TranslationMemory with search, stream, import-session,
// and export methods needed by persistent backends (SQLite and PostgreSQL).
//
// The search methods use the parameters:
//   - query:        text search query (empty = no text filter)
//   - anyLocale:    require matches in this locale's variant (empty = any locale)
//   - requireLocale: additionally require the entry to have this locale variant
//     (empty = no additional requirement)
//
// Callers doing bilingual browsing typically pass `(srcLocale, tgtLocale)`;
// monolingual browsing passes `(locale, "")`; fully open search passes `("", "")`.
type TMStore interface {
	TranslationMemory

	// AddWithStream inserts or updates a TM entry associated with a stream.
	// The stream is a persistence concern (e.g., a git branch name).
	AddWithStream(entry TMEntry, stream string) error

	// SearchEntries performs a text search across variant text, returning
	// full multilingual entries.
	SearchEntries(query, anyLocale, requireLocale string, offset, limit int) ([]TMEntry, int)

	// SearchEntriesFiltered performs a text search with additional facet
	// filters applied.
	SearchEntriesFiltered(query, anyLocale, requireLocale string, filter SearchFilter, offset, limit int) ([]TMEntry, int)

	// SearchEntriesForStream performs a text search with stream
	// inheritance. streamChain is the ordered list of ancestor streams
	// to search (e.g., ["feature/rebrand", "main", ""]); entries from
	// earlier streams take priority.
	SearchEntriesForStream(query, anyLocale, requireLocale, stream string, streamChain []string, offset, limit int) ([]TMEntry, int)

	// GetEntry fetches a single entry by ID with all its variants populated.
	GetEntry(id string) (TMEntry, bool)

	// Entries returns all entries (for export).
	Entries() []TMEntry

	// FacetStats returns aggregated facet data across the full TM.
	FacetStats() FacetData

	// FacetStatsFiltered returns facet counts scoped to entries matching
	// the given search query and filter. This lets the UI reflect faceted
	// counts for the current result set.
	FacetStatsFiltered(query, anyLocale, requireLocale string, filter SearchFilter) FacetData

	// LocaleStats returns the number of entries having a variant for each
	// locale, ordered by count descending. An entry with N variants
	// contributes to N LocaleFacet counts.
	LocaleStats() []LocaleFacet

	// ActivityStats returns daily entry counts over time based on created_at.
	ActivityStats() []ActivityStat

	// --- Import sessions ---

	// CreateImportSession inserts a new import session row.
	CreateImportSession(session ImportSession) error

	// GetImportSession fetches a session by ID.
	GetImportSession(id string) (ImportSession, bool)

	// FindImportSessionByHash returns the most recent session matching
	// the given file hash (for re-import dedup warnings).
	FindImportSessionByHash(hash string) (ImportSession, bool)

	// ListImportSessions returns all sessions ordered by imported_at DESC.
	ListImportSessions() []ImportSession

	// UpdateImportSessionCount sets the entry_count on a session after
	// the import loop completes.
	UpdateImportSessionCount(id string, count int) error

	// DeleteImportSession removes a session row. Origins referencing it
	// have their session_id cleared to empty via the FK's ON DELETE
	// SET NULL behavior — the entries themselves are not affected.
	DeleteImportSession(id string) error
}
