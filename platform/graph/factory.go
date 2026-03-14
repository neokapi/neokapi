package graph

import "github.com/jackc/pgx/v5/pgxpool"

// NewGraphStore creates a new AGE-backed GraphStore.
func NewGraphStore(pool *pgxpool.Pool) *AGEGraphStore {
	return NewAGEGraphStore(pool)
}
