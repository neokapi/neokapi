package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/storage"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	voicestore "github.com/neokapi/neokapi/bowrain/voice"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	pb "github.com/neokapi/neokapi/core/proto/sync/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTransitionDeps builds the worker the way bowrain-worker does: both stores
// from ONE pool, with the push transition wired over it.
func newTransitionDeps(t *testing.T) (*WorkerDeps, *storage.PgDB) {
	t.Helper()
	db := pgtest.NewTestDB(t)
	cs, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	voice, err := voicestore.NewPostgresVoiceStore(db)
	require.NoError(t, err)

	deps := &WorkerDeps{ContentStore: cs, VoiceStore: voice}
	deps.PushTransition = func(ctx context.Context, fn func(store.PushApplier, coreprofile.Store) error) error {
		return db.Transition(ctx, func(tx storage.Runner) error {
			return fn(cs.Bind(tx), voice.Bind(tx))
		})
	}
	return deps, db
}

func transitionEntries(t *testing.T) []*pb.SyncContextEntry {
	t.Helper()
	voice, err := json.Marshal(&coreprofile.VoiceProfile{
		Name: "Acme Voice",
		Tone: coreprofile.ToneProfile{Formality: "neutral"},
	})
	require.NoError(t, err)
	return []*pb.SyncContextEntry{
		contextEntry("docs", map[string]string{"product": "kapi", "channel": "docs"}, "docs", "Acme Voice", voice),
	}
}

// TestPushTransition_ReconcileRollsBackWithTheContent is what the seam is for.
//
// The reconcile does more than create empty collections: it writes the
// workspace's voice profiles from what the push declared. Run outside the
// transition — as it was — a push that failed afterwards had already changed
// what governs the workspace on the strength of content that never landed.
func TestPushTransition_ReconcileRollsBackWithTheContent(t *testing.T) {
	deps, _ := newTransitionDeps(t)
	ctx := t.Context()
	newContextProject(t, ctx, deps, "p1", "ws1")

	boom := errors.New("the content stage failed")
	err := deps.PushTransition(ctx, func(tx store.PushApplier, voice coreprofile.Store) error {
		if _, rerr := reconcileContext(ctx, tx, voice, "p1", "main", "ws1", "user-1", transitionEntries(t)); rerr != nil {
			return rerr
		}
		// Everything the reconcile wrote is visible inside the transition.
		cols, lerr := tx.ListCollections(ctx, "p1", "main")
		require.NoError(t, lerr)
		assert.NotEmpty(t, cols, "the reconcile's own writes are readable within the transition")
		return boom
	})
	require.ErrorIs(t, err, boom)

	// Nothing survives it — not the collection, and not the voice profile.
	col, cerr := deps.ContentStore.GetCollectionByName(ctx, "p1", "docs", "main")
	assert.True(t, cerr != nil || col == nil,
		"a collection reconciled by a failed push does not outlive it")

	profiles, perr := deps.VoiceStore.ListProfiles(ctx, "ws1")
	require.NoError(t, perr)
	assert.Empty(t, profiles,
		"and neither does the voice profile it wrote — the workspace is governed by what landed")
}

// The committing case still lands both, in the same transition.
func TestPushTransition_CommitLandsBoth(t *testing.T) {
	deps, _ := newTransitionDeps(t)
	ctx := t.Context()
	newContextProject(t, ctx, deps, "p1", "ws1")

	err := deps.PushTransition(ctx, func(tx store.PushApplier, voice coreprofile.Store) error {
		_, rerr := reconcileContext(ctx, tx, voice, "p1", "main", "ws1", "user-1", transitionEntries(t))
		return rerr
	})
	require.NoError(t, err)

	col, cerr := deps.ContentStore.GetCollectionByName(ctx, "p1", "docs", "main")
	require.NoError(t, cerr)
	require.NotNil(t, col)
	assert.Equal(t, "docs", col.Name)

	profiles, perr := deps.VoiceStore.ListProfiles(ctx, "ws1")
	require.NoError(t, perr)
	require.Len(t, profiles, 1)
	assert.Equal(t, "Acme Voice", profiles[0].Name)
}

// The pooled store stays usable beside a bound one: Bind copies rather than
// mutating, so a worker that binds a transition does not redirect every other
// caller's statements into it.
func TestBindLeavesThePooledStoreAlone(t *testing.T) {
	deps, db := newTransitionDeps(t)
	ctx := t.Context()
	newContextProject(t, ctx, deps, "p1", "ws1")

	voice := deps.VoiceStore.(*voicestore.PostgresVoiceStore)
	err := db.Transition(ctx, func(tx storage.Runner) error {
		bound := voice.Bind(tx)
		require.NoError(t, bound.CreateProfile(ctx, &coreprofile.VoiceProfile{
			ID: "p-bound", Scope: "ws1", Name: "Bound",
		}))
		// The unbound store is still the pool, so it cannot see the
		// uncommitted row.
		got, lerr := voice.ListProfiles(ctx, "ws1")
		require.NoError(t, lerr)
		assert.Empty(t, got, "the pooled store is not redirected into the transition")
		return nil
	})
	require.NoError(t, err)

	got, lerr := voice.ListProfiles(ctx, "ws1")
	require.NoError(t, lerr)
	require.Len(t, got, 1, "and sees it once the transition commits")
}
