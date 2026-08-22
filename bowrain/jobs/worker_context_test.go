package jobs

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/store"
	bloblocal "github.com/neokapi/neokapi/bowrain/storage/localblob"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	voicestore "github.com/neokapi/neokapi/bowrain/voice"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	pb "github.com/neokapi/neokapi/core/proto/sync/v1"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newContextTestDeps builds WorkerDeps with a content store and a voice store,
// so the context reconcile can both create collections and bind the voices they
// carry.
func newContextTestDeps(t *testing.T) *WorkerDeps {
	t.Helper()
	db := pgtest.NewTestDB(t)
	cs, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	bs, err := bloblocal.New(t.TempDir())
	require.NoError(t, err)
	voice, err := voicestore.NewPostgresVoiceStore(db)
	require.NoError(t, err)
	return &WorkerDeps{ContentStore: cs, BlobStore: bs, VoiceStore: voice}
}

// contextEntry builds a declared collection entry with its content hash
// stamped, exactly as the client stamps it before negotiating.
func contextEntry(name string, coords map[string]string, channel, voice string, voiceJSON []byte) *pb.SyncContextEntry {
	e := &pb.SyncContextEntry{
		Name:             name,
		Coordinates:      coords,
		Channel:          channel,
		VoiceProfile:     voice,
		VoiceProfileJson: voiceJSON,
		Owner:            venue.ContextOwnerRecipe,
	}
	e.ContentHash = venue.ComputeContextEntryHash(e)
	return e
}

func newContextProject(t *testing.T, ctx context.Context, deps *WorkerDeps, projectID, workspaceID string) {
	t.Helper()
	require.NoError(t, deps.ContentStore.CreateProject(ctx, &store.Project{
		ID: projectID, Name: projectID, WorkspaceID: workspaceID,
	}))
}

// TestReconcileContext_CreatesCollectionWithGovernance is the base case: a
// declared collection becomes a row, and the governance resolved for its point
// is written into the two keys core/profile reads it from — so the collection
// is not merely recorded but actually governs.
func TestReconcileContext_CreatesCollectionWithGovernance(t *testing.T) {
	deps := newContextTestDeps(t)
	ctx := t.Context()
	newContextProject(t, ctx, deps, "p1", "ws1")

	voice, err := json.Marshal(&coreprofile.VoiceProfile{
		Name: "Acme Voice",
		Tone: coreprofile.ToneProfile{Formality: "neutral"},
	})
	require.NoError(t, err)

	entries := []*pb.SyncContextEntry{
		contextEntry("docs", map[string]string{"product": "kapi", "channel": "docs"}, "docs", "Acme Voice", voice),
	}
	res, err := reconcileContext(ctx, deps, "p1", "main", "ws1", "user-1", entries)
	require.NoError(t, err)
	assert.Equal(t, []string{"docs"}, res.Created)

	col, err := deps.ContentStore.GetCollectionByName(ctx, "p1", "docs", "main")
	require.NoError(t, err)
	require.NotNil(t, col)
	assert.Equal(t, venue.ContextOwnerRecipe, col.Owner)
	assert.Equal(t, map[string]string{"product": "kapi", "channel": "docs"}, col.Context)
	assert.Equal(t, entries[0].ContentHash, col.ContextHash)

	// The governance landed where the resolution path reads it.
	profileID := col.ConnectorConfig[coreprofile.PropertyProfileID]
	require.NotEmpty(t, profileID, "the resolved profile id must be bound on the collection")
	assert.Equal(t, "docs", col.ConnectorConfig[coreprofile.PropertyChannel])

	stored, err := deps.VoiceStore.GetProfile(ctx, profileID)
	require.NoError(t, err)
	assert.Equal(t, "Acme Voice", stored.Name)
	assert.Equal(t, "neutral", stored.Tone.Formality)
}

