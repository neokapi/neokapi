package jobs

import (
	"testing"

	pb "github.com/neokapi/neokapi/core/proto/sync/v1"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// previewEntry is a declared collection that names where its strings can be
// read in place.
func previewEntry(name, kind, url string) *pb.SyncContextEntry {
	e := &pb.SyncContextEntry{
		Name:  name,
		Owner: venue.ContextOwnerRecipe,
	}
	if kind != "" || url != "" {
		e.Preview = &pb.SyncPreviewSource{Kind: kind, Url: url}
	}
	e.ContentHash = venue.ComputeContextEntryHash(e)
	return e
}

// The recipe's declaration reaches the row the reviewer reads, per collection —
// two collections in one push name two hosts, which is the case that made a
// project-level setting wrong.
func TestReconcileContext_CarriesThePreviewHostPerCollection(t *testing.T) {
	deps := newContextTestDeps(t)
	ctx := t.Context()
	newContextProject(t, ctx, deps, "p1", "ws1")

	_, err := reconcileContext(ctx, deps, "p1", "main", "ws1", "user-1", []*pb.SyncContextEntry{
		previewEntry("bowrain-app", "storybook", "https://neokapi.github.io/storybook/bowrain/"),
		previewEntry("neokapi-desktop", "storybook", "https://neokapi.github.io/storybook/kapi/"),
		previewEntry("neokapi-docs", "", ""),
	})
	require.NoError(t, err)

	app, err := deps.ContentStore.GetCollectionByName(ctx, "p1", "bowrain-app", "main")
	require.NoError(t, err)
	assert.Equal(t, "storybook", app.PreviewKind)
	assert.Equal(t, "https://neokapi.github.io/storybook/bowrain/", app.PreviewURL)

	desktop, err := deps.ContentStore.GetCollectionByName(ctx, "p1", "neokapi-desktop", "main")
	require.NoError(t, err)
	assert.Equal(t, "https://neokapi.github.io/storybook/kapi/", desktop.PreviewURL,
		"each collection keeps its own host")

	docs, err := deps.ContentStore.GetCollectionByName(ctx, "p1", "neokapi-docs", "main")
	require.NoError(t, err)
	assert.Empty(t, docs.PreviewURL)
}

// Re-pointing a collection at a different host is a recipe edit, and lands.
// The entry hash moves with it, which is what gets the row rewritten at all.
func TestReconcileContext_RepointingTheHostLands(t *testing.T) {
	deps := newContextTestDeps(t)
	ctx := t.Context()
	newContextProject(t, ctx, deps, "p1", "ws1")

	_, err := reconcileContext(ctx, deps, "p1", "main", "ws1", "user-1",
		[]*pb.SyncContextEntry{previewEntry("app", "storybook", "https://old.example/sb/")})
	require.NoError(t, err)

	res, err := reconcileContext(ctx, deps, "p1", "main", "ws1", "user-1",
		[]*pb.SyncContextEntry{previewEntry("app", "storybook", "https://new.example/sb/")})
	require.NoError(t, err)
	assert.Equal(t, []string{"app"}, res.Updated)

	col, err := deps.ContentStore.GetCollectionByName(ctx, "p1", "app", "main")
	require.NoError(t, err)
	assert.Equal(t, "https://new.example/sb/", col.PreviewURL)
}

// A recipe that drops its `preview:` block means the collection no longer has
// one. Leaving the old URL would keep offering a reading of components that may
// no longer be published there.
func TestReconcileContext_DroppingTheHostClearsIt(t *testing.T) {
	deps := newContextTestDeps(t)
	ctx := t.Context()
	newContextProject(t, ctx, deps, "p1", "ws1")

	_, err := reconcileContext(ctx, deps, "p1", "main", "ws1", "user-1",
		[]*pb.SyncContextEntry{previewEntry("app", "storybook", "https://example.dev/sb/")})
	require.NoError(t, err)

	_, err = reconcileContext(ctx, deps, "p1", "main", "ws1", "user-1",
		[]*pb.SyncContextEntry{previewEntry("app", "", "")})
	require.NoError(t, err)

	col, err := deps.ContentStore.GetCollectionByName(ctx, "p1", "app", "main")
	require.NoError(t, err)
	assert.Empty(t, col.PreviewKind)
	assert.Empty(t, col.PreviewURL)
}

// A kind the server does not recognise is stored and served unchanged: what can
// be read is the client's judgement, and a server that dropped an unfamiliar
// kind would make itself the reason a newer reviewer cannot show a preview.
func TestReconcileContext_AnUnfamiliarKindIsStoredNotDropped(t *testing.T) {
	deps := newContextTestDeps(t)
	ctx := t.Context()
	newContextProject(t, ctx, deps, "p1", "ws1")

	_, err := reconcileContext(ctx, deps, "p1", "main", "ws1", "user-1",
		[]*pb.SyncContextEntry{previewEntry("app", "ladle", "https://example.dev/ladle/")})
	require.NoError(t, err)

	col, err := deps.ContentStore.GetCollectionByName(ctx, "p1", "app", "main")
	require.NoError(t, err)
	assert.Equal(t, "ladle", col.PreviewKind)
}
