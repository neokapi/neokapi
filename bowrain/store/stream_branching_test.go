package store

import (
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A stream owns its content.
//
// It did not. CreateStream copied item rows and left blocks shared, so two
// branches could not differ in source by construction, and MergeStream counted
// the change log without moving a row — it returned a MergeResult describing
// work nothing had done.
//
// These tests are about the property, not the plumbing: a branch is independent,
// a diff says how, and a merge either fast-forwards or refuses.

func seedStream(t *testing.T, s *PostgresStore, projectID, stream, itemName string, units ...string) {
	t.Helper()
	ctx := t.Context()
	require.NoError(t, s.StoreItem(ctx, projectID, stream, &platstore.Item{Name: itemName, Format: "json"}))
	blocks := make([]*model.Block, 0, len(units))
	for _, u := range units {
		b := model.NewBlock(u, "text of "+u)
		b.Translatable = true
		b.SetTargetText("nb", "tekst for "+u)
		blocks = append(blocks, b)
	}
	require.NoError(t, s.StoreBlocksForItem(ctx, projectID, stream, itemName, blocks))
}

func streamBlockCount(t *testing.T, s *PostgresStore, projectID, stream string) int {
	t.Helper()
	return countRows(t, s, "blocks", `project_id=$1 AND stream=$2`, projectID, stream)
}

// sourceOf reads one unit's source text on one stream, by the producer's key.
func sourceOf(t *testing.T, s *PostgresStore, projectID, stream, unit string) string {
	t.Helper()
	rows, err := s.GetBlocks(t.Context(), platstore.BlockQuery{
		ProjectID: projectID, Stream: stream, Limit: 100,
	})
	require.NoError(t, err)
	for _, r := range rows {
		if r.SourceID == unit {
			return r.Block.SourceText()
		}
	}
	return ""
}

func TestBranchStartsAsItsParentAndKeepsTheSameIds(t *testing.T) {
	s := newTestStore(t)
	p := createTestProject(t, s)
	ctx := t.Context()
	seedStream(t, s, p.ID, "main", "en.json", "greeting", "farewell")

	require.NoError(t, s.CreateStream(ctx, &platstore.Stream{
		ProjectID: p.ID, Name: "feature", Parent: "main",
	}))

	// The content came across, not just the item rows.
	assert.Equal(t, 2, streamBlockCount(t, s, p.ID, "feature"),
		"a branch holds its parent's content, not a reference to it")
	assert.Equal(t, "text of greeting", sourceOf(t, s, p.ID, "feature", "greeting"))

	// Same ids on both sides — what lets a diff compare by key.
	main, err := s.GetBlocks(ctx, platstore.BlockQuery{ProjectID: p.ID, Stream: "main", Limit: 100})
	require.NoError(t, err)
	branch, err := s.GetBlocks(ctx, platstore.BlockQuery{ProjectID: p.ID, Stream: "feature", Limit: 100})
	require.NoError(t, err)
	mainIDs := map[string]bool{}
	for _, b := range main {
		mainIDs[b.Block.ID] = true
	}
	require.NotEmpty(t, mainIDs)
	for _, b := range branch {
		assert.True(t, mainIDs[b.Block.ID], "branch block %s should carry its parent's id", b.Block.ID)
	}

	// And the governance came with it, which is why a branch is reviewable.
	assert.Equal(t, 2, countRows(t, s, "translations", `project_id=$1 AND stream=$2`, p.ID, "feature"),
		"a branch inherits its parent's translations")
}

func TestEditingABranchLeavesItsParentAlone(t *testing.T) {
	s := newTestStore(t)
	p := createTestProject(t, s)
	ctx := t.Context()
	seedStream(t, s, p.ID, "main", "en.json", "greeting")
	require.NoError(t, s.CreateStream(ctx, &platstore.Stream{
		ProjectID: p.ID, Name: "feature", Parent: "main",
	}))

	// Rewrite the source on the branch only.
	edited := model.NewBlock("greeting", "rewritten on the branch")
	edited.Translatable = true
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "feature", "en.json", []*model.Block{edited}))

	assert.Equal(t, "rewritten on the branch", sourceOf(t, s, p.ID, "feature", "greeting"))
	assert.Equal(t, "text of greeting", sourceOf(t, s, p.ID, "main", "greeting"),
		"main must not see an edit made on a branch")
}

// Every read is scoped, so a branch does not inflate its parent's totals. This
// is the test that catches a query somebody forgot to scope.
func TestBranchDoesNotLeakIntoItsParentsCounts(t *testing.T) {
	s := newTestStore(t)
	p := createTestProject(t, s)
	ctx := t.Context()
	seedStream(t, s, p.ID, "main", "en.json", "greeting", "farewell")

	before, err := s.CountBlocks(ctx, platstore.BlockQuery{
		ProjectID: p.ID, Stream: "main", TargetLocale: "nb",
	})
	require.NoError(t, err)
	require.Equal(t, 2, before.Translatable)

	require.NoError(t, s.CreateStream(ctx, &platstore.Stream{
		ProjectID: p.ID, Name: "feature", Parent: "main",
	}))
	seedStream(t, s, p.ID, "feature", "branch-only.json", "extra")

	after, err := s.CountBlocks(ctx, platstore.BlockQuery{
		ProjectID: p.ID, Stream: "main", TargetLocale: "nb",
	})
	require.NoError(t, err)
	assert.Equal(t, before.Translatable, after.Translatable,
		"main's counts must not move because a branch gained content")

	stats, err := s.GetBlockStats(ctx, p.ID, "main")
	require.NoError(t, err)
	assert.Len(t, stats, 2, "main's per-block stats cover main's blocks only")
}

