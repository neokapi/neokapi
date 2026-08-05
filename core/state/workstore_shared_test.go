package state_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/storage"
)

func sharedDB(t *testing.T) *storage.DB {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "store.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestOpenWorkFromDB_SameBehaviourAsItsOwnFile(t *testing.T) {
	db := sharedDB(t)
	committed := filepath.Join(t.TempDir(), "units")

	w, err := state.OpenWorkFromDB(t.Context(), db, committed)
	require.NoError(t, err)

	var ledgered int
	require.NoError(t, db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM state`).Scan(&ledgered))
	assert.Positive(t, ledgered, "the shared database carries the work ledger under its own name")

	require.NoError(t, w.Put(t.Context(), unit("u1", "d-intro", "Alpha")))
	require.NoError(t, w.Commit(t.Context()))

	onDisk, err := state.ReadCommitted(committed)
	require.NoError(t, err)
	require.Len(t, onDisk, 1)
	assert.Equal(t, "u1", onDisk[0].Unit)
}

// An adopted working store seeds from the committed record just as one that
// opened its own file does — the working set is an index over that record
// however it is stored.
func TestOpenWorkFromDB_SeedsFromCommitted(t *testing.T) {
	committed := filepath.Join(t.TempDir(), "units")
	require.NoError(t, state.WriteCommitted(committed, []state.UnitState{
		unit("u1", "d-intro", "Alpha"),
		unit("u2", "d-intro", "Bravo"),
	}))

	w, err := state.OpenWorkFromDB(t.Context(), sharedDB(t), committed)
	require.NoError(t, err)

	all, err := w.All(t.Context())
	require.NoError(t, err)
	assert.Len(t, all, 2)

	pending, err := w.Pending(t.Context())
	require.NoError(t, err)
	assert.Zero(t, pending, "a re-derived unit is not a staged decision")
}

// Closing an adopted store must leave the pool open for the subsystems sharing
// it.
func TestOpenWorkFromDB_CloseDoesNotCloseAdoptedPool(t *testing.T) {
	db := sharedDB(t)
	w, err := state.OpenWorkFromDB(t.Context(), db, filepath.Join(t.TempDir(), "units"))
	require.NoError(t, err)

	require.NoError(t, w.Close())

	var n int
	assert.NoError(t, db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM unit_state`).Scan(&n),
		"the pool is still usable after the adopting store closed")
}

func TestOpenWorkFromDB_RejectsNilDatabase(t *testing.T) {
	_, err := state.OpenWorkFromDB(t.Context(), nil, t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil database")
}

// Staged is the set Pending counts. It exists so a working store being replaced
// can hand over the decisions no committed source can reproduce.
func TestStaged_ReturnsExactlyWhatPendingCounts(t *testing.T) {
	committed := filepath.Join(t.TempDir(), "units")
	require.NoError(t, state.WriteCommitted(committed, []state.UnitState{
		unit("u-seeded", "d-intro", "Seeded"),
	}))

	for _, tc := range []struct {
		name string
		open func(*testing.T) *state.WorkStore
	}{
		{
			name: "database",
			open: func(t *testing.T) *state.WorkStore {
				w, err := state.OpenWork(t.Context(), filepath.Join(t.TempDir(), "work", "state.db"), committed)
				require.NoError(t, err)
				return w
			},
		},
		{
			name: "sidecar",
			open: func(t *testing.T) *state.WorkStore {
				w, err := state.OpenWorkSidecar(t.Context(), filepath.Join(t.TempDir(), "store.json"), committed)
				require.NoError(t, err)
				return w
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := tc.open(t)
			t.Cleanup(func() { _ = w.Close() })

			staged, err := w.Staged(t.Context())
			require.NoError(t, err)
			assert.Empty(t, staged, "a seeded unit is not staged")

			require.NoError(t, w.Put(t.Context(), unit("u-decided", "d-intro", "Decided")))

			staged, err = w.Staged(t.Context())
			require.NoError(t, err)
			require.Len(t, staged, 1)
			assert.Equal(t, "u-decided", staged[0].Unit)

			pending, err := w.Pending(t.Context())
			require.NoError(t, err)
			assert.Equal(t, len(staged), pending)

			require.NoError(t, w.Commit(t.Context()))
			staged, err = w.Staged(t.Context())
			require.NoError(t, err)
			assert.Empty(t, staged, "committing clears the staged set")
		})
	}
}

// The sidecar working store is the browser build's, but it is not built out of
// browser-only parts: it persists and reloads on every build, which is what
// lets the predecessor sweep read one.
func TestOpenWorkSidecar_PersistsAndReloads(t *testing.T) {
	dir := t.TempDir()
	sidecar := filepath.Join(dir, "store.json")
	committed := filepath.Join(dir, "units")

	first, err := state.OpenWorkSidecar(t.Context(), sidecar, committed)
	require.NoError(t, err)
	require.NoError(t, first.Put(t.Context(), unit("u1", "d-intro", "Alpha")))
	require.NoError(t, first.Close())
	require.FileExists(t, sidecar)

	second, err := state.OpenWorkSidecar(t.Context(), sidecar, committed)
	require.NoError(t, err)
	t.Cleanup(func() { _ = second.Close() })

	staged, err := second.Staged(t.Context())
	require.NoError(t, err)
	require.Len(t, staged, 1)
	assert.Equal(t, "u1", staged[0].Unit)
}
