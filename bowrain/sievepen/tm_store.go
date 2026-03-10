package sievepen

import fw "github.com/gokapi/gokapi/core/sievepen"

// TMStore extends TranslationMemory with search and export methods
// needed by the bowrain server (REST + gRPC handlers).
type TMStore interface {
	fw.TranslationMemory

	// AddWithStream inserts or updates a TM entry associated with a stream.
	// The stream is a persistence concern (e.g., a git branch name).
	AddWithStream(entry fw.TMEntry, stream string) error

	// SearchEntries performs a case-insensitive substring search on source/target text.
	SearchEntries(query, sourceLocale, targetLocale string, offset, limit int) ([]fw.TMEntry, int)

	// SearchEntriesForStream performs a case-insensitive substring search with stream
	// inheritance. The streamChain is the ordered list of ancestor streams to search
	// (e.g., ["feature/rebrand", "main", ""]). Entries from earlier streams in the
	// chain take priority.
	SearchEntriesForStream(query, sourceLocale, targetLocale, stream string, streamChain []string, offset, limit int) ([]fw.TMEntry, int)

	// GetEntry fetches a single entry by ID.
	GetEntry(id string) (fw.TMEntry, bool)

	// Entries returns all entries (for export).
	Entries() []fw.TMEntry
}
