package convergence_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/convergence"
)

// A pass that only re-stages drafts held for review writes nothing: its
// pass_done reports no produced units and the held drafts on their own,
// and the run's Produced is zero.
func TestLoopReportsHeldDraftsApartFromProduced(t *testing.T) {
	states := []convergence.PassState{
		// Before the first pass: the delivered tree, nothing held.
		{Pending: []string{"nl"}, Produced: 96},
		// After pass 1: the pass drafted 18 units the gate holds.
		{Pending: []string{"nl"}, Produced: 96, WaitingForReview: 18},
		// Before pass 2.
		{Pending: []string{"nl"}, Produced: 96, WaitingForReview: 18},
		// After pass 2: no change, so the loop stops.
		{Pending: []string{"nl"}, Produced: 96, WaitingForReview: 18},
	}
	derived := 0
	var events []convergence.Event
	res, err := convergence.Loop(context.Background(), convergence.LoopOptions{UntilGate: true, MaxPasses: 5},
		convergence.LoopFuncs{
			Derive: func(context.Context) (convergence.PassState, error) {
				s := states[derived]
				derived++
				return s, nil
			},
			Produce: func(context.Context, string, int, *convergence.Emitter) (convergence.PassProduction, error) {
				return convergence.PassProduction{Done: 38, ViaDraft: 18}, nil
			},
		}, convergence.NewEmitter(func(ev convergence.Event) { events = append(events, ev) }))
	require.NoError(t, err)

	assert.Equal(t, 2, res.Passes, "a pass that drafts for review is progress, the next one is not")
	assert.Zero(t, res.Produced, "a run that wrote nothing reports nothing produced")
	assert.Equal(t, 18, res.Final.WaitingForReview)
	var done []convergence.Event
	for _, ev := range events {
		if ev.Type == convergence.EventPassDone {
			done = append(done, ev)
		}
	}
	require.Len(t, done, 2)
	for _, ev := range done {
		assert.Zero(t, ev.ProducedDelta)
		assert.Equal(t, 18, ev.WaitingForReview)
	}
}

// The run's Produced sums what each pass wrote.
func TestLoopSumsProducedAcrossPasses(t *testing.T) {
	states := []convergence.PassState{
		{Pending: []string{"fr"}, Produced: 0},
		{Pending: []string{"fr"}, Produced: 10},
		{Pending: []string{"fr"}, Produced: 10},
		{Produced: 14},
	}
	derived := 0
	res, err := convergence.Loop(context.Background(), convergence.LoopOptions{UntilGate: true, MaxPasses: 5},
		convergence.LoopFuncs{
			Derive: func(context.Context) (convergence.PassState, error) {
				s := states[derived]
				derived++
				return s, nil
			},
			Produce: func(context.Context, string, int, *convergence.Emitter) (convergence.PassProduction, error) {
				return convergence.PassProduction{}, nil
			},
		}, nil)
	require.NoError(t, err)
	assert.Equal(t, 2, res.Passes)
	assert.Equal(t, 14, res.Produced)
}
