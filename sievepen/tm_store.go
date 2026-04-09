package sievepen

// TMStore extends TranslationMemory with search, stream, and export methods
// needed by persistent backends (SQLite and PostgreSQL).
type TMStore interface {
	TranslationMemory

	// AddWithStream inserts or updates a TM entry associated with a stream.
	// The stream is a persistence concern (e.g., a git branch name).
	AddWithStream(entry TMEntry, stream string) error

	// SearchEntries performs a case-insensitive substring search on source/target text.
	SearchEntries(query, sourceLocale, targetLocale string, offset, limit int) ([]TMEntry, int)

	// SearchEntriesForStream performs a case-insensitive substring search with stream
	// inheritance. The streamChain is the ordered list of ancestor streams to search
	// (e.g., ["feature/rebrand", "main", ""]). Entries from earlier streams in the
	// chain take priority.
	SearchEntriesForStream(query, sourceLocale, targetLocale, stream string, streamChain []string, offset, limit int) ([]TMEntry, int)

	// GetEntry fetches a single entry by ID.
	GetEntry(id string) (TMEntry, bool)

	// Entries returns all entries (for export).
	Entries() []TMEntry

	// SearchEntriesGrouped returns entries grouped by source text.
	SearchEntriesGrouped(query, sourceLocale string, offset, limit int) ([]TMEntryGroup, int)

	// SearchEntriesFiltered performs a search with additional facet filters.
	SearchEntriesFiltered(query, sourceLocale, targetLocale string, filter SearchFilter, offset, limit int) ([]TMEntry, int)

	// SearchEntriesGroupedFiltered returns entries grouped by source text with facet filters.
	SearchEntriesGroupedFiltered(query, sourceLocale string, filter SearchFilter, offset, limit int) ([]TMEntryGroup, int)

	// FacetStats returns aggregated facet data for filtering UI.
	FacetStats() FacetData
}
