package termbase

import (
	"context"

	"github.com/neokapi/neokapi/core/model"
)

// TBStore extends TermBase with stream-aware methods needed by
// persistent backends (SQLite and PostgreSQL).
type TBStore interface {
	TermBase

	// AddConceptWithStream inserts or updates a concept associated with a stream.
	// The stream is a persistence concern (e.g., a git branch name).
	AddConceptWithStream(ctx context.Context, concept Concept, stream string) error

	// SearchForStream performs a case-insensitive text search with stream inheritance.
	// The streamChain is the ordered list of ancestor streams to search
	// (e.g., ["feature/rebrand", "main", ""]). Concepts from earlier streams
	// in the chain take priority.
	SearchForStream(ctx context.Context, query string, sourceLocale, targetLocale model.LocaleID, stream string, streamChain []string, offset, limit int) ([]Concept, int, error)
}
