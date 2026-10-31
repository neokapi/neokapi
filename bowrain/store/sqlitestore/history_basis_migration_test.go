package sqlitestore

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// historyBasisV37 is the body version 37 carried before the change was lost
// from main. Desktop databases applied it and recorded 37; version 40
// re-issues it.
const historyBasisV37 = `
	ALTER TABLE block_history ADD COLUMN basis      TEXT NOT NULL DEFAULT '';
	ALTER TABLE block_history ADD COLUMN basis_from TEXT NOT NULL DEFAULT '';
	DELETE FROM unit_decisions
	 WHERE review_state = '' AND status = '' AND decided_by = ''
	   AND note = '' AND assignee = '' AND parked = 0;
`

// Version 40 adds the block history basis to a working copy that never had
// it. On one that applied version 37 it adds no column twice and removes the
// basis-only ledger rows written since. Either way the working copy ends with
// one basis and one basis_from column and no basis-only rows.
func TestMigrations_HistoryBasisReissuedAs40(t *testing.T) {
	for _, tc := range []struct {
		name     string
		applyV37 bool
	}{
		{name: "never applied 37"},
		{name: "applied 37", applyV37: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dbPath := filepath.Join(t.TempDir(), "store.db")
			s, err := NewSQLiteStore(dbPath)
			require.NoError(t, err)
			ctx := t.Context()

			for _, stmt := range []string{
				`ALTER TABLE block_history DROP COLUMN basis`,
				`ALTER TABLE block_history DROP COLUMN basis_from`,
				`INSERT INTO unit_decisions (project_id, stream, item_name, unit, variant, review_state, decided_by)
				 VALUES ('p1', 'main', 'en.json', 'decided', 'nb', 'approved', 'reviewer@example.com')`,
			} {
				_, err := s.db.ExecContext(ctx, stmt)
				require.NoError(t, err, stmt)
			}
			if tc.applyV37 {
				_, err := s.db.ExecContext(ctx, historyBasisV37)
				require.NoError(t, err)
			}
			// A basis-only ledger row, of the kind a push wrote while the
			// change was missing from main.
			_, err = s.db.ExecContext(ctx,
				`INSERT INTO unit_decisions (project_id, stream, item_name, unit, variant, draft_basis)
				 VALUES ('p1', 'main', 'en.json', 'basis-only', 'nb', 'rev-1')`)
			require.NoError(t, err)
			_, err = s.db.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version >= 40`)
			require.NoError(t, err)
			require.NoError(t, s.Close())

			reopened, err := NewSQLiteStore(dbPath)
			require.NoError(t, err)
			defer reopened.Close()

			cols := queryStrings(t, reopened,
				`SELECT name FROM pragma_table_info('block_history')
				  WHERE name IN ('basis', 'basis_from') ORDER BY name`)
			assert.Equal(t, []string{"basis", "basis_from"}, cols)

			units := queryStrings(t, reopened,
				`SELECT unit FROM unit_decisions WHERE project_id = 'p1' ORDER BY unit`)
			assert.Equal(t, []string{"decided"}, units, "the basis-only row goes; the decision stays")

			var applied int
			require.NoError(t, reopened.db.QueryRowContext(ctx,
				`SELECT count(*) FROM schema_migrations WHERE version = 40`).Scan(&applied))
			assert.Equal(t, 1, applied)
		})
	}
}

func queryStrings(t *testing.T, s *SQLiteStore, query string, args ...any) []string {
	t.Helper()
	rows, err := s.db.QueryContext(t.Context(), query, args...)
	require.NoError(t, err)
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		require.NoError(t, rows.Scan(&v))
		out = append(out, v)
	}
	require.NoError(t, rows.Err())
	return out
}
