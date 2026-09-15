package server

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/core/model"
)

// inFlightPushes is a job store whose sync pushes stay in flight until a test
// releases them.
type inFlightPushes struct {
	noopJobStore
	active atomic.Int32
}

func (j *inFlightPushes) CountActivePushApplies(context.Context, string, string) (int, error) {
	return int(j.active.Load()), nil
}

// sourceSettled reports whether settlement has stamped any of the project's
// blocks.
func sourceSettled(cs *sqlitestore.SQLiteStore, projectID string) bool {
	got, err := cs.GetBlocks(context.Background(), platstore.BlockQuery{ProjectID: projectID, Stream: "main"})
	if err != nil {
		return false
	}
	for _, sb := range got {
		if sb.Block != nil && sb.Block.Properties[propSettledHash] != "" {
			return true
		}
	}
	return false
}

// withPushWait shortens the run's push wait for a test.
func withPushWait(t *testing.T, poll, limit time.Duration) {
	t.Helper()
	prevPoll, prevLimit := pushApplyPollInterval, pushApplyWaitLimit
	pushApplyPollInterval, pushApplyWaitLimit = poll, limit
	t.Cleanup(func() { pushApplyPollInterval, pushApplyWaitLimit = prevPoll, prevLimit })
}

// A server run settles source only once the project's in-flight pushes have
// applied. A client starts a run a few seconds after its push is accepted,
// while a large push is still applying; settling then reads blocks the push is
// about to change or remove.
func TestDrive_SettlesSourceOnlyAfterInFlightPushesApply(t *testing.T) {
	s, cs, runStore := sourceFirstHarness(t)
	pushes := &inFlightPushes{}
	pushes.active.Store(1)
	s.JobStore = pushes
	withPushWait(t, 10*time.Millisecond, time.Minute)

	mkProject(t, cs, "p", nil)
	storeSourceBlock(t, cs, "p", "a.json", "b1", "A well-formed sentence.", model.SourceStatusNew)
	run := &bstore.ConvergenceRun{ProjectID: "p", Trigger: "cli", State: bstore.ConvergenceRunRunning}
	require.NoError(t, runStore.CreateRun(t.Context(), run))

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.convergence.driveWith(context.Background(), run, stallingFuncs([]string{"fr"}, 1))
	}()

	require.Never(t, func() bool { return sourceSettled(cs, "p") }, 300*time.Millisecond, 20*time.Millisecond,
		"source is not settled while a push is still applying")
	pushes.active.Store(0)
	require.Eventually(t, func() bool { return sourceSettled(cs, "p") }, 5*time.Second, 20*time.Millisecond,
		"source is settled once the push has applied")
	<-done
}

// A push job left queued, by a worker that stopped, delays a run by the wait
// limit and no longer.
func TestDrive_SettlesSourceAfterTheWaitLimit(t *testing.T) {
	s, cs, runStore := sourceFirstHarness(t)
	pushes := &inFlightPushes{}
	pushes.active.Store(1)
	s.JobStore = pushes
	withPushWait(t, 10*time.Millisecond, 50*time.Millisecond)

	mkProject(t, cs, "p", nil)
	storeSourceBlock(t, cs, "p", "a.json", "b1", "A well-formed sentence.", model.SourceStatusNew)
	run := &bstore.ConvergenceRun{ProjectID: "p", Trigger: "cli", State: bstore.ConvergenceRunRunning}
	require.NoError(t, runStore.CreateRun(t.Context(), run))

	s.convergence.driveWith(t.Context(), run, stallingFuncs([]string{"fr"}, 1))
	require.True(t, sourceSettled(cs, "p"), "source is settled after the wait limit")
}
