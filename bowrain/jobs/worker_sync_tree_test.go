package jobs

import (
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	pb "github.com/neokapi/neokapi/core/proto/sync/v1"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// treeOf builds the declaration a producer would send for the blocks it read,
// so a test says what the source now holds rather than how it is encoded.
func treeOf(items map[string][]*pb.SyncBlock) venue.Tree {
	tree := venue.Tree{}
	for path, blocks := range items {
		ti := venue.TreeItem{Path: path}
		for _, b := range blocks {
			ti.Keys = append(ti.Keys, b.Id)
			ti.Content = append(ti.Content, model.ComputeContentHash(b.SourceText))
		}
		tree[path] = ti
	}
	return tree
}

func declaring(scope []string, tree venue.Tree) map[string]any {
	return map[string]any{
		"scope": scope,
		"tree":  tree,
	}
}

func itemNames(t *testing.T, deps *WorkerDeps, projectID string) []string {
	t.Helper()
	items, err := deps.ContentStore.ListItems(t.Context(), projectID, "main")
	require.NoError(t, err)
	names := make([]string, 0, len(items))
	for _, it := range items {
		names = append(names, it.Name)
	}
	return names
}

// Renaming a file and pushing leaves one item, at the new path, with the
// identity its approvals hang from.
//
// The producer sends nothing at all for an unchanged file that merely moved —
// its blocks are content the venue already holds — so this is decided entirely
// from the declared tree. Getting it wrong is not cosmetic: "delete the old,
// create the new" orphans every approval on the file.
func TestARenamedFileKeepsItsIdentity(t *testing.T) {
	deps := newTestWorkerDeps(t)
	ctx := t.Context()

	projectID := "rename-project"
	require.NoError(t, deps.ContentStore.CreateProject(ctx, &store.Project{ID: projectID, Name: "Rename"}))

	blocks := []*pb.SyncBlock{
		{Id: "b1", ItemName: "old/name.json", SourceText: "One", Translatable: true},
		{Id: "b2", ItemName: "old/name.json", SourceText: "Two", Translatable: true},
	}
	first := uploadPush(t, deps, "job-rename-1", projectID, blocks, declaring(
		[]string{"**"}, treeOf(map[string][]*pb.SyncBlock{"old/name.json": blocks})))
	require.NoError(t, ProcessSyncPushJobForTest(ctx, deps, first.ID))

	before, err := deps.ContentStore.GetItem(ctx, projectID, "main", "old/name.json")
	require.NoError(t, err)
	require.NotNil(t, before)
	originalID := before.ID

	// The push after the rename carries NO blocks: the content is unchanged, so
	// the venue already holds every one of them. Only the declaration moved.
	moved := []*pb.SyncBlock{
		{Id: "b1", ItemName: "new/name.json", SourceText: "One", Translatable: true},
		{Id: "b2", ItemName: "new/name.json", SourceText: "Two", Translatable: true},
	}
	second := uploadPush(t, deps, "job-rename-2", projectID, nil, declaring(
		[]string{"**"}, treeOf(map[string][]*pb.SyncBlock{"new/name.json": moved})))
	require.NoError(t, ProcessSyncPushJobForTest(ctx, deps, second.ID))

	assert.Equal(t, []string{"new/name.json"}, itemNames(t, deps, projectID),
		"a rename leaves one item, not two")

	after, err := deps.ContentStore.GetItem(ctx, projectID, "main", "new/name.json")
	require.NoError(t, err)
	require.NotNil(t, after)
	assert.Equal(t, originalID, after.ID,
		"the moved file keeps the identity its approvals hang from")

	// And its content moved with it, rather than being left addressed to a path
	// nothing points at.
	rows, err := deps.ContentStore.GetBlocks(ctx, store.BlockQuery{
		ProjectID: projectID, Stream: "main", ItemName: "new/name.json", Limit: 100})
	require.NoError(t, err)
	assert.Len(t, rows, 2)
}

// Deleting a file and pushing leaves no item. The push carries nothing about
// it — a deletion sends no block by definition — so the scope and the tree are
// the only things that can say it happened.
func TestADeletedFileLeavesNoItem(t *testing.T) {
	deps := newTestWorkerDeps(t)
	ctx := t.Context()

	projectID := "delete-project"
	require.NoError(t, deps.ContentStore.CreateProject(ctx, &store.Project{ID: projectID, Name: "Delete"}))

	a := []*pb.SyncBlock{{Id: "a1", ItemName: "a.json", SourceText: "Keep", Translatable: true}}
	b := []*pb.SyncBlock{{Id: "b1", ItemName: "b.json", SourceText: "Drop", Translatable: true}}
	first := uploadPush(t, deps, "job-del-1", projectID, append(append([]*pb.SyncBlock{}, a...), b...),
		declaring([]string{"**"}, treeOf(map[string][]*pb.SyncBlock{"a.json": a, "b.json": b})))
	require.NoError(t, ProcessSyncPushJobForTest(ctx, deps, first.ID))
	require.ElementsMatch(t, []string{"a.json", "b.json"}, itemNames(t, deps, projectID))

	second := uploadPush(t, deps, "job-del-2", projectID, nil,
		declaring([]string{"**"}, treeOf(map[string][]*pb.SyncBlock{"a.json": a})))
	require.NoError(t, ProcessSyncPushJobForTest(ctx, deps, second.ID))

	assert.Equal(t, []string{"a.json"}, itemNames(t, deps, projectID),
		"a file the source no longer has leaves no item")
}

// A scoped push touches nothing outside its scope. This is what makes
// `kapi push <subdir>` safe by construction rather than by nobody looking: the
// files outside are out of scope by the same rule that governs the rest.
func TestAScopedPushTouchesNothingOutsideIt(t *testing.T) {
	deps := newTestWorkerDeps(t)
	ctx := t.Context()

	projectID := "scoped-project"
	require.NoError(t, deps.ContentStore.CreateProject(ctx, &store.Project{ID: projectID, Name: "Scoped"}))

	inside := []*pb.SyncBlock{{Id: "i1", ItemName: "docs/a.json", SourceText: "Inside", Translatable: true}}
	outside := []*pb.SyncBlock{{Id: "o1", ItemName: "web/b.json", SourceText: "Outside", Translatable: true}}
	first := uploadPush(t, deps, "job-scope-1", projectID,
		append(append([]*pb.SyncBlock{}, inside...), outside...),
		declaring([]string{"**"}, treeOf(map[string][]*pb.SyncBlock{
			"docs/a.json": inside, "web/b.json": outside})))
	require.NoError(t, ProcessSyncPushJobForTest(ctx, deps, first.ID))
	require.ElementsMatch(t, []string{"docs/a.json", "web/b.json"}, itemNames(t, deps, projectID))

	// A push scoped to docs/ mentions nothing under web/. That silence must not
	// be read as a deletion.
	second := uploadPush(t, deps, "job-scope-2", projectID, nil,
		declaring([]string{"docs"}, treeOf(map[string][]*pb.SyncBlock{"docs/a.json": inside})))
	require.NoError(t, ProcessSyncPushJobForTest(ctx, deps, second.ID))

	assert.ElementsMatch(t, []string{"docs/a.json", "web/b.json"}, itemNames(t, deps, projectID),
		"a scoped push says nothing about the files it did not look at")
}

// A producer too old to declare a scope makes no claim about what is missing,
// so its push stays purely additive — the behaviour that existed before there
// were scopes, kept rather than broken.
func TestAPushWithNoScopeRemovesNothing(t *testing.T) {
	deps := newTestWorkerDeps(t)
	ctx := t.Context()

	projectID := "noscope-project"
	require.NoError(t, deps.ContentStore.CreateProject(ctx, &store.Project{ID: projectID, Name: "No scope"}))

	a := []*pb.SyncBlock{{Id: "a1", ItemName: "a.json", SourceText: "Keep", Translatable: true}}
	b := []*pb.SyncBlock{{Id: "b1", ItemName: "b.json", SourceText: "Also keep", Translatable: true}}
	first := uploadPush(t, deps, "job-noscope-1", projectID,
		append(append([]*pb.SyncBlock{}, a...), b...), nil)
	require.NoError(t, ProcessSyncPushJobForTest(ctx, deps, first.ID))
	require.ElementsMatch(t, []string{"a.json", "b.json"}, itemNames(t, deps, projectID))

	// A tree, but no scope: the venue has nothing that lets it read b.json's
	// absence as a removal.
	second := uploadPush(t, deps, "job-noscope-2", projectID, nil,
		map[string]any{"tree": treeOf(map[string][]*pb.SyncBlock{"a.json": a})})
	require.NoError(t, ProcessSyncPushJobForTest(ctx, deps, second.ID))

	assert.ElementsMatch(t, []string{"a.json", "b.json"}, itemNames(t, deps, projectID),
		"without a declared scope, absence is silence rather than deletion")
}

// The declared tree still prunes blocks within an item, which is the level
// #2124 reached — a string deleted from a file it shares with others.
func TestTheDeclaredTreePrunesBlocksWithinAnItem(t *testing.T) {
	deps := newTestWorkerDeps(t)
	ctx := t.Context()

	projectID := "prune-project"
	require.NoError(t, deps.ContentStore.CreateProject(ctx, &store.Project{ID: projectID, Name: "Prune"}))

	both := []*pb.SyncBlock{
		{Id: "k1", ItemName: "a.json", SourceText: "Stays", Translatable: true},
		{Id: "k2", ItemName: "a.json", SourceText: "Goes", Translatable: true},
	}
	first := uploadPush(t, deps, "job-prune-1", projectID, both,
		declaring([]string{"**"}, treeOf(map[string][]*pb.SyncBlock{"a.json": both})))
	require.NoError(t, ProcessSyncPushJobForTest(ctx, deps, first.ID))

	kept := both[:1]
	second := uploadPush(t, deps, "job-prune-2", projectID, nil,
		declaring([]string{"**"}, treeOf(map[string][]*pb.SyncBlock{"a.json": kept})))
	require.NoError(t, ProcessSyncPushJobForTest(ctx, deps, second.ID))

	rows, err := deps.ContentStore.GetBlocks(ctx, store.BlockQuery{
		ProjectID: projectID, Stream: "main", ItemName: "a.json", Limit: 100})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "Stays", rows[0].SourceText())
}
