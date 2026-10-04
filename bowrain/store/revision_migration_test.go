package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// A database at version 40 grades its decisions by text hash. Version 41
// drops the hashes, clears the draft marks and the settlement stamps that name
// one, and leaves every stored block without a source revision until its
// source is written again. A decision recorded before revisions then names no
// basis: it reads back with its verdict, is counted as unknown, and stops
// projecting its rung the next time its source is written.
func TestMigrations_ADatabaseGradedByHashLosesThePairing(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	b := blockWithTarget("greeting", "Hello", "Hei", model.TargetStatusEstablished)
	b.Properties = map[string]string{"context": "homepage"}
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{b}))
	_, err := s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{{
		ItemName: "en.json", Unit: "greeting", Variant: "nb",
		Status: string(model.TargetStatusEstablished), ReviewState: venue.ReviewStateApproved,
		DecidedBy: "reviewer@example.com", Updated: "2026-09-01T10:00:00Z",
	}})
	require.NoError(t, err)

	// What a database at version 40 holds: the hash pairing on every record,
	// a draft mark and a settlement stamp naming a text hash, and blocks with
	// no source revision.
	for _, stmt := range []string{
		`ALTER TABLE unit_decisions ADD COLUMN target_hash TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE unit_decisions ADD COLUMN content_hash TEXT NOT NULL DEFAULT ''`,
		`UPDATE unit_decisions SET target_hash = 't-hash', content_hash = 's-hash', draft_basis = 's-hash'`,
		`UPDATE blocks SET properties = '{"__source_settled_hash":"s-hash","context":"homepage"}'`,
		`ALTER TABLE blocks DROP COLUMN source_revision`,
		`DELETE FROM store_schema_migrations WHERE version >= 41`,
	} {
		_, err := s.db.ExecContext(ctx, stmt)
		require.NoError(t, err, stmt)
	}
	require.NoError(t, storage.MigratePostgresNS(s.db, "store_schema_migrations", Migrations))

	hasColumn := func(table, column string) bool {
		var n int
		require.NoError(t, s.db.QueryRowContext(ctx,
			`SELECT count(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = $1 AND column_name = $2`,
			table, column).Scan(&n))
		return n > 0
	}
	assert.False(t, hasColumn("unit_decisions", "target_hash"))
	assert.False(t, hasColumn("unit_decisions", "content_hash"))

	got, err := s.GetUnitDecision(ctx, p.ID, "main", "en.json", "greeting", "nb")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, venue.ReviewStateApproved, got.ReviewState, "the verdict reads back")
	assert.Empty(t, got.Basis, "it names no source")
	drafts, err := s.ListDraftBases(ctx, p.ID, "main")
	require.NoError(t, err)
	assert.Empty(t, drafts, "a mark that named a text hash is cleared")

	rows, err := s.GetBlocks(ctx, platstore.BlockQuery{ProjectID: p.ID, Stream: "main", ItemName: "en.json"})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Empty(t, rows[0].SourceRevision, "no source revision until the source is written again")
	assert.NotContains(t, rows[0].Block.Properties, "__source_settled_hash", "the settlement stamp that named a text hash is cleared")
	assert.Equal(t, "homepage", rows[0].Block.Properties["context"], "every other property stays")

	tallies, err := s.TallyDecisionBasis(ctx, p.ID, "main")
	require.NoError(t, err)
	require.Len(t, tallies, 1)
	assert.Equal(t, 1, tallies[0].BasisUnknown, "a record with no basis is counted as unknown")
	assert.Zero(t, tallies[0].Stale)
	require.Equal(t, model.TargetStatusEstablished, targetStatus(t, s, p.ID, "en.json", "greeting"))

	// The source is written again, as the next push or settle pass writes it.
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{blockWithText("greeting", "Hello")}))
	rows, err = s.GetBlocks(ctx, platstore.BlockQuery{ProjectID: p.ID, Stream: "main", ItemName: "en.json"})
	require.NoError(t, err)
	assert.Equal(t, sourceRevision("Hello"), rows[0].SourceRevision, "the write stamps the revision")
	assert.Equal(t, model.TargetStatusTranslated, targetStatus(t, s, p.ID, "en.json", "greeting"),
		"an approval that names no source stops projecting its rung")
}
