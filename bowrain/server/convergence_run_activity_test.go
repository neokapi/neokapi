package server

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/event"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
)

// The terminal run frame is what the activity recorder reads, and the recorder
// files every entry under the workspace SLUG. A frame that named only the
// project could not be filed at all, so this pins what the orchestrator now
// publishes: the workspace, and the run's outcome folded out of its standing.

// runFrameHarness wires a Server with a content store, a run store, an event
// bus and a scripted auth store — enough to drive a run to a terminal state and
// read the frame it announces.
func runFrameHarness(t *testing.T) (*Server, *bstore.ConvergenceRunStore, *frameCollector) {
	t.Helper()
	cs, err := sqlitestore.NewSQLiteStore(filepath.Join(t.TempDir(), "run-frame.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	require.NoError(t, cs.CreateProject(t.Context(), &platstore.Project{
		ID: "proj-1", Name: "proj-1", WorkspaceID: srcWSID,
	}))

	bus := event.NewChannelEventBus()
	t.Cleanup(bus.Close)
	frames := &frameCollector{}
	bus.SubscribeGroup("run-frame-test", frames.observe)

	runStore := bstore.NewConvergenceRunStore(cs.DB())
	s := &Server{
		ConvergenceRunStore: runStore,
		ContentStore:        cs,
		EventBus:            bus,
		AuthStore:           newSrcAuthStore(),
	}
	s.convergence = newConvergenceOrchestrator(s)
	return s, runStore, frames
}

// frameCollector keeps the convergence.run.completed frames the bus delivers.
type frameCollector struct {
	mu     sync.Mutex
	frames []platev.Event
}

func (f *frameCollector) observe(ev platev.Event) error {
	if ev.Type != platev.EventConvergenceRunCompleted {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.frames = append(f.frames, ev)
	return nil
}

func (f *frameCollector) await(t *testing.T) platev.Event {
	t.Helper()
	var got platev.Event
	require.Eventually(t, func() bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		if len(f.frames) == 0 {
			return false
		}
		got = f.frames[0]
		return true
	}, 2*time.Second, 10*time.Millisecond, "the run must announce its terminal state")
	return got
}

func TestConvergenceRunFrame_CarriesWorkspaceAndOutcome(t *testing.T) {
	s, runStore, frames := runFrameHarness(t)
	ctx := context.Background()
	run := &bstore.ConvergenceRun{ProjectID: "proj-1", Trigger: "cli", State: bstore.ConvergenceRunRunning}
	require.NoError(t, runStore.CreateRun(ctx, run))

	s.convergence.driveWith(ctx, run, convergingFuncs([]string{"fr-FR", "de-DE"}, 10))

	ev := frames.await(t)
	assert.Equal(t, srcWSSlug, ev.Data["workspace_slug"],
		"without the slug the activity recorder files the run under no workspace")
	assert.Equal(t, srcWSID, ev.Data["workspace_id"], "the id is the recorder's fallback")
	assert.Equal(t, run.ID, ev.Data["run_id"])
	assert.Equal(t, bstore.ConvergenceRunConverged, ev.Data["state"])
	assert.Equal(t, "2", ev.Data["locales_count"])
	assert.Equal(t, "20", ev.Data["produced_count"], "both locales produced ten units each")
	assert.ElementsMatch(t, []string{"fr-FR", "de-DE"},
		strings.Split(ev.Data["locales"], ","), "the locale list rides the frame for drill-down")
}

// A run that never reached a locale still announces itself; the counts simply
// report nothing worked, which is the honest reading of a stalled run.
func TestConvergenceRunFrame_ParkedRunStillAnnounces(t *testing.T) {
	s, runStore, frames := runFrameHarness(t)
	ctx := context.Background()
	run := &bstore.ConvergenceRun{ProjectID: "proj-1", Trigger: "cli", State: bstore.ConvergenceRunRunning}
	require.NoError(t, runStore.CreateRun(ctx, run))

	s.convergence.driveWith(ctx, run, stallingFuncs([]string{"fr-FR"}, 10))

	ev := frames.await(t)
	assert.Equal(t, srcWSSlug, ev.Data["workspace_slug"])
	assert.Equal(t, bstore.ConvergenceRunParked, ev.Data["state"])
	assert.Equal(t, "0", ev.Data["produced_count"])
}