// TestReconcileContext_DoublePushIsIdempotent is the property the whole
// content-hash column exists for. Pushing the same recipe twice must produce
// one row and leave it untouched the second time — a row rewritten with
// identical values would churn UpdatedAt and make "when did this collection
// last change?" unanswerable.
func TestReconcileContext_DoublePushIsIdempotent(t *testing.T) {
	deps := newContextTestDeps(t)
	ctx := t.Context()
	newContextProject(t, ctx, deps, "p1", "ws1")

	voice, err := json.Marshal(&coreprofile.VoiceProfile{Name: "Acme Voice"})
	require.NoError(t, err)
	entries := func() []*pb.SyncContextEntry {
		return []*pb.SyncContextEntry{
			contextEntry("docs", map[string]string{"product": "kapi"}, "", "Acme Voice", voice),
		}
	}

	first, err := reconcileContext(ctx, deps, "p1", "main", "ws1", "user-1", entries())
	require.NoError(t, err)
	require.Equal(t, []string{"docs"}, first.Created)

	before, err := deps.ContentStore.GetCollectionByName(ctx, "p1", "docs", "main")
	require.NoError(t, err)

	second, err := reconcileContext(ctx, deps, "p1", "main", "ws1", "user-1", entries())
	require.NoError(t, err)
	assert.Empty(t, second.Created, "the second push must not create a second row")
	assert.Empty(t, second.Updated, "an unchanged recipe must not rewrite the row")
	assert.Equal(t, []string{"docs"}, second.Unchanged)

	all, err := deps.ContentStore.ListCollections(ctx, "p1", "main")
	require.NoError(t, err)
	assert.Len(t, all, 1, "two pushes, one collection")

	after, err := deps.ContentStore.GetCollectionByName(ctx, "p1", "docs", "main")
	require.NoError(t, err)
	assert.Equal(t, before.ID, after.ID)
	assert.True(t, before.UpdatedAt.Equal(after.UpdatedAt),
		"an unchanged recipe must not churn UpdatedAt: was %s, now %s", before.UpdatedAt, after.UpdatedAt)

	// The voice was upserted once, not versioned on every push.
	profiles, err := deps.VoiceStore.ListProfiles(ctx, "ws1")
	require.NoError(t, err)
	require.Len(t, profiles, 1)
	assert.Equal(t, 1, profiles[0].Version, "an unedited voice must not manufacture a version")
}

// TestReconcileContext_ChangedRecipeUpdates is the other half of idempotence:
// a recipe that actually moved must land. Without this the hash comparison
// above could pass by never writing anything.
func TestReconcileContext_ChangedRecipeUpdates(t *testing.T) {
	deps := newContextTestDeps(t)
	ctx := t.Context()
	newContextProject(t, ctx, deps, "p1", "ws1")

	_, err := reconcileContext(ctx, deps, "p1", "main", "ws1", "user-1", []*pb.SyncContextEntry{
		contextEntry("docs", map[string]string{"product": "kapi"}, "", "", nil),
	})
	require.NoError(t, err)

	res, err := reconcileContext(ctx, deps, "p1", "main", "ws1", "user-1", []*pb.SyncContextEntry{
		contextEntry("docs", map[string]string{"product": "bowrain"}, "email", "", nil),
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"docs"}, res.Updated)

	col, err := deps.ContentStore.GetCollectionByName(ctx, "p1", "docs", "main")
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"product": "bowrain"}, col.Context)
	assert.Equal(t, "email", col.ConnectorConfig[coreprofile.PropertyChannel])
}

// TestReconcileContext_UndeclaredIsReportedNotDeleted pins the contract that
// matters most: a collection is where content lives, so a recipe edit that
// stops declaring one is reported and nothing else. Deleting it would take the
// content grouped under it, and no recipe edit is consent to that.
func TestReconcileContext_UndeclaredIsReportedNotDeleted(t *testing.T) {
	deps := newContextTestDeps(t)
	ctx := t.Context()
	newContextProject(t, ctx, deps, "p1", "ws1")

	_, err := reconcileContext(ctx, deps, "p1", "main", "ws1", "user-1", []*pb.SyncContextEntry{
		contextEntry("docs", nil, "", "", nil),
		contextEntry("marketing", nil, "", "", nil),
	})
	require.NoError(t, err)

	// The recipe drops "marketing".
	res, err := reconcileContext(ctx, deps, "p1", "main", "ws1", "user-1", []*pb.SyncContextEntry{
		contextEntry("docs", nil, "", "", nil),
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"marketing"}, res.Undeclared)

	still, err := deps.ContentStore.GetCollectionByName(ctx, "p1", "marketing", "main")
	require.NoError(t, err)
	require.NotNil(t, still, "an undeclared collection must survive the push that stopped declaring it")
	assert.Equal(t, "marketing", still.Name)
}

// TestReconcileContext_WorkspaceOwnedIsNotReported completes the ownership
// rule from the other side. A collection the workspace created was never the
// recipe's to declare, so its absence from a push says nothing and must not be
// reported as a removal.
func TestReconcileContext_WorkspaceOwnedIsNotReported(t *testing.T) {
	deps := newContextTestDeps(t)
	ctx := t.Context()
	newContextProject(t, ctx, deps, "p1", "ws1")

	require.NoError(t, deps.ContentStore.CreateCollection(ctx, &store.Collection{
		ProjectID: "p1", Name: "uploads", Stream: "main", Owner: venue.ContextOwnerWorkspace,
	}))

	res, err := reconcileContext(ctx, deps, "p1", "main", "ws1", "user-1", []*pb.SyncContextEntry{
		contextEntry("docs", nil, "", "", nil),
	})
	require.NoError(t, err)
	assert.Empty(t, res.Undeclared, "a workspace-owned collection is not the recipe's to declare or drop")
}

