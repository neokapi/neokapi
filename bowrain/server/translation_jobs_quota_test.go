package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/core/convergence"
)

// A workspace over its AI usage limit gets no new translation jobs. The worker
// would refuse each one on arrival and each refusal would be announced as a
// failure, so production refuses once, with a stall the run parks on. The job
// store and queue are no-op fakes that panic if reached: the refusal must come
// before either is touched.
func TestCreateTranslationJobsRefusesAWorkspaceOverItsUsageLimit(t *testing.T) {
	s := &Server{
		JobStore:   noopJobStore{},
		JobQueue:   noopQueue{},
		QuotaStore: &meterQuotaStore{remaining: 0},
	}
	proj := &platstore.Project{ID: "proj-1", WorkspaceID: "ws-1"}

	ids, err := s.createTranslationJobs(context.Background(), proj, "main",
		[]string{"en.json", "docs/intro.md"}, []string{"nb", "de"}, "push-1", "acme", "")

	require.ErrorIs(t, err, errStallQuotaExceeded)
	assert.Empty(t, ids)
}

// The refusal parks the run with its own reason and a message that says what
// to do, the way a credit refusal does.
func TestOrchestrator_DriveStallsOnUsageLimit(t *testing.T) {
	s, runStore := newOrchestratorHarness(t)
	ctx := context.Background()
	run := &bstore.ConvergenceRun{ProjectID: "proj-1", Trigger: "push", State: bstore.ConvergenceRunRunning}
	require.NoError(t, runStore.CreateRun(ctx, run))

	s.convergence.driveWith(ctx, run, stallOnProduce(errStallQuotaExceeded))

	got, err := runStore.GetRun(ctx, run.ID)
	require.NoError(t, err)
	assert.Equal(t, bstore.ConvergenceRunParked, got.State)
	assert.Equal(t, StallQuotaExceeded, got.StallReason)
	assert.Contains(t, got.Error, "usage limit")
}

// stallOnProduce models one pending locale whose production refuses with err.
func stallOnProduce(err error) convergence.LoopFuncs {
	return convergence.LoopFuncs{
		Derive: func(context.Context) (convergence.PassState, error) {
			return convergence.PassState{
				Pending:    []string{"fr-FR"},
				UnitTotals: map[string]int{"fr-FR": 10},
			}, nil
		},
		Produce: func(context.Context, string, int, *convergence.Emitter) (convergence.PassProduction, error) {
			return convergence.PassProduction{}, err
		},
	}
}
