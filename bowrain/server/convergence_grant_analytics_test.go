package server

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/neokapi/neokapi/bowrain/analytics"
	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/jobs"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file covers the grant-sizing analytics (how large must the one-time
// new-workspace grant be?): the cold-start `first_run` marker and the bucketed
// magnitude carried on the estimate and run events.

// grantAnalyticsHarness wires a Server with a real SQLite ContentStore and
// ConvergenceRunStore — enough for the workspace-scoped first-run probe.
func grantAnalyticsHarness(t *testing.T) (*Server, *sqlitestore.SQLiteStore, *bstore.ConvergenceRunStore) {
	t.Helper()
	cs, err := sqlitestore.NewSQLiteStore(filepath.Join(t.TempDir(), "grant.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.Close() })
	runStore := bstore.NewConvergenceRunStore(cs.DB())
	s := &Server{ContentStore: cs, ConvergenceRunStore: runStore}
	s.convergence = newConvergenceOrchestrator(s)
	return s, cs, runStore
}

func mkWorkspaceProject(t *testing.T, cs *sqlitestore.SQLiteStore, id, workspaceID string) {
	t.Helper()
	require.NoError(t, cs.CreateProject(t.Context(), &platstore.Project{
		ID: id, Name: id, WorkspaceID: workspaceID,
		DefaultSourceLanguage: model.LocaleEnglish,
		TargetLanguages:       []model.LocaleID{model.LocaleFrench},
	}))
}

// TestIsFirstConvergenceRun: the workspace's first run reads true, every later
// one false — including a first run on a SECOND project of the same workspace,
// which is no longer cold-start.
func TestIsFirstConvergenceRun(t *testing.T) {
	s, cs, runStore := grantAnalyticsHarness(t)
	mkWorkspaceProject(t, cs, "p1", "ws1")
	mkWorkspaceProject(t, cs, "p2", "ws1")
	mkWorkspaceProject(t, cs, "other", "ws2")
	ctx := t.Context()

	// Runs are seeded in a terminal state: the store's one-running-run-per-
	// project index would reject a second insert otherwise.
	base := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	done := bstore.ConvergenceRunConverged

	// Before anything ran, a pre-flight on p1 is a cold start.
	assert.True(t, s.isFirstConvergenceRun(ctx, "ws1", "p1", base, ""))

	first := &bstore.ConvergenceRun{ProjectID: "p1", Trigger: "manual", State: done, CreatedAt: base}
	require.NoError(t, runStore.CreateRun(ctx, first))
	assert.True(t, s.isFirstConvergenceRun(ctx, "ws1", "p1", first.CreatedAt, first.ID),
		"the workspace's first run is the cold-start cohort")

	// A second run on the same project.
	second := &bstore.ConvergenceRun{
		ProjectID: "p1", Trigger: "push", State: done, CreatedAt: base.Add(time.Hour)}
	require.NoError(t, runStore.CreateRun(ctx, second))
	assert.False(t, s.isFirstConvergenceRun(ctx, "ws1", "p1", second.CreatedAt, second.ID))

	// The first run's answer is stable at its terminal state, minutes later.
	assert.True(t, s.isFirstConvergenceRun(ctx, "ws1", "p1", first.CreatedAt, first.ID))

	// A different PROJECT in the same workspace is not a cold start.
	sibling := &bstore.ConvergenceRun{
		ProjectID: "p2", Trigger: "manual", State: done, CreatedAt: base.Add(2 * time.Hour)}
	require.NoError(t, runStore.CreateRun(ctx, sibling))
	assert.False(t, s.isFirstConvergenceRun(ctx, "ws1", "p2", sibling.CreatedAt, sibling.ID),
		"first_run is workspace-scoped, not project-scoped")

	// Another workspace is unaffected by ws1's history.
	otherFirst := &bstore.ConvergenceRun{
		ProjectID: "other", Trigger: "manual", State: done, CreatedAt: base.Add(3 * time.Hour)}
	require.NoError(t, runStore.CreateRun(ctx, otherFirst))
	assert.True(t, s.isFirstConvergenceRun(ctx, "ws2", "other", otherFirst.CreatedAt, otherFirst.ID))

	// No workspace (anonymous/self-hosted project): the probe degrades to
	// project scope rather than reporting every run as first.
	assert.False(t, s.isFirstConvergenceRun(ctx, "", "p1", second.CreatedAt, second.ID))

	// No run store configured: never claims a cold start.
	assert.False(t, (&Server{}).isFirstConvergenceRun(ctx, "ws1", "p1", time.Now().UTC(), ""))
}

// TestRunCompletedProps asserts the terminal event's grant-sizing payload:
// realized spend and TM share are bucketed, and the cohort marker rides along.
func TestRunCompletedProps(t *testing.T) {
	created := time.Date(2026, 7, 1, 10, 0, 0, 0, time.UTC)
	finished := created.Add(90 * time.Second)
	run := &bstore.ConvergenceRun{
		ID: "r1", ProjectID: "p1", State: bstore.ConvergenceRunConverged,
		Passes: 2, CreatedAt: created, FinishedAt: &finished,
	}
	standing := convergence.NewStanding()
	standing.Observe(convergence.Event{
		Type: convergence.EventUnitProgress, Locale: "fr-FR", Done: 100, ViaTM: 75, ViaAI: 25,
	})

	// 60_000 tokens ≈ 60_000 credits → the 50k–100k band, below the 200K grant.
	props := runCompletedProps(run, standing, "ws1", 60_000, true)
	assert.Equal(t, "ws1", props[analytics.PropWorkspaceID])
	assert.Equal(t, "p1", props[analytics.PropProjectID])
	assert.Equal(t, "50k_100k", props[analytics.PropConsumedCreditsBucket])
	assert.Equal(t, "51_75", props[analytics.PropTMLeveragePctBucket])
	assert.Equal(t, true, props[analytics.PropFirstRun])
	// The pre-existing taxonomy is untouched.
	assert.Equal(t, "11_100", props["via_tm"])
	assert.Equal(t, "11_100", props["via_ai"])
	assert.Equal(t, "30s_2m", props["duration_bucket"])
	assert.Equal(t, bstore.ConvergenceRunConverged, props["outcome"])

	// A TM-only run spends nothing and reports full leverage.
	tmOnly := convergence.NewStanding()
	tmOnly.Observe(convergence.Event{
		Type: convergence.EventUnitProgress, Locale: "fr-FR", Done: 40, ViaTM: 40, ViaAI: 0,
	})
	props = runCompletedProps(run, tmOnly, "ws1", 0, false)
	assert.Equal(t, "0", props[analytics.PropConsumedCreditsBucket])
	assert.Equal(t, "100", props[analytics.PropTMLeveragePctBucket])
	assert.Equal(t, false, props[analytics.PropFirstRun])

	// No magnitude is ever exact: the payload carries no raw credit/token value.
	for k, v := range props {
		if k == "passes" {
			continue // a pass count is neither content nor a size
		}
		_, isInt := v.(int)
		assert.False(t, isInt, "property %q must be bucketed, not a raw number", k)
	}
}

// TestRunTokenAccumulation: per-pass token usage sums onto the run and is read
// exactly once at the terminal state.
func TestRunTokenAccumulation(t *testing.T) {
	s, _, _ := grantAnalyticsHarness(t)
	o := s.convergence
	o.addRunTokens("r1", 400)
	o.addRunTokens("r1", 600)
	o.addRunTokens("r2", 10)
	o.addRunTokens("", 999)  // no run in scope: dropped
	o.addRunTokens("r1", -5) // never subtracts
	assert.Equal(t, 1000, o.takeRunTokens("r1"))
	assert.Equal(t, 0, o.takeRunTokens("r1"), "the entry is freed after the read")
	assert.Equal(t, 10, o.takeRunTokens("r2"))
}

// TestEstimateComputedProps asserts the pre-flight event's payload: the
// uncensored demand, bucketed, with the cohort marker.
func TestEstimateComputedProps(t *testing.T) {
	proj := &platstore.Project{ID: "p1", WorkspaceID: "ws1"}
	view := convergenceEstimateView{
		Source: jobs.SourceReadiness{Gate: model.SourceGateChecked, Total: 14, Ready: 10, Held: 4},
		Totals: jobs.EstimateTotals{Pending: 50, ViaTM: 10, ViaAI: 40, TokenEstimate: 250_000},
		Credits: &convergenceCreditsView{
			EstimatedCredits: 250_000, Balance: 200_000, CoversAllAI: false,
		},
	}
	props := estimateComputedProps(proj, view, true)
	assert.Equal(t, "ws1", props[analytics.PropWorkspaceID])
	assert.Equal(t, true, props[analytics.PropSourceHeld])
	// 250K credits of demand does NOT fit the 200K grant — the band above it.
	assert.Equal(t, "200k_500k", props[analytics.PropEstimatedCreditsBucket])
	assert.Equal(t, "11_100", props[analytics.PropAIUnitsBucket])
	assert.Equal(t, "1_25", props[analytics.PropTMLeveragePctBucket])
	assert.Equal(t, false, props[analytics.PropCoversAllAI])
	assert.Equal(t, true, props[analytics.PropFirstRun])

	// Self-hosted (billing unconfigured): demand is still reported, coverage is
	// simply absent rather than a misleading "covered".
	view.Credits = nil
	props = estimateComputedProps(proj, view, false)
	assert.NotContains(t, props, analytics.PropCoversAllAI)
	assert.Equal(t, "200k_500k", props[analytics.PropEstimatedCreditsBucket])
}