func TestDeletingABranchLeavesItsParentWhole(t *testing.T) {
	s := newTestStore(t)
	p := createTestProject(t, s)
	ctx := t.Context()
	seedStream(t, s, p.ID, "main", "en.json", "greeting", "farewell")
	require.NoError(t, s.CreateStream(ctx, &platstore.Stream{
		ProjectID: p.ID, Name: "feature", Parent: "main",
	}))

	require.NoError(t, s.DeleteStream(ctx, p.ID, "feature"))

	assert.Zero(t, streamBlockCount(t, s, p.ID, "feature"), "a deleted stream keeps nothing")
	assert.Equal(t, 2, streamBlockCount(t, s, p.ID, "main"), "and its parent is untouched")
	assert.Equal(t, "text of greeting", sourceOf(t, s, p.ID, "main", "greeting"))
}

// A diff reads both sides rather than replaying the change log, so a unit edited
// and edited back reports no difference — the log has two entries and the
// streams have none.
func TestDiffComparesContentRatherThanHistory(t *testing.T) {
	s := newTestStore(t)
	p := createTestProject(t, s)
	ctx := t.Context()
	seedStream(t, s, p.ID, "main", "en.json", "greeting", "farewell")
	require.NoError(t, s.CreateStream(ctx, &platstore.Stream{
		ProjectID: p.ID, Name: "feature", Parent: "main",
	}))

	write := func(text string) {
		b := model.NewBlock("greeting", text)
		b.Translatable = true
		require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "feature", "en.json", []*model.Block{b}))
	}
	write("changed")
	write("text of greeting") // and back again

	diff, err := s.DiffStream(ctx, p.ID, "feature")
	require.NoError(t, err)
	assert.Empty(t, diff.Changes, "a unit edited back to its parent's wording is not a difference")

	write("genuinely different")
	diff, err = s.DiffStream(ctx, p.ID, "feature")
	require.NoError(t, err)
	require.Len(t, diff.Changes, 1)
	assert.Equal(t, platstore.ChangeModified, diff.Changes[0].ChangeType)
}

func TestMergeFastForwardsTheParentOntoTheBranch(t *testing.T) {
	s := newTestStore(t)
	p := createTestProject(t, s)
	ctx := t.Context()
	seedStream(t, s, p.ID, "main", "en.json", "greeting")
	require.NoError(t, s.CreateStream(ctx, &platstore.Stream{
		ProjectID: p.ID, Name: "feature", Parent: "main",
	}))

	edited := model.NewBlock("greeting", "the branch's wording")
	edited.Translatable = true
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "feature", "en.json", []*model.Block{edited}))

	_, err := s.MergeStream(ctx, p.ID, "feature", platstore.MergeOptions{})
	require.NoError(t, err)

	assert.Equal(t, "the branch's wording", sourceOf(t, s, p.ID, "main", "greeting"),
		"a fast-forward moves the branch's content onto its parent")

	diff, err := s.DiffStream(ctx, p.ID, "feature")
	require.NoError(t, err)
	assert.Empty(t, diff.Changes, "after a fast-forward the two streams agree")
}

// The refusal is the point of choosing fast-forward: a merge that had to pick
// between two edits of one unit would be picking between two people's approved
// wording.
func TestMergeRefusesWhenTheParentHasMoved(t *testing.T) {
	s := newTestStore(t)
	p := createTestProject(t, s)
	ctx := t.Context()
	seedStream(t, s, p.ID, "main", "en.json", "greeting")
	require.NoError(t, s.CreateStream(ctx, &platstore.Stream{
		ProjectID: p.ID, Name: "feature", Parent: "main",
	}))

	onBranch := model.NewBlock("greeting", "the branch's wording")
	onBranch.Translatable = true
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "feature", "en.json", []*model.Block{onBranch}))

	// main moves after the branch was taken.
	onMain := model.NewBlock("greeting", "main moved on")
	onMain.Translatable = true
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{onMain}))

	_, err := s.MergeStream(ctx, p.ID, "feature", platstore.MergeOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fast-forward only")
	assert.Equal(t, "main moved on", sourceOf(t, s, p.ID, "main", "greeting"),
		"a refused merge changes nothing")
}

// A rename is a rename: the address moves and the approvals stay.
func TestRenamingAnItemKeepsItsIdentity(t *testing.T) {
	s := newTestStore(t)
	p := createTestProject(t, s)
	ctx := t.Context()
	seedStream(t, s, p.ID, "main", "docs/intro.md", "greeting")

	item, err := s.GetItem(ctx, p.ID, "main", "docs/intro.md")
	require.NoError(t, err)
	require.NotEmpty(t, item.ID, "an item carries an identity separate from its path")

	// The path is an address now, not the key, so it can move.
	_, err = s.db.ExecContext(ctx,
		`UPDATE items SET name='docs/getting-started.md' WHERE project_id=$1 AND stream='main' AND id=$2`,
		p.ID, item.ID)
	require.NoError(t, err)

	moved, err := s.GetItem(ctx, p.ID, "main", "docs/getting-started.md")
	require.NoError(t, err)
	assert.Equal(t, item.ID, moved.ID, "the same file, at a new address")
}
