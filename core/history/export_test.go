package history

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// ReachedPlan returns the steps of the plan SQLite chooses for Reached.
func ReachedPlan(t *testing.T, s *Store) []string {
	t.Helper()
	rows, err := s.db.QueryContext(t.Context(), `EXPLAIN QUERY PLAN `+reachedQuery, `[{"b":"p","e":"en","r":"r:1"}]`, "d-1")
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
