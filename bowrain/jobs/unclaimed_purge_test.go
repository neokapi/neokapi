package jobs

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/neokapi/neokapi/bowrain/auth"
	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/storage"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newPurgeTestDB opens a real PostgreSQL from BOWRAIN_TEST_DATABASE_URL, closed
// per test (keeps the package goleak check green), cleaning the tables the
// purge touches. Skips when the env var is unset.
func newPurgeTestDB(t *testing.T) *storage.PgDB {
	t.Helper()
	dbURL := os.Getenv("BOWRAIN_TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("BOWRAIN_TEST_DATABASE_URL not set")
	}
	db, err := storage.OpenPostgres(dbURL)
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = db.ExecContext(ctx, "DELETE FROM projects")
		_, _ = db.ExecContext(ctx, "DELETE FROM unclaimed_projects")
		closePgDB(db)
	})
	return db
}

// seedUnclaimed inserts an unclaimed auth record plus its content-store project.
func seedUnclaimed(t *testing.T, authStore *auth.PostgresAuthStore, cs store.ContentStore, projectID, tokenHash, workspaceID string, expiresAt time.Time) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, authStore.CreateUnclaimedProject(ctx, projectID, tokenHash, "Proj "+projectID, "en", "fr", expiresAt))
	require.NoError(t, cs.CreateProject(ctx, &store.Project{
		ID:                    projectID,
		Name:                  "Proj " + projectID,
		DefaultSourceLanguage: model.LocaleID("en"),
		TargetLanguages:       []model.LocaleID{"fr"},
		WorkspaceID:           workspaceID,
	}))
}

func TestUnclaimedProjectPurger_PurgeOnce(t *testing.T) {
	db := newPurgeTestDB(t)
	ctx := context.Background()

	authStore, err := auth.NewAuthStoreFromDB(db)
	require.NoError(t, err)

	cs, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)

	now := time.Now()

	// expired + unclaimed → auth record purged AND content project deleted.
	seedUnclaimed(t, authStore, cs, "proj-expired", "hash-expired", "", now.Add(-time.Hour))

	// live (unexpired) → survives entirely.
	seedUnclaimed(t, authStore, cs, "proj-live", "hash-live", "", now.Add(24*time.Hour))

	// expired but WorkspaceID set (a claim raced ahead) → auth record purged,
	// but the CLAIMED content project must be guarded from deletion.
	seedUnclaimed(t, authStore, cs, "proj-claimed", "hash-claimed", "ws-1", now.Add(-time.Hour))

	// a non-anonymous WorkspaceID-less project (e.g. gRPC-created) with NO
	// unclaimed record → never in the expired-ID set, so it must survive.
	require.NoError(t, cs.CreateProject(ctx, &store.Project{
		ID:                    "proj-grpc",
		Name:                  "grpc",
		DefaultSourceLanguage: model.LocaleID("en"),
		TargetLanguages:       []model.LocaleID{"de"},
	}))

	purger := NewUnclaimedProjectPurger(UnclaimedPurgeConfig{
		Lister:   NewPgUnclaimedLister(db),
		Auth:     authStore,
		Projects: cs,
		Interval: time.Hour, // irrelevant; we drive purgeOnce directly
	})
	t.Cleanup(purger.Close)

	purgedAuth, deleted, err := purger.purgeOnce(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, purgedAuth, "both expired unclaimed auth records purged")
	assert.Equal(t, 1, deleted, "only the expired unclaimed content project deleted")

	// Orphan is gone.
	_, err = cs.GetProject(ctx, "proj-expired")
	require.Error(t, err, "expired unclaimed content project must be deleted")

	// Live project survives (both auth record and content project).
	live, err := cs.GetProject(ctx, "proj-live")
	require.NoError(t, err)
	assert.Equal(t, "proj-live", live.ID)
	_, err = authStore.GetUnclaimedByToken(ctx, "hash-live")
	require.NoError(t, err, "unexpired unclaimed auth record survives")

	// Claimed project is guarded despite its auth record having expired.
	claimed, err := cs.GetProject(ctx, "proj-claimed")
	require.NoError(t, err, "claimed project must never be deleted")
	assert.Equal(t, "ws-1", claimed.WorkspaceID)

	// Unrelated non-anonymous project survives.
	_, err = cs.GetProject(ctx, "proj-grpc")
	require.NoError(t, err)

	// Expired auth records are gone (purged by PurgeExpiredUnclaimed).
	_, err = authStore.GetUnclaimedByToken(ctx, "hash-expired")
	require.Error(t, err)
	_, err = authStore.GetUnclaimedByToken(ctx, "hash-claimed")
	assert.Error(t, err)
}

func TestPgUnclaimedLister_ListsOnlyExpired(t *testing.T) {
	db := newPurgeTestDB(t)
	ctx := context.Background()

	authStore, err := auth.NewAuthStoreFromDB(db)
	require.NoError(t, err)

	now := time.Now()
	require.NoError(t, authStore.CreateUnclaimedProject(ctx, "a", "h-a", "A", "en", "fr", now.Add(-time.Hour)))
	require.NoError(t, authStore.CreateUnclaimedProject(ctx, "b", "h-b", "B", "en", "fr", now.Add(-2*time.Hour)))
	require.NoError(t, authStore.CreateUnclaimedProject(ctx, "c", "h-c", "C", "en", "fr", now.Add(time.Hour)))

	ids, err := NewPgUnclaimedLister(db).ListExpiredUnclaimedProjectIDs(ctx)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"a", "b"}, ids)
}

// Compile-time checks that the production stores satisfy the purge interfaces.
var (
	_ UnclaimedAuthPurger = (*auth.PostgresAuthStore)(nil)
	_ OrphanProjectStore  = (store.ContentStore)(nil)
)
