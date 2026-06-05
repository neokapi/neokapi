package sievepen

import "context"

// TMStore extends TranslationMemory with search, stream, import-session,
// and export methods needed by persistent backends (SQLite and PostgreSQL).
//
// The search and facet methods take a SearchParams struct so callers can't
// transpose the query/locale/stream arguments. See SearchParams for the
// per-field semantics.
type TMStore interface {
	TranslationMemory

	// AddWithStream inserts or updates a TM entry associated with a stream.
	// The stream is a persistence concern (e.g., a git branch name).
	AddWithStream(ctx context.Context, entry TMEntry, stream string) error

	// SearchEntries performs a text search across variant text, returning
	// full multilingual entries. params.Stream/StreamChain are ignored.
	SearchEntries(ctx context.Context, params SearchParams) ([]TMEntry, int, error)

	// SearchEntriesFiltered performs a text search with additional facet
	// filters applied (params.Filter). params.Stream/StreamChain are ignored.
	SearchEntriesFiltered(ctx context.Context, params SearchParams) ([]TMEntry, int, error)

	// SearchEntriesForStream performs a text search with stream
	// inheritance. params.StreamChain is the ordered list of ancestor
	// streams to search (e.g., ["feature/rebrand", "main", ""]); entries
	// from earlier streams take priority.
	SearchEntriesForStream(ctx context.Context, params SearchParams) ([]TMEntry, int, error)

	// GetEntry fetches a single entry by ID with all its variants populated.
	GetEntry(ctx context.Context, id string) (TMEntry, bool, error)

	// Entries returns all entries (for export).
	Entries(ctx context.Context) ([]TMEntry, error)

	// FacetStats returns aggregated facet data across the full TM.
	FacetStats(ctx context.Context) (FacetData, error)

	// FacetStatsFiltered returns facet counts scoped to entries matching
	// the given search query and filter (params.Filter). This lets the UI
	// reflect faceted counts for the current result set.
	FacetStatsFiltered(ctx context.Context, params SearchParams) (FacetData, error)

	// LocaleStats returns the number of entries having a variant for each
	// locale, ordered by count descending. An entry with N variants
	// contributes to N LocaleFacet counts.
	LocaleStats(ctx context.Context) ([]LocaleFacet, error)

	// ActivityStats returns daily entry counts over time based on created_at.
	ActivityStats(ctx context.Context) ([]ActivityStat, error)

	// --- Import sessions ---

	// CreateImportSession inserts a new import session row.
	CreateImportSession(ctx context.Context, session ImportSession) error

	// GetImportSession fetches a session by ID.
	GetImportSession(ctx context.Context, id string) (ImportSession, bool, error)

	// FindImportSessionByHash returns the most recent session matching
	// the given file hash (for re-import dedup warnings).
	FindImportSessionByHash(ctx context.Context, hash string) (ImportSession, bool, error)

	// ListImportSessions returns all sessions ordered by imported_at DESC.
	ListImportSessions(ctx context.Context) ([]ImportSession, error)

	// UpdateImportSessionCount sets the entry_count on a session after
	// the import loop completes.
	UpdateImportSessionCount(ctx context.Context, id string, count int) error

	// DeleteImportSession removes a session row. Origins referencing it
	// have their session_id cleared to empty via the FK's ON DELETE
	// SET NULL behavior — the entries themselves are not affected.
	DeleteImportSession(ctx context.Context, id string) error
}

// BulkAdder is an optional capability implemented by backends that support
// batched inserts. The TMX importer detects it and commits an entire file
// in a single transaction, which is the difference between reasonable and
// unreasonable import times on large corpora (EUR-Lex, bitextor).
type BulkAdder interface {
	BulkAddWithStream(ctx context.Context, entries []TMEntry, stream string) error
}
