package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"github.com/labstack/echo/v4"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/model"
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
// progress (e.g. every unit fails a bound check), so the loop parks the
// remainder. It reports FailingChecks so the run-standing propagation is
// exercised.
func stallingFuncs(locales []string, total int) convergence.LoopFuncs {
	return convergence.LoopFuncs{
		Derive: func(ctx context.Context) (convergence.PassState, error) {
			st := convergence.PassState{UnitTotals: map[string]int{}, FailingChecks: len(locales)}
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

// TestOrchestrator_DriveStallsOnCredits asserts a credit refusal produces a
// PARKED run carrying stall_reason=needs_credits + a human message — NOT a
// silent, reasonless park (strategy 2026-07-dogfood doc 06, theme C). The
// terminal done frame must carry the same stall_reason so the UI/SSE can label
// it, and the reason surfaces through the run view.
func TestOrchestrator_DriveStallsOnCredits(t *testing.T) {
	s, runStore := newOrchestratorHarness(t)
	ctx := context.Background()
	run := &bstore.ConvergenceRun{ProjectID: "proj-1", Trigger: "push", State: bstore.ConvergenceRunRunning}
	require.NoError(t, runStore.CreateRun(ctx, run))

	// Produce refuses with the typed credit sentinel — exactly what the server
	// produceFunc propagates when createTranslationJobs hits a zero-credit
	// workspace.
	funcs := convergence.LoopFuncs{
		Derive: func(context.Context) (convergence.PassState, error) {
			return convergence.PassState{
				Pending:    []string{"fr-FR"},
				UnitTotals: map[string]int{"fr-FR": 10},
			}, nil
		},
		Produce: func(context.Context, string, int, *convergence.Emitter) (int, int, int, error) {
			return 0, 0, 0, errStallNeedsCredits
		},
	}
	s.convergence.driveWith(ctx, run, funcs)

	got, err := runStore.GetRun(ctx, run.ID)
	require.NoError(t, err)
	// Parked (work saved), NOT failed — and labeled.
	assert.Equal(t, bstore.ConvergenceRunParked, got.State)
	assert.Equal(t, convergence.StallNeedsCredits, got.StallReason)
	assert.NotEmpty(t, got.Error, "a labeled stall carries a human message")
	// The reason surfaces through the REST view.
	view := toConvergenceRunView(got)
	assert.Equal(t, "needs_credits", view.StallReason)

	// The terminal done frame carries the stall_reason so the SSE/UI can label
	// the stall without a second round-trip.
	_, payloads, err := runStore.ListEvents(ctx, run.ID, 0)
	require.NoError(t, err)
	require.NotEmpty(t, payloads)
	var probe struct {
		Type        string `json:"type"`
		State       string `json:"state"`
		StallReason string `json:"stallReason"`
	}
	require.NoError(t, json.Unmarshal(payloads[len(payloads)-1], &probe))
	assert.Equal(t, "done", probe.Type)
	assert.Equal(t, "parked", probe.State)
	assert.Equal(t, "needs_credits", probe.StallReason)
}

// TestOrchestrator_RunContextColumns asserts the loop-observability columns
// populate as a run progresses and surface through the run view (theme D): the
// heartbeat (last_activity) advances, and a terminal run freezes its live
// position (current_stage/current_locale) so the row does not read as still
// producing.
func TestOrchestrator_RunContextColumns(t *testing.T) {
	s, runStore := newOrchestratorHarness(t)
	ctx := context.Background()
	run := &bstore.ConvergenceRun{ProjectID: "proj-1", Trigger: "cli", State: bstore.ConvergenceRunRunning}
	require.NoError(t, runStore.CreateRun(ctx, run))

	var mu sync.Mutex
	produced := map[string]int{}
	funcs := convergence.LoopFuncs{
		Derive: func(context.Context) (convergence.PassState, error) {
			mu.Lock()
			defer mu.Unlock()
			st := convergence.PassState{UnitTotals: map[string]int{"fr-FR": 5}, Produced: produced["fr-FR"]}
			if produced["fr-FR"] < 5 {
				st.Pending = []string{"fr-FR"}
			}
			return st, nil
		},
		Produce: func(ctx context.Context, locale string, pass int, emit *convergence.Emitter) (int, int, int, error) {
			// An ai_translate-staged progress event: the emitter records the
			// stage/locale/heartbeat on the row.
			emit.Emit(convergence.Event{
				Type: convergence.EventUnitProgress, Stage: convergence.StageAITranslate,
				Pass: pass, Locale: locale, Done: 5, ViaAI: 5,
			})
			mu.Lock()
			produced[locale] = 5
			mu.Unlock()
			return 5, 0, 5, nil
		},
	}
	s.convergence.driveWith(ctx, run, funcs)

	got, err := runStore.GetRun(ctx, run.ID)
	require.NoError(t, err)
	assert.Equal(t, bstore.ConvergenceRunConverged, got.State)
	// last_activity populated from the run's progress and surfaces via the view.
	require.NotNil(t, got.LastActivity)
	assert.False(t, got.LastActivity.IsZero())
	assert.NotEmpty(t, toConvergenceRunView(got).LastActivity)
	// A terminal run has stopped moving: the live position is frozen.
	assert.Empty(t, got.CurrentStage)
	assert.Empty(t, got.CurrentLocale)
}

// TestOrchestrator_ParkLabelsReason asserts an ordinary park (no typed stall
// error) still gets a machine-readable reason: checks_failing when bound checks
// demoted units, else no_progress. This distinguishes "real pending review"
// from an out-of-credits block on the same parked state (theme C).
func TestOrchestrator_ParkLabelsReason(t *testing.T) {
	s, runStore := newOrchestratorHarness(t)
	ctx := context.Background()
	run := &bstore.ConvergenceRun{ProjectID: "proj-1", Trigger: "push", State: bstore.ConvergenceRunRunning}
	require.NoError(t, runStore.CreateRun(ctx, run))

	// stallingFuncs reports FailingChecks each pass, so the park is labeled
	// checks_failing.
	s.convergence.driveWith(ctx, run, stallingFuncs([]string{"fr-FR"}, 10))

	got, err := runStore.GetRun(ctx, run.ID)
	require.NoError(t, err)
	assert.Equal(t, bstore.ConvergenceRunParked, got.State)
	assert.Equal(t, convergence.StallChecksFailing, got.StallReason)
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
	// FailingChecks derived each pass is carried into the run standing + JSON.
	assert.Equal(t, 1, got.FailingChecks)
	assert.Equal(t, 1, toConvergenceRunView(got).FailingChecks)
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
	assert.Len(t, live, len(persisted))
}

func TestOrchestrator_DriveFailed(t *testing.T) {
	s, runStore := newOrchestratorHarness(t)
	ctx := context.Background()
	run := &bstore.ConvergenceRun{ProjectID: "proj-1", Trigger: "cli", State: bstore.ConvergenceRunRunning}
	require.NoError(t, runStore.CreateRun(ctx, run))

	// Derive fails (e.g. a provider outage): the run must end failed, not parked,
	// and the terminal done frame must carry state "failed" so a caller does not
	// exit 0 as normal parked completion (F2).
	funcs := convergence.LoopFuncs{
		Derive: func(context.Context) (convergence.PassState, error) {
			return convergence.PassState{}, errors.New("provider down")
		},
		Produce: func(context.Context, string, int, *convergence.Emitter) (int, int, int, error) {
			return 0, 0, 0, nil
		},
	}
	s.convergence.driveWith(ctx, run, funcs)

	got, err := runStore.GetRun(ctx, run.ID)
	require.NoError(t, err)
	assert.Equal(t, bstore.ConvergenceRunFailed, got.State)
	assert.Contains(t, got.Error, "provider down")
	// The failure cause surfaces in the run view so `kapi up` can print it.
	assert.Contains(t, toConvergenceRunView(got).Error, "provider down")

	_, payloads, err := runStore.ListEvents(ctx, run.ID, 0)
	require.NoError(t, err)
	require.NotEmpty(t, payloads)
	var probe struct {
		Type  string `json:"type"`
		State string `json:"state"`
	}
	require.NoError(t, json.Unmarshal(payloads[len(payloads)-1], &probe))
	assert.Equal(t, "done", probe.Type)
	assert.Equal(t, "failed", probe.State)
}

func TestRunForProject_IDOR(t *testing.T) {
	cs, err := sqlitestore.NewSQLiteStore(filepath.Join(t.TempDir(), "runs.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	ctx := context.Background()
	require.NoError(t, cs.CreateProject(ctx, &platstore.Project{ID: "proj-a", Name: "a"}))
	require.NoError(t, cs.CreateProject(ctx, &platstore.Project{ID: "proj-b", Name: "b"}))

	rs := bstore.NewConvergenceRunStore(cs.DB())
	runB := &bstore.ConvergenceRun{ProjectID: "proj-b", Trigger: "cli"}
	require.NoError(t, rs.CreateRun(ctx, runB))
	s := &Server{ConvergenceRunStore: rs}

	newCtx := func(projectID, runID string) echo.Context {
		e := echo.New()
		c := e.NewContext(httptest.NewRequest(http.MethodGet, "/", nil), httptest.NewRecorder())
		c.SetParamNames("id", "runID")
		c.SetParamValues(projectID, runID)
		return c
	}

	// A user authorized on project A cannot reach project B's run (F1): the
	// cross-project lookup reports not-found, never leaks the run.
	_, ok := s.runForProject(newCtx("proj-a", runB.ID))
	assert.False(t, ok, "run from another project must not resolve")

	// The owning project resolves it.
	got, ok := s.runForProject(newCtx("proj-b", runB.ID))
	require.True(t, ok)
	assert.Equal(t, runB.ID, got.ID)
}

func TestResumeSeq(t *testing.T) {
	newCtx := func(header, after string) echo.Context {
		e := echo.New()
		req := httptest.NewRequest(http.MethodGet, "/?after="+after, nil)
		if header != "" {
			req.Header.Set("Last-Event-ID", header)
		}
		return e.NewContext(req, httptest.NewRecorder())
	}
	assert.Equal(t, -1, resumeSeq(newCtx("", "")), "absent → from start")
	assert.Equal(t, 7, resumeSeq(newCtx("7", "")), "Last-Event-ID honored")
	assert.Equal(t, 3, resumeSeq(newCtx("", "3")), "?after fallback")
	assert.Equal(t, 7, resumeSeq(newCtx("7", "3")), "header wins over ?after")
	assert.Equal(t, -1, resumeSeq(newCtx("nope", "")), "invalid → from start")
}

func TestCountFailingBlocks(t *testing.T) {
	// A translated block that drops a source placeholder fails QA (an
	// error-severity finding); a clean block and a non-translatable block do not
	// count (F6).
	fr := model.LocaleFrench
	dropped := &model.Block{
		ID:           "b1",
		Translatable: true,
		Source:       []model.Run{{Text: &model.TextRun{Text: "Hello "}}, {Ph: &model.PlaceholderRun{ID: "1", Disp: "{name}"}}},
		Targets: map[model.VariantKey]*model.Target{
			model.Variant(fr): {Runs: []model.Run{{Text: &model.TextRun{Text: "Bonjour"}}}},
		},
	}
	clean := &model.Block{
		ID:           "b2",
		Translatable: true,
		Source:       []model.Run{{Text: &model.TextRun{Text: "Yes"}}},
		Targets: map[model.VariantKey]*model.Target{
			model.Variant(fr): {Runs: []model.Run{{Text: &model.TextRun{Text: "Oui"}}}},
		},
	}
	nonTranslatable := &model.Block{ID: "b3", Translatable: false}

	blocks := []*platstore.StoredBlock{{Block: dropped}, {Block: clean}, {Block: nonTranslatable}}
	assert.Equal(t, 1, countFailingBlocks(t.Context(), blocks, fr))
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
