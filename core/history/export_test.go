package history

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// ReachedPlan returns the steps of the plan SQLite chooses for Reached.
func ReachedPlan(t *testing.T, s *Store) []string {
	t.Helper()
	return plan(t, s, reachedQuery, `[{"b":"p","e":"en","r":"r:1"}]`, "d-1")
}

// LatestPlan returns the steps of the plan SQLite chooses for Latest of one
// document's editions.
func LatestPlan(t *testing.T, s *Store, editions ...string) []string {
	t.Helper()
	args := []any{"d-1"}
	for _, e := range editions {
		args = append(args, e)
	}
	return plan(t, s, latestQuery(len(editions)), args...)
}

func plan(t *testing.T, s *Store, query string, args ...any) []string {
	t.Helper()
	rows, err := s.db.QueryContext(t.Context(), `EXPLAIN QUERY PLAN `+query, args...)
	require.NoError(t, err)
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var id, parent, unused int
		var detail string
		require.NoError(t, rows.Scan(&id, &parent, &unused, &detail))
		out = append(out, detail)
	}
	require.NoError(t, rows.Err())
	return out
}
