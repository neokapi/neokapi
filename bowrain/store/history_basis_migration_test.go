package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/core/venue"
)

// historyBasisV43 is the body version 43 carried before the change was lost
// from main. Production databases applied it and recorded 43; version 46
// re-issues it.
const historyBasisV43 = `
	ALTER TABLE block_history ADD COLUMN IF NOT EXISTS basis      TEXT NOT NULL DEFAULT '';
	ALTER TABLE block_history ADD COLUMN IF NOT EXISTS basis_from TEXT NOT NULL DEFAULT '';
	DELETE FROM unit_decisions
	 WHERE review_state = '' AND status = '' AND decided_by = ''
	   AND note = '' AND assignee = '' AND NOT parked;
`

// Version 46 adds the block history basis to a database that never had it.
// On a database that applied version 43 it keeps the columns and removes the
// basis-only ledger rows written since. Either way the database ends with one
// basis and one basis_from column and no basis-only rows.
func TestMigrations_HistoryBasisReissuedAs46(t *testing.T) {
	for _, tc := range []struct {
		name    string
		prepare []string
	}{
		{
			name: "never applied 43",
			prepare: []string{
				`ALTER TABLE block_history DROP COLUMN basis`,
				`ALTER TABLE block_history DROP COLUMN basis_from`,
			},
		},
		{
			name: "applied 43",
			prepare: []string{
				`ALTER TABLE block_history DROP COLUMN basis`,
				`ALTER TABLE block_history DROP COLUMN basis_from`,
				historyBasisV43,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestStore(t)
			ctx := t.Context()
			p := createTestProject(t, s)

			_, err := s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{{
				ItemName: "en.json", Unit: "decided", Variant: "nb",
				ReviewState: venue.ReviewStateApproved, DecidedBy: "reviewer@example.com",
				Updated: "2026-09-01T10:00:00Z",
			}})
			require.NoError(t, err)

			for _, stmt := range tc.prepare {
				_, err := s.db.ExecContext(ctx, stmt)
				require.NoError(t, err, stmt)
			}
			// A basis-only ledger row, of the kind a push wrote while the
			// change was missing from main.
			_, err = s.db.ExecContext(ctx,
				`INSERT INTO unit_decisions (project_id, stream, item_name, unit, variant, draft_basis)
				 VALUES ($1, 'main', 'en.json', 'basis-only', 'nb', 'rev-1')`, p.ID)
			require.NoError(t, err)
			_, err = s.db.ExecContext(ctx, `DELETE FROM store_schema_migrations WHERE version >= 46`)
			require.NoError(t, err)

			require.NoError(t, storage.MigratePostgresNS(s.db, "store_schema_migrations", Migrations))

			cols := queryStrings(t, s,
				`SELECT column_name FROM information_schema.columns
				  WHERE table_schema = current_schema() AND table_name = 'block_history'
				    AND column_name IN ('basis', 'basis_from') ORDER BY column_name`)
			assert.Equal(t, []string{"basis", "basis_from"}, cols)

			units := queryStrings(t, s,
				`SELECT unit FROM unit_decisions WHERE project_id = $1 ORDER BY unit`, p.ID)
			assert.Equal(t, []string{"decided"}, units, "the basis-only row goes; the decision stays")

			var applied int
			require.NoError(t, s.db.QueryRowContext(ctx,
				`SELECT count(*) FROM store_schema_migrations WHERE version = 46`).Scan(&applied))
			assert.Equal(t, 1, applied)

			// A second pass finds nothing to do.
			require.NoError(t, storage.MigratePostgresNS(s.db, "store_schema_migrations", Migrations))
		})
	}
}

func queryStrings(t *testing.T, s *PostgresStore, query string, args ...any) []string {
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
