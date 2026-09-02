package event

import (
	"testing"

	"github.com/stretchr/testify/assert"

	platev "github.com/neokapi/neokapi/bowrain/core/event"
	bstore "github.com/neokapi/neokapi/bowrain/store"
)

// A finished convergence run is the loop's own headline in the feed. It takes
// both a recorder case for convergence.run.completed and a workspace on the
// event: the orchestrator's frame names only the project, so a mapped event
// with no workspace lands under none.

func TestMapEventToActivity_ConvergenceRunCompleted(t *testing.T) {
	r := &ActivityRecorder{}
	a := r.mapEventToActivity(platev.Event{
		Type:      platev.EventConvergenceRunCompleted,
		ProjectID: "proj-1",
		Data: map[string]string{
			"workspace_slug": "acme",
			"workspace_id":   "ws-1",
			"run_id":         "run-7",
			"state":          "converged",
			"passes":         "2",
			"locales":        "fr,de,ja",
			"locales_count":  "3",
			"produced_count": "128",
		},
	})
	if assert.NotNil(t, a, "a completed run must reach the feed") {
		assert.Equal(t, "acme", a.WorkspaceID, "the recorder files on the slug the event now carries")
		assert.Equal(t, bstore.ActivityFlowCompleted, a.Type)
		assert.Equal(t, "run", a.EntityType)
		assert.Equal(t, "run-7", a.EntityID)
		assert.Equal(t, "convergence run converged: 3 languages · 128 translations produced", a.Summary)
		assert.Equal(t, "system", a.ActorID, "a run has no human actor")
		// The locale list rides the data for drill-down; the summary counts it.
		assert.Equal(t, "fr,de,ja", a.Data["locales"])
		assert.NotContains(t, a.Summary, "fr,de,ja")
	}
}

// The workspace id is the fallback the recorder already applies, so a run whose
// slug could not be resolved still lands somewhere rather than nowhere.
func TestMapEventToActivity_ConvergenceRunFallsBackToWorkspaceID(t *testing.T) {
	r := &ActivityRecorder{}
	a := r.mapEventToActivity(platev.Event{
		Type:      platev.EventConvergenceRunCompleted,
		ProjectID: "proj-1",
		Data: map[string]string{
			"workspace_id": "ws-1",
			"run_id":       "run-8",
			"state":        "converged",
		},
	})
	if assert.NotNil(t, a) {
		assert.Equal(t, "ws-1", a.WorkspaceID)
	}
}

// A failed run is the entry most worth scanning for, so it maps too — as a
// failure, not as a completion.
func TestMapEventToActivity_ConvergenceRunFailed(t *testing.T) {
	r := &ActivityRecorder{}
	a := r.mapEventToActivity(platev.Event{
		Type:      platev.EventConvergenceRunCompleted,
		ProjectID: "proj-1",
		Data: map[string]string{
			"workspace_slug": "acme",
			"run_id":         "run-9",
			"state":          "failed",
		},
	})
	if assert.NotNil(t, a) {
		assert.Equal(t, bstore.ActivityFlowFailed, a.Type)
		assert.Equal(t, "convergence run failed", a.Summary)
	}
}

func TestConvergenceRunSummary(t *testing.T) {
	cases := []struct {
		name string
		data map[string]string
		want string
	}{
		{
			name: "converged with work done",
			data: map[string]string{"state": "converged", "locales_count": "3", "produced_count": "128"},
			want: "convergence run converged: 3 languages · 128 translations produced",
		},
		{
			name: "singular language and translation",
			data: map[string]string{"state": "converged", "locales_count": "1", "produced_count": "1"},
			want: "convergence run converged: 1 language · 1 translation produced",
		},
		{
			name: "converged with nothing new to produce",
			data: map[string]string{"state": "converged", "locales_count": "2", "produced_count": "0"},
			want: "convergence run converged: 2 languages · 0 translations produced",
		},
		{
			name: "parked names the reason it stopped",
			data: map[string]string{"state": "parked", "stall_reason": "needs_credits", "locales_count": "2", "produced_count": "4"},
			want: "convergence run parked (needs_credits): 2 languages · 4 translations produced",
		},
		{
			name: "held on source before any locale was worked",
			data: map[string]string{"state": "parked", "stall_reason": "source_not_ready"},
			want: "convergence run parked (source_not_ready)",
		},
		{
			name: "large counts are grouped, never enumerated",
			data: map[string]string{"state": "converged", "locales_count": "12", "produced_count": "20345"},
			want: "convergence run converged: 12 languages · 20,345 translations produced",
		},
		{
			name: "an event with no state still reads as an outcome",
			data: map[string]string{},
			want: "convergence run finished",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, convergenceRunSummary(tc.data))
		})
	}
}
