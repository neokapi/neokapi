package store

import (
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/store/internal/storeutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// orphanFixture is one item, on one stream, with one block carrying a row in
// every table a deletion has to reach.
type orphanFixture struct {
	projectID string
	stream    string
	itemName  string
	unit      string
	blockID   string
}

// seedOrphanFixture writes the fixture through the real writers: the content
// store for the block, its target and its annotation; AddBlockNote for the
// note; SourceProposalStore for the proposal; UpsertUnitDecisions for the
// ledger row. overlays_ext is the one exception — its writer is the blockstore
// session, which imports this package — so its row is inserted directly.
func seedOrphanFixture(t *testing.T, s *PostgresStore, projectID, stream, itemName string) orphanFixture {
	t.Helper()
	ctx := t.Context()

	require.NoError(t, s.StoreItem(ctx, projectID, stream, &platstore.Item{Name: itemName, Format: "json"}))
	b := model.NewBlock("greeting", "Hello")
	b.Translatable = true
	b.SetTargetText("nb", "Hei")
	b.SetAnno(model.AnnoNote, &model.Notes{Items: []*model.NoteAnnotation{
		{Text: "keep it short", From: "reviewer", Annotates: "source"},
	}})
	require.NoError(t, s.StoreBlocksForItem(ctx, projectID, stream, itemName, []*model.Block{b}))

	rows, err := s.GetBlocks(ctx, platstore.BlockQuery{
		ProjectID: projectID, Stream: stream, ItemName: itemName, Limit: 10,
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	blockID := rows[0].Block.ID

	require.NoError(t, s.AddBlockNote(ctx, projectID, stream, blockID, model.BlockNote{
		Author: "reviewer@example.com", Text: "the source reads oddly here",
	}))
	require.NoError(t, NewSourceProposalStore(s.db.DB).Create(ctx, &ProposedSourceChange{
		ProjectID: projectID, Stream: stream, ItemName: itemName, BlockID: blockID,
		OriginalSource: "Hello", ProposedSource: "Hello there", FinderUser: "reviewer",
	}))
	_, err = s.UpsertUnitDecisions(ctx, projectID, stream, []venue.UnitDecision{{
		ItemName: itemName, Unit: "greeting", Variant: "nb",
		Status:      string(model.TargetStatusReviewed),
		TargetHash:  state.TargetHash("Hei"),
		ContentHash: state.SourceHash("Hello"),
		Updated:     "2026-08-04T10:00:00Z",
	}})
	require.NoError(t, err)
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO overlays_ext (project_id, stream, block_id, kind, payload)
		 VALUES ($1,$2,$3,'segmentation','{}'::jsonb)`, projectID, stream, blockID)
	require.NoError(t, err)

	return orphanFixture{projectID: projectID, stream: stream, itemName: itemName, unit: "greeting", blockID: blockID}
}

func countRows(t *testing.T, s *PostgresStore, table, where string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, s.db.QueryRowContext(t.Context(),
		`SELECT count(*) FROM `+table+` WHERE `+where, args...).Scan(&n))
	return n
}

// TestDeleteBlock_LeavesNoOrphans and its siblings are #1895's regression: a
// row that describes a block must not outlive the block, whichever verb removes
// it. Each case seeds one row per table through the real writer, runs one
// deletion verb, and asserts nothing describing the removed content is left.
func TestDeleteBlock_LeavesNoOrphans(t *testing.T) {
	s := newTestStore(t)
	p := createTestProject(t, s)
	f := seedOrphanFixture(t, s, p.ID, "main", "en.json")

	require.NoError(t, s.DeleteBlock(t.Context(), p.ID, "main", f.blockID))

	for _, table := range storeutil.BlockScopedTables() {
		assert.Zero(t, countRows(t, s, table, `project_id=$1 AND block_id=$2`, p.ID, f.blockID),
			"%s: a deleted block leaves nothing behind", table)
	}
	assert.Zero(t, countRows(t, s, "unit_decisions", `project_id=$1 AND unit=$2`, p.ID, f.unit),
		"unit_decisions: the ledger row goes with the unit")
}

func TestDeleteItem_LeavesNoOrphans(t *testing.T) {
	s := newTestStore(t)
	p := createTestProject(t, s)
	f := seedOrphanFixture(t, s, p.ID, "main", "en.json")

	require.NoError(t, s.DeleteItem(t.Context(), p.ID, "main", "en.json"))

	for _, table := range storeutil.BlockScopedTables() {
		assert.Zero(t, countRows(t, s, table, `project_id=$1 AND block_id=$2`, p.ID, f.blockID),
			"%s: a deleted item leaves none of its blocks' rows behind", table)
	}
	assert.Zero(t, countRows(t, s, "unit_decisions", `project_id=$1 AND item_name=$2`, p.ID, "en.json"))
}

// TestDeleteStream_LeavesNoOrphans: a deleted stream is erased. Every table
// carrying a stream column loses the stream's rows, and a stream created again
// under the same name inherits nothing.
func TestDeleteStream_LeavesNoOrphans(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	// main keeps its own copy of everything: the sweep must not reach it.
	mainFixture := seedOrphanFixture(t, s, p.ID, "main", "en.json")
	require.NoError(t, s.CreateStream(ctx, &platstore.Stream{ProjectID: p.ID, Name: "feature", Parent: "main"}))
	seedOrphanFixture(t, s, p.ID, "feature", "branch.json")

	require.NoError(t, s.DeleteStream(ctx, p.ID, "feature"))

	for _, table := range storeutil.StreamScopedTables() {
		assert.Zero(t, countRows(t, s, table, `project_id=$1 AND stream=$2`, p.ID, "feature"),
			"%s: a deleted stream keeps nothing", table)
		assert.NotZero(t, countRows(t, s, table, `project_id=$1 AND stream=$2`, p.ID, "main"),
			"%s: main's rows are untouched", table)
	}
	// The branch's own item is gone, so its blocks — which no item names any
	// more — are reclaimed with everything filed under them.
	assert.Zero(t, countRows(t, s, "blocks", `project_id=$1 AND item_name=$2`, p.ID, "branch.json"))
	got, err := s.GetBlock(ctx, p.ID, "main", mainFixture.blockID)
	require.NoError(t, err, "main's shared block survives the branch delete")
	assert.Equal(t, "Hello", got.Block.SourceText())

	// A stream created again under the same name starts empty.
	require.NoError(t, s.CreateStream(ctx, &platstore.Stream{ProjectID: p.ID, Name: "feature", Parent: "main"}))
	for _, table := range storeutil.StreamScopedTables() {
		if table == "items" {
			continue // CreateStream copies the parent's items by design
		}
		assert.Zero(t, countRows(t, s, table, `project_id=$1 AND stream=$2`, p.ID, "feature"),
			"%s: a recreated stream inherits nothing from the dead one", table)
	}
}

// TestDeleteProject_LeavesNoOrphans: the projects foreign key cascades most of
// the content, and the verb clears what carries only a bare project_id.
func TestDeleteProject_LeavesNoOrphans(t *testing.T) {
	s := newTestStore(t)
	p := createTestProject(t, s)
	seedOrphanFixture(t, s, p.ID, "main", "en.json")

	require.NoError(t, s.DeleteProject(t.Context(), p.ID))

	tables := append(storeutil.ProjectScopedTablesWithoutCascade(),
		"blocks", "items", "translations", "annotations", "overlays_ext")
	for _, table := range tables {
		assert.Zero(t, countRows(t, s, table, `project_id=$1`, p.ID),
			"%s: a deleted project leaves nothing behind", table)
	}
}
