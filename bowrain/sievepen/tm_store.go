package sievepen

import fw "github.com/gokapi/gokapi/core/sievepen"

// TMStore extends TranslationMemory with search and export methods
// needed by the bowrain server (REST + gRPC handlers).
type TMStore interface {
	fw.TranslationMemory

	// SearchEntries performs a case-insensitive substring search on source/target text.
	SearchEntries(query, sourceLocale, targetLocale string, offset, limit int) ([]fw.TMEntry, int)

	// GetEntry fetches a single entry by ID.
	GetEntry(id string) (fw.TMEntry, bool)

	// Entries returns all entries (for export).
	Entries() []fw.TMEntry
}