// TestReconcileContext_DeclaringClaimsAWorkspaceCollection pins what happens
// when both sides name the same collection: declaring it in kapi.yaml claims
// it, so ownership — the field every later refusal is keyed on — flips to the
// recipe.
func TestReconcileContext_DeclaringClaimsAWorkspaceCollection(t *testing.T) {
	deps := newContextTestDeps(t)
	ctx := t.Context()
	newContextProject(t, ctx, deps, "p1", "ws1")

	require.NoError(t, deps.ContentStore.CreateCollection(ctx, &store.Collection{
		ProjectID: "p1", Name: "docs", Stream: "main", Owner: venue.ContextOwnerWorkspace,
	}))

	res, err := reconcileContext(ctx, deps, "p1", "main", "ws1", "user-1", []*pb.SyncContextEntry{
		contextEntry("docs", map[string]string{"product": "kapi"}, "", "", nil),
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"docs"}, res.Claimed)
	assert.Empty(t, res.Created, "claiming must not create a duplicate row")

	col, err := deps.ContentStore.GetCollectionByName(ctx, "p1", "docs", "main")
	require.NoError(t, err)
	assert.Equal(t, venue.ContextOwnerRecipe, col.Owner)
}

// TestReconcileContext_ChangedVoiceLandsAsANewVersion pins the brand fold-in:
// the voice now travels inside the push, and an edited one goes through the
// store's versioning rather than clobbering what the server holds.
func TestReconcileContext_ChangedVoiceLandsAsANewVersion(t *testing.T) {
	deps := newContextTestDeps(t)
	ctx := t.Context()
	newContextProject(t, ctx, deps, "p1", "ws1")

	first, err := json.Marshal(&coreprofile.VoiceProfile{
		Name: "Acme Voice", Tone: coreprofile.ToneProfile{Formality: "neutral"},
	})
	require.NoError(t, err)
	_, err = reconcileContext(ctx, deps, "p1", "main", "ws1", "user-1", []*pb.SyncContextEntry{
		contextEntry("docs", nil, "", "Acme Voice", first),
	})
	require.NoError(t, err)

	second, err := json.Marshal(&coreprofile.VoiceProfile{
		Name: "Acme Voice", Tone: coreprofile.ToneProfile{Formality: "formal"},
	})
	require.NoError(t, err)
	_, err = reconcileContext(ctx, deps, "p1", "main", "ws1", "user-1", []*pb.SyncContextEntry{
		contextEntry("docs", nil, "", "Acme Voice", second),
	})
	require.NoError(t, err)

	profiles, err := deps.VoiceStore.ListProfiles(ctx, "ws1")
	require.NoError(t, err)
	require.Len(t, profiles, 1, "the voice is matched by name, not duplicated")
	assert.Equal(t, "formal", profiles[0].Tone.Formality)
	assert.Equal(t, 2, profiles[0].Version, "an edited voice lands as a new version")

	versions, err := deps.VoiceStore.ListProfileVersions(ctx, profiles[0].ID)
	require.NoError(t, err)
	assert.NotEmpty(t, versions, "the superseded state is archived, not lost")
}

// TestReconcileContext_NoVoiceCarriesStructureOnly pins what --no-brand means
// now that governance travels with the push: the collections still land, and
// nothing binds a voice.
func TestReconcileContext_NoVoiceCarriesStructureOnly(t *testing.T) {
	deps := newContextTestDeps(t)
	ctx := t.Context()
	newContextProject(t, ctx, deps, "p1", "ws1")

	res, err := reconcileContext(ctx, deps, "p1", "main", "ws1", "user-1", []*pb.SyncContextEntry{
		contextEntry("docs", map[string]string{"product": "kapi"}, "", "", nil),
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"docs"}, res.Created)

	col, err := deps.ContentStore.GetCollectionByName(ctx, "p1", "docs", "main")
	require.NoError(t, err)
	assert.NotContains(t, col.ConnectorConfig, coreprofile.PropertyProfileID)

	profiles, err := deps.VoiceStore.ListProfiles(ctx, "ws1")
	require.NoError(t, err)
	assert.Empty(t, profiles)
}

// TestReconcileContext_UnclaimedProjectReconcilesStructure pins the degraded
// case: a project not claimed into a workspace has no brand hub to bind a voice
// in, and the honest outcome is the structure without the governance rather
// than a failed push.
func TestReconcileContext_UnclaimedProjectReconcilesStructure(t *testing.T) {
	deps := newContextTestDeps(t)
	ctx := t.Context()
	newContextProject(t, ctx, deps, "p1", "")

	voice, err := json.Marshal(&coreprofile.VoiceProfile{Name: "Acme Voice"})
	require.NoError(t, err)
	res, err := reconcileContext(ctx, deps, "p1", "main", "", "user-1", []*pb.SyncContextEntry{
		contextEntry("docs", nil, "", "Acme Voice", voice),
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"docs"}, res.Created)
	assert.Empty(t, res.ProfileIDs)
}
