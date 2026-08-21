package sqlitestore

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

// The SQLite store honors the same deletion contract the Postgres store's
// delete_orphans_test.go pins — one contract, two backends. A row that
// describes a block must not outlive the block, whichever verb removes it.

// seedOrphanFixture writes one item, on one stream, with one block carrying a
// row in every table a deletion has to reach. Through the real writers, except
// overlays_ext (its writer is the blockstore session, which imports the sibling
// store package) and proposed_source_changes (SourceProposalStore is written
// for Postgres placeholders).
func seedOrphanFixture(t *testing.T, s *SQLiteStore, projectID, stream, itemName string) string {
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
	blockID := blockIDForSource(t, s, projectID, stream, itemName, "greeting")
	require.NotEmpty(t, blockID)

	require.NoError(t, s.AddBlockNote(ctx, projectID, stream, blockID, model.BlockNote{
		Author: "reviewer@example.com", Text: "the source reads oddly here",
	}))
	_, err := s.UpsertUnitDecisions(ctx, projectID, stream, []venue.UnitDecision{{
		ItemName: itemName, Unit: "greeting", Variant: "nb",
		Status:      string(model.TargetStatusReviewed),
		TargetHash:  state.TargetHash("Hei"),
		ContentHash: state.SourceHash("Hello"),
		Updated:     "2026-08-04T10:00:00Z",
	}})
	require.NoError(t, err)
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO overlays_ext (project_id, stream, block_id, kind, payload)
		 VALUES (?,?,?,'segmentation','{}')`, projectID, stream, blockID)
	require.NoError(t, err)
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO proposed_source_changes
			(id, workspace_id, project_id, stream, item_name, block_id, kind,
			 original_source, proposed_source, status, created_at, updated_at)
		 VALUES (?, 'ws-1', ?, ?, ?, ?, 'text-fix', 'Hello', 'Hello there', 'open',
			 '2026-08-04T10:00:00Z', '2026-08-04T10:00:00Z')`,
		blockID+"-proposal", projectID, stream, itemName, blockID)
	require.NoError(t, err)

	return blockID
}

// blockIDForSource is the stored (internal) id of the block a producer sent
// under sourceID. StoreBlocksForItem maps the caller's id to source_id and
// mints its own, so the id has to be read back rather than assumed.
func blockIDForSource(t *testing.T, s *SQLiteStore, projectID, stream, itemName, sourceID string) string {
	t.Helper()
	rows, err := s.GetBlocks(t.Context(), platstore.BlockQuery{
		ProjectID: projectID, Stream: stream, ItemName: itemName, Limit: 100,
	})
	require.NoError(t, err)
	for _, sb := range rows {
		if sb.SourceID == sourceID {
			return sb.Block.ID
		}
	}
	return ""
}

func countRows(t *testing.T, s *SQLiteStore, table, where string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, s.db.QueryRowContext(t.Context(),
		`SELECT count(*) FROM `+table+` WHERE `+where, args...).Scan(&n))
	return n
}

func TestDeleteBlock_LeavesNoOrphans_SQLite(t *testing.T) {
	s := newTestStore(t)
	p := createTestProject(t, s)
	blockID := seedOrphanFixture(t, s, p.ID, "main", "en.json")

	require.NoError(t, s.DeleteBlock(t.Context(), p.ID, "main", blockID))

	for _, table := range storeutil.BlockScopedTables() {
		assert.Zero(t, countRows(t, s, table, `project_id=? AND block_id=?`, p.ID, blockID),
			"%s: a deleted block leaves nothing behind", table)
	}
	assert.Zero(t, countRows(t, s, "unit_decisions", `project_id=? AND unit=?`, p.ID, "greeting"),
		"unit_decisions: the ledger row goes with the unit")
}

func TestDeleteItem_LeavesNoOrphans_SQLite(t *testing.T) {
	s := newTestStore(t)
	p := createTestProject(t, s)
	blockID := seedOrphanFixture(t, s, p.ID, "main", "en.json")

	require.NoError(t, s.DeleteItem(t.Context(), p.ID, "main", "en.json"))

	for _, table := range storeutil.BlockScopedTables() {
		assert.Zero(t, countRows(t, s, table, `project_id=? AND block_id=?`, p.ID, blockID),
			"%s: a deleted item leaves none of its blocks' rows behind", table)
	}
	assert.Zero(t, countRows(t, s, "unit_decisions", `project_id=? AND item_name=?`, p.ID, "en.json"))
}

// A deleted STREAM is covered in the Postgres suite only: making a second
// stream is CreateStream, and branching is server-only (store.StreamBranchStore).
func TestDeleteProject_LeavesNoOrphans_SQLite(t *testing.T) {
	s := newTestStore(t)
	p := createTestProject(t, s)
	seedOrphanFixture(t, s, p.ID, "main", "en.json")

	require.NoError(t, s.DeleteProject(t.Context(), p.ID))

	tables := append(storeutil.ProjectScopedTablesWithoutCascade(),
		"blocks", "items", "translations", "annotations", "overlays_ext")
	for _, table := range tables {
		assert.Zero(t, countRows(t, s, table, `project_id=?`, p.ID),
			"%s: a deleted project leaves nothing behind", table)
	}
}

// The stream sweep and the block sweep are one list each, shared by both
// dialects: a table added to storeutil is reached by all four verbs at once.
func TestDeletionTableListsAreShared(t *testing.T) {
	assert.Contains(t, storeutil.BlockScopedTables(), "block_notes")
	assert.Contains(t, storeutil.BlockScopedTables(), "proposed_source_changes")
	assert.NotContains(t, storeutil.BlockScopedTables(), "change_log",
		"a change log that forgot the removal would be no log")
	assert.NotContains(t, storeutil.BlockScopedTables(), "version_blocks",
		"a version names the blocks it froze")
}
