package termbase

import fw "github.com/gokapi/gokapi/core/termbase"

// TBStore extends TermBase with stream-aware methods needed by the
// bowrain server (REST + gRPC handlers).
type TBStore interface {
	fw.TermBase

	// AddConceptWithStream inserts or updates a concept associated with a stream.
	// The stream is a persistence concern (e.g., a git branch name).
	AddConceptWithStream(concept fw.Concept, stream string) error

	// SearchForStream performs a case-insensitive text search with stream inheritance.
	// The streamChain is the ordered list of ancestor streams to search
	// (e.g., ["feature/rebrand", "main", ""]). Concepts from earlier streams
	// in the chain take priority.
	SearchForStream(query, sourceLocale, targetLocale, stream string, streamChain []string, offset, limit int) ([]fw.Concept, int)
}
