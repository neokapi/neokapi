package store_test

import (
	"context"
	"path/filepath"
	"testing"

	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// forgeInstallationBackends returns one store per backend the table ships on.
// The store is a single *sql.DB implementation, so the point of running the
// suite twice is the SCHEMA: the pair of migrations (Postgres 13, SQLite 12)
// must actually apply, and the columns must scan identically on both drivers —
// a BIGINT/INTEGER id and TEXT RFC3339 timestamps.
func forgeInstallationBackends(t *testing.T) map[string]func(t *testing.T) *bstore.ForgeInstallationStore {
	t.Helper()
	return map[string]func(t *testing.T) *bstore.ForgeInstallationStore{
		"postgres": func(t *testing.T) *bstore.ForgeInstallationStore {
			t.Helper()
			db := pgtest.NewTestDB(t)
			_, err := bstore.NewPostgresStoreFromDB(db)
			require.NoError(t, err)
			return bstore.NewForgeInstallationStore(db.DB)
		},
		"sqlite": func(t *testing.T) *bstore.ForgeInstallationStore {
			t.Helper()
			cs, err := sqlitestore.NewSQLiteStore(filepath.Join(t.TempDir(), "installations.db"))
			require.NoError(t, err)
			t.Cleanup(func() { _ = cs.Close() })
			return bstore.NewForgeInstallationStore(cs.DB())
		},
	}
}

func TestForgeInstallationStore(t *testing.T) {
	for name, open := range forgeInstallationBackends(t) {
		t.Run(name, func(t *testing.T) {
			t.Run("record leaves the installation unclaimed", func(t *testing.T) {
				s := open(t)
				ctx := context.Background()

				require.NoError(t, s.Record(ctx, 42, "acme"))
				inst, err := s.Get(ctx, 42)
				require.NoError(t, err)
				assert.Equal(t, int64(42), inst.InstallationID)
				assert.Equal(t, "acme", inst.Account)
				assert.False(t, inst.Claimed())
				assert.False(t, inst.CreatedAt.IsZero(), "timestamps round-trip through TEXT RFC3339")

				owned, err := s.OwnedBy(ctx, 42, "ws1")
				require.NoError(t, err)
				assert.False(t, owned, "recorded is not owned")
			})

			t.Run("claim binds, and the first claim wins", func(t *testing.T) {
				s := open(t)
				ctx := context.Background()

				require.NoError(t, s.Record(ctx, 42, "acme"))
				inst, err := s.Claim(ctx, 42, "ws1")
				require.NoError(t, err)
				assert.Equal(t, "ws1", inst.WorkspaceID)
				assert.Equal(t, "acme", inst.Account, "claiming must not clear the webhook's metadata")

				owned, err := s.OwnedBy(ctx, 42, "ws1")
				require.NoError(t, err)
				assert.True(t, owned)
				owned, err = s.OwnedBy(ctx, 42, "ws2")
				require.NoError(t, err)
				assert.False(t, owned)

				// Re-claiming from the owner is idempotent; anyone else is refused.
				_, err = s.Claim(ctx, 42, "ws1")
				require.NoError(t, err)
				_, err = s.Claim(ctx, 42, "ws2")
				require.ErrorIs(t, err, bstore.ErrForgeInstallationClaimed)

				inst, err = s.Get(ctx, 42)
				require.NoError(t, err)
				assert.Equal(t, "ws1", inst.WorkspaceID, "a refused claim changes nothing")
			})

			t.Run("claim creates the row when the webhook has not landed", func(t *testing.T) {
				s := open(t)
				ctx := context.Background()

				// The redirect and the delivery race; the claim must not depend
				// on having lost that race.
				inst, err := s.Claim(ctx, 77, "ws1")
				require.NoError(t, err)
				assert.Equal(t, "ws1", inst.WorkspaceID)

				// The delivery arriving afterwards refreshes the account and
				// leaves ownership alone.
				require.NoError(t, s.Record(ctx, 77, "acme"))
				inst, err = s.Get(ctx, 77)
				require.NoError(t, err)
				assert.Equal(t, "acme", inst.Account)
				assert.Equal(t, "ws1", inst.WorkspaceID, "a later delivery must never unbind")
			})

			t.Run("unknown installations are owned by nobody", func(t *testing.T) {
				s := open(t)
				ctx := context.Background()

				_, err := s.Get(ctx, 999)
				require.ErrorIs(t, err, bstore.ErrForgeInstallationNotFound)

				owned, err := s.OwnedBy(ctx, 999, "ws1")
				require.NoError(t, err)
				assert.False(t, owned)

				// A missing workspace context never matches, even against a
				// row whose workspace_id is the empty string.
				require.NoError(t, s.Record(ctx, 999, "acme"))
				owned, err = s.OwnedBy(ctx, 999, "")
				require.NoError(t, err)
				assert.False(t, owned, "an empty workspace must never match an unclaimed row")
			})

			t.Run("list returns only what the workspace owns", func(t *testing.T) {
				s := open(t)
				ctx := context.Background()

				_, err := s.Claim(ctx, 1, "ws1")
				require.NoError(t, err)
				_, err = s.Claim(ctx, 2, "ws2")
				require.NoError(t, err)
				require.NoError(t, s.Record(ctx, 3, "unclaimed"))

				got, err := s.List(ctx, "ws1")
				require.NoError(t, err)
				require.Len(t, got, 1)
				assert.Equal(t, int64(1), got[0].InstallationID)

				got, err = s.List(ctx, "")
				require.NoError(t, err)
				assert.Empty(t, got, "unclaimed rows belong to no workspace and are never listed")
			})

			t.Run("forget revokes, and a re-install starts unclaimed", func(t *testing.T) {
				s := open(t)
				ctx := context.Background()

				_, err := s.Claim(ctx, 42, "ws1")
				require.NoError(t, err)
				require.NoError(t, s.Forget(ctx, 42))

				_, err = s.Get(ctx, 42)
				require.ErrorIs(t, err, bstore.ErrForgeInstallationNotFound)
				owned, err := s.OwnedBy(ctx, 42, "ws1")
				require.NoError(t, err)
				assert.False(t, owned)

				// The same id installed again does not inherit the old claim.
				require.NoError(t, s.Record(ctx, 42, "acme"))
				owned, err = s.OwnedBy(ctx, 42, "ws1")
				require.NoError(t, err)
				assert.False(t, owned)

				// Forgetting what is not there is not an error.
				require.NoError(t, s.Forget(ctx, 12345))
			})

			t.Run("rejects incomplete writes", func(t *testing.T) {
				s := open(t)
				ctx := context.Background()

				require.Error(t, s.Record(ctx, 0, "acme"))
				_, err := s.Claim(ctx, 0, "ws1")
				require.Error(t, err)
				_, err = s.Claim(ctx, 42, "")
				require.Error(t, err)
			})
		})
	}
}
