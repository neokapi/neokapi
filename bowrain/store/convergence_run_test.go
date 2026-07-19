package store_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/convergence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// projectMaker creates the FK-parent project a run references, on the backend
// under test.
type convergeBackend struct {
	name       string
	db         *sql.DB
	newProject func(t *testing.T, id string)
}

// sqliteConvergeBackend builds a SQLite-backed store + project maker. SQLite is
// always exercised (no container needed).
func sqliteConvergeBackend(t *testing.T) convergeBackend {
	t.Helper()
	s, err := sqlitestore.NewSQLiteStore(filepath.Join(t.TempDir(), "runs.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	return convergeBackend{
		name: "sqlite",
		db:   s.DB(),
		newProject: func(t *testing.T, id string) {
			require.NoError(t, s.CreateProject(t.Context(), &platstore.Project{ID: id, Name: id}))
		},
	}
}

// pgConvergeBackend builds a PostgreSQL-backed store + project maker. Skipped in
// -short mode by pgtest.NewTestDB.
func pgConvergeBackend(t *testing.T) convergeBackend {
	t.Helper()
	db := pgtest.NewTestDB(t)
	s, err := store.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	return convergeBackend{
		name: "postgres",
		db:   db.DB,
		newProject: func(t *testing.T, id string) {
			require.NoError(t, s.CreateProject(t.Context(), &platstore.Project{ID: id, Name: id}))
		},
	}
}

func TestConvergenceRunStore_Lifecycle(t *testing.T) {
	for _, mk := range []func(*testing.T) convergeBackend{sqliteConvergeBackend, pgConvergeBackend} {
		be := mk(t)
		t.Run(be.name, func(t *testing.T) {
			ctx := context.Background()
			be.newProject(t, "proj-1")
			rs := store.NewConvergenceRunStore(be.db)

			run := &store.ConvergenceRun{ProjectID: "proj-1", Trigger: "cli"}
			require.NoError(t, rs.CreateRun(ctx, run))
			assert.NotEmpty(t, run.ID)
			assert.Equal(t, store.ConvergenceRunRunning, run.State)

			// ActiveRun finds the running run.
			active, err := rs.ActiveRun(ctx, "proj-1")
			require.NoError(t, err)
			require.NotNil(t, active)
			assert.Equal(t, run.ID, active.ID)

			// Update to converged with standing.
			run.State = store.ConvergenceRunConverged
			run.Passes = 2
			run.Standing = []store.ConvergenceLocaleStanding{
				{Locale: "fr-FR", State: "shippable", Units: 96, Produced: 96, ViaAI: 60, ViaTM: 36},
				{Locale: "de-DE", State: "parked", Units: 96, Produced: 40},
			}
			now := run.CreatedAt
			run.FinishedAt = &now
			require.NoError(t, rs.UpdateRun(ctx, run))

			got, err := rs.GetRun(ctx, run.ID)
			require.NoError(t, err)
			assert.Equal(t, store.ConvergenceRunConverged, got.State)
			assert.Equal(t, 2, got.Passes)
			require.Len(t, got.Standing, 2)
			assert.Equal(t, "fr-FR", got.Standing[0].Locale)
			assert.Equal(t, 60, got.Standing[0].ViaAI)
			require.NotNil(t, got.FinishedAt)

			// No longer active.
			active, err = rs.ActiveRun(ctx, "proj-1")
			require.NoError(t, err)
			assert.Nil(t, active)

			// List newest-first.
			runs, err := rs.ListRuns(ctx, "proj-1", 10)
			require.NoError(t, err)
			require.Len(t, runs, 1)
		})
	}
}

func TestConvergenceRunStore_GuardAndSweep(t *testing.T) {
	for _, mk := range []func(*testing.T) convergeBackend{sqliteConvergeBackend, pgConvergeBackend} {
		be := mk(t)
		t.Run(be.name, func(t *testing.T) {
			ctx := context.Background()
			be.newProject(t, "proj-1")
			rs := store.NewConvergenceRunStore(be.db)

			// First running run succeeds.
			first := &store.ConvergenceRun{ProjectID: "proj-1", Trigger: "cli"}
			require.NoError(t, rs.CreateRunGuarded(ctx, first))

			// A second running run for the same project is rejected by the
			// partial unique index and mapped to ErrActiveRunExists (F8).
			second := &store.ConvergenceRun{ProjectID: "proj-1", Trigger: "push"}
			require.ErrorIs(t, rs.CreateRunGuarded(ctx, second), store.ErrActiveRunExists)

			// Sweep marks the running run failed (F3), freeing the guard.
			n, err := rs.FailInterruptedRuns(ctx, "interrupted by server restart")
			require.NoError(t, err)
			assert.Equal(t, int64(1), n)
			got, err := rs.GetRun(ctx, first.ID)
			require.NoError(t, err)
			assert.Equal(t, store.ConvergenceRunFailed, got.State)
			assert.Equal(t, "interrupted by server restart", got.Error)
			require.NotNil(t, got.FinishedAt)

			// With no running run, a new run starts cleanly.
			third := &store.ConvergenceRun{ProjectID: "proj-1", Trigger: "cli"}
			require.NoError(t, rs.CreateRunGuarded(ctx, third))
			active, err := rs.ActiveRun(ctx, "proj-1")
			require.NoError(t, err)
			require.NotNil(t, active)
			assert.Equal(t, third.ID, active.ID)
		})
	}
}

func TestConvergenceRunStore_Events(t *testing.T) {
	for _, mk := range []func(*testing.T) convergeBackend{sqliteConvergeBackend, pgConvergeBackend} {
		be := mk(t)
		t.Run(be.name, func(t *testing.T) {
			ctx := context.Background()
			be.newProject(t, "proj-1")
			rs := store.NewConvergenceRunStore(be.db)
			run := &store.ConvergenceRun{ProjectID: "proj-1", Trigger: "push"}
			require.NoError(t, rs.CreateRun(ctx, run))

			payloads := [][]byte{
				[]byte(`{"type":"pass_start","pass":1}`),
				[]byte(`{"type":"locale_done","locale":"fr-FR","done":10}`),
				[]byte(`{"type":"done","state":"converged"}`),
			}
			for i, p := range payloads {
				seq, err := rs.AppendEvent(ctx, run.ID, p)
				require.NoError(t, err)
				assert.Equal(t, i, seq)
			}

			// Replay from 0.
			seqs, got, err := rs.ListEvents(ctx, run.ID, 0)
			require.NoError(t, err)
			require.Len(t, got, 3)
			assert.Equal(t, []int{0, 1, 2}, seqs)
			assert.JSONEq(t, `{"type":"done","state":"converged"}`, string(got[2]))

			// Replay from a cursor.
			_, tail, err := rs.ListEvents(ctx, run.ID, 2)
			require.NoError(t, err)
			require.Len(t, tail, 1)
		})
	}
}

func TestConvergenceRunStore_LatestRunForProjects(t *testing.T) {
	for _, mk := range []func(*testing.T) convergeBackend{sqliteConvergeBackend, pgConvergeBackend} {
		be := mk(t)
		t.Run(be.name, func(t *testing.T) {
			ctx := context.Background()
			rs := store.NewConvergenceRunStore(be.db)

			// No projects → nil, nil (the empty-workspace boundary).
			run, err := rs.LatestRunForProjects(ctx, nil)
			require.NoError(t, err)
			assert.Nil(t, run)

			// Projects without any runs → nil, nil.
			be.newProject(t, "proj-a")
			be.newProject(t, "proj-b")
			be.newProject(t, "proj-c") // never runs
			run, err = rs.LatestRunForProjects(ctx, []string{"proj-a", "proj-b", "proj-c"})
			require.NoError(t, err)
			assert.Nil(t, run)

			// Runs across projects → the newest run wins, regardless of project.
			base := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
			older := &store.ConvergenceRun{
				ProjectID: "proj-a", Trigger: "push",
				State: store.ConvergenceRunConverged, CreatedAt: base,
			}
			require.NoError(t, rs.CreateRun(ctx, older))
			newest := &store.ConvergenceRun{
				ProjectID: "proj-b", Trigger: "manual",
				State: store.ConvergenceRunParked, StallReason: convergence.StallNeedsCredits,
				CreatedAt: base.Add(time.Hour),
			}
			require.NoError(t, rs.CreateRun(ctx, newest))

			got, err := rs.LatestRunForProjects(ctx, []string{"proj-a", "proj-b", "proj-c"})
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, newest.ID, got.ID)
			assert.Equal(t, "proj-b", got.ProjectID)
			assert.Equal(t, store.ConvergenceRunParked, got.State)
			assert.Equal(t, convergence.StallNeedsCredits, got.StallReason)

			// Scoping: a filter that excludes the newest run's project falls
			// back to the newest run among the included projects.
			got, err = rs.LatestRunForProjects(ctx, []string{"proj-a", "proj-c"})
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, older.ID, got.ID)
		})
	}
}
