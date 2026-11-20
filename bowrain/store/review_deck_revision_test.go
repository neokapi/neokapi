package store_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/model"
)

// The deck revision is what the server's deck summaries are kept under, so
// it has to move on every committed change a deck is counted from and stand
// still otherwise.
func TestReviewDeckRevision_MovesOnCommittedChanges(t *testing.T) {
	db := pgtest.NewTestDB(t)
	cs, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	ctx := t.Context()
	for _, id := range []string{"p1", "p2"} {
		require.NoError(t, cs.CreateProject(ctx, &platstore.Project{ID: id, Name: id, DefaultSourceLanguage: "en"}))
	}
	rev := func(project string) int64 {
		t.Helper()
		n, err := cs.ReviewDeckRevision(ctx, project)
		require.NoError(t, err)
		return n
	}
	b := &model.Block{ID: "b1", Translatable: true}
	b.SetSourceText("Hello")
	b.SetTargetText("fr", "Bonjour")
	require.NoError(t, cs.StoreBlocksForItem(ctx, "p1", "main", "greetings.txt", []*model.Block{b}))

	start, other := rev("p1"), rev("p2")
	assert.Equal(t, start, rev("p1"), "a read moves nothing")

	_, err = db.DB.ExecContext(ctx, `UPDATE translations SET text = 'Salut' WHERE project_id = 'p1'`)
	require.NoError(t, err)
	afterEdit := rev("p1")
	assert.Greater(t, afterEdit, start, "a translation edit moves the revision")
	assert.Equal(t, other, rev("p2"), "another project's revision stands")

	// A transaction moves the revision once it commits, and not before.
	tx, err := db.DB.BeginTx(ctx, nil)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `DELETE FROM translations WHERE project_id = 'p1'`)
	require.NoError(t, err)
	_, err = tx.ExecContext(ctx, `UPDATE blocks SET position = position + 1 WHERE project_id = 'p1'`)
	require.NoError(t, err)
	assert.Equal(t, afterEdit, rev("p1"), "an open transaction has moved nothing")
	require.NoError(t, tx.Commit())
	afterCommit := rev("p1")
	assert.Greater(t, afterCommit, afterEdit)

	// Folding the change rows into the base leaves the revision as it was.
	for range 600 {
		_, err = db.DB.ExecContext(ctx, `UPDATE blocks SET position = position WHERE project_id = 'p1'`)
		require.NoError(t, err)
	}
	full := rev("p1")
	assert.Equal(t, afterCommit+600, full)
	assert.Equal(t, full, rev("p1"), "compaction keeps the revision")
	var pending int
	require.NoError(t, db.DB.QueryRowContext(ctx, `SELECT count(*) FROM review_deck_changes WHERE project_id = 'p1'`).Scan(&pending))
	assert.Zero(t, pending, "the change rows were folded into the base")
}
