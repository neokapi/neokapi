package server

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/core/convergence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newOrchestratorHarness builds a Server with only the convergence run store
// (SQLite) + orchestrator wired — enough to exercise the run lifecycle without
// the full block store / job queue.
func newOrchestratorHarness(t *testing.T) (*Server, *bstore.ConvergenceRunStore) {
	t.Helper()
	cs, err := sqlitestore.NewSQLiteStore(filepath.Join(t.TempDir(), "runs.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	require.NoError(t, cs.CreateProject(t.Context(), &platstore.Project{ID: "proj-1", Name: "proj-1"}))

	runStore := bstore.NewConvergenceRunStore(cs.DB())
	s := &Server{ConvergenceRunStore: runStore}
	s.convergence = newConvergenceOrchestrator(s)
	return s, runStore
}

// convergingFuncs models a project that converges: each pending locale is fully
// produced in a single pass.
func convergingFuncs(locales []string, total int) convergence.LoopFuncs {
	var mu sync.Mutex
	produced := map[string]int{}
	return convergence.LoopFuncs{
		Derive: func(ctx context.Context) (convergence.PassState, error) {
			mu.Lock()
			defer mu.Unlock()
			st := convergence.PassState{UnitTotals: map[string]int{}}
			for _, l := range locales {
				st.UnitTotals[l] = total
				st.Produced += produced[l]
				if produced[l] < total {
					st.Pending = append(st.Pending, l)
				}
			}
			return st, nil
		},
		Produce: func(ctx context.Context, locale string, pass int, emit *convergence.Emitter) (int, int, int, error) {
			mu.Lock()
			produced[locale] = total
			mu.Unlock()
			emit.Emit(convergence.Event{Type: convergence.EventUnitProgress, Pass: pass, Locale: locale, Done: total, ViaAI: total})
			return total, 0, total, nil
		},
	}
}

// stallingFuncs models a project that cannot advance: production makes no
// progress, so the loop parks the remainder.
func stallingFuncs(locales []string, total int) convergence.LoopFuncs {
	return convergence.LoopFuncs{
		Derive: func(ctx context.Context) (convergence.PassState, error) {
			st := convergence.PassState{UnitTotals: map[string]int{}}
			for _, l := range locales {
				st.UnitTotals[l] = total
				st.Pending = append(st.Pending, l)
			}
			return st, nil
		},
		Produce: func(ctx context.Context, locale string, pass int, emit *convergence.Emitter) (int, int, int, error) {
			return 0, 0, 0, nil // no progress
		},
	}
}

func TestOrchestrator_DriveConverges(t *testing.T) {
	s, runStore := newOrchestratorHarness(t)
	ctx := context.Background()
	run := &bstore.ConvergenceRun{ProjectID: "proj-1", Trigger: "cli", State: bstore.ConvergenceRunRunning}
	require.NoError(t, runStore.CreateRun(ctx, run))

	s.convergence.driveWith(ctx, run, convergingFuncs([]string{"fr-FR", "de-DE"}, 10))

	got, err := runStore.GetRun(ctx, run.ID)
	require.NoError(t, err)
	assert.Equal(t, bstore.ConvergenceRunConverged, got.State)
	assert.Equal(t, 1, got.Passes)
	require.NotNil(t, got.FinishedAt)
	require.Len(t, got.Standing, 2)
	for _, ls := range got.Standing {
		assert.Equal(t, convergence.LocaleShippable, ls.State)
		assert.Equal(t, 10, ls.Produced)
	}

	// Every event persisted, terminating in exactly one done frame.
	_, payloads, err := runStore.ListEvents(ctx, run.ID, 0)
	require.NoError(t, err)
	require.NotEmpty(t, payloads)
	assert.True(t, isTerminalEvent(payloads[len(payloads)-1]), "stream ends with done")
	types := convergenceEventTypes(t, payloads)
	assert.Contains(t, types, string(convergence.EventPassStart))
	assert.Contains(t, types, string(convergence.EventLocaleDone))
	assert.Contains(t, types, string(convergence.EventDone))
}

func TestOrchestrator_DriveParks(t *testing.T) {
	s, runStore := newOrchestratorHarness(t)
	ctx := context.Background()
	run := &bstore.ConvergenceRun{ProjectID: "proj-1", Trigger: "push", State: bstore.ConvergenceRunRunning}
	require.NoError(t, runStore.CreateRun(ctx, run))

	s.convergence.driveWith(ctx, run, stallingFuncs([]string{"fr-FR"}, 10))

	got, err := runStore.GetRun(ctx, run.ID)
	require.NoError(t, err)
	assert.Equal(t, bstore.ConvergenceRunParked, got.State)
	require.Len(t, got.Standing, 1)
	assert.Equal(t, convergence.LocaleParked, got.Standing[0].State)
}

func TestOrchestrator_SSEReplayAndLive(t *testing.T) {
	s, runStore := newOrchestratorHarness(t)
	ctx := context.Background()
	run := &bstore.ConvergenceRun{ProjectID: "proj-1", Trigger: "cli", State: bstore.ConvergenceRunRunning}
	require.NoError(t, runStore.CreateRun(ctx, run))

	// Subscribe before driving; collect live frames until the done frame.
	ch := s.convergence.hub.subscribe(run.ID)
	defer s.convergence.hub.unsubscribe(run.ID, ch)

	done := make(chan struct{})
	var live [][]byte
	go func() {
		for f := range ch {
			live = append(live, f.data)
			if isTerminalEvent(f.data) {
				close(done)
				return
			}
		}
	}()

	s.convergence.driveWith(ctx, run, convergingFuncs([]string{"fr-FR"}, 5))
	<-done

	// The live stream and the persisted replay agree in count.
	_, persisted, err := runStore.ListEvents(ctx, run.ID, 0)
	require.NoError(t, err)
	assert.Equal(t, len(persisted), len(live))
}

func convergenceEventTypes(t *testing.T, payloads [][]byte) []string {
	t.Helper()
	var out []string
	for _, p := range payloads {
		var probe struct {
			Type string `json:"type"`
		}
		require.NoError(t, json.Unmarshal(p, &probe))
		out = append(out, probe.Type)
	}
	return out
}
