package service

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/safeio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newCappedFlowService builds a FlowService with an explicit small admission
// budget and a short shed-wait, bypassing the env-driven defaults so the
// resource-cap behavior is exercised deterministically.
func newCappedFlowService(totalBytes int64, wait time.Duration) *FlowService {
	fs := NewFlowService(nil, nil, nil)
	fs.admission = safeio.NewAdmission(totalBytes)
	fs.admissionWait = wait
	return fs
}

// TestFlowServiceAcquireCapacity_ShedsWhenSaturated proves that once the budget
// is held, a further acquire that cannot fit is shed with ErrCapacityExhausted
// after the wait, and that the budget is reusable once released.
func TestFlowServiceAcquireCapacity_ShedsWhenSaturated(t *testing.T) {
	fs := newCappedFlowService(100, 50*time.Millisecond)
	ctx := context.Background()

	release1, err := fs.AcquireCapacity(ctx, 60)
	require.NoError(t, err)

	// Only 40 bytes remain; a 60-byte request cannot fit and is shed.
	start := time.Now()
	_, err = fs.AcquireCapacity(ctx, 60)
	require.ErrorIs(t, err, ErrCapacityExhausted)
	assert.GreaterOrEqual(t, time.Since(start), 40*time.Millisecond,
		"a saturated acquire should queue for ~admissionWait before shedding")

	// Releasing the first frees the budget for a fresh acquire.
	release1()
	release3, err := fs.AcquireCapacity(ctx, 60)
	require.NoError(t, err)
	release3()
}

// TestFlowServiceAcquireCapacity_NilAdmissionNoCap proves that a negative budget
// disables the cap entirely: acquires always succeed immediately with a no-op
// release, and calling release twice is safe.
func TestFlowServiceAcquireCapacity_NilAdmissionNoCap(t *testing.T) {
	fs := NewFlowService(nil, nil, nil)
	fs.admission = safeio.NewAdmission(-1) // negative → nil (no cap)
	require.Nil(t, fs.admission)

	release, err := fs.AcquireCapacity(context.Background(), 1<<40)
	require.NoError(t, err)
	release()
	release() // idempotent
}

// TestFlowServiceAcquireCapacity_ContextCanceled proves caller cancellation is
// surfaced as the context error, not misreported as capacity exhaustion.
func TestFlowServiceAcquireCapacity_ContextCanceled(t *testing.T) {
	fs := newCappedFlowService(100, time.Second)

	// Hold the whole budget so the next acquire must wait.
	release, err := fs.AcquireCapacity(context.Background(), 100)
	require.NoError(t, err)
	defer release()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already canceled
	_, err = fs.AcquireCapacity(ctx, 100)
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrCapacityExhausted)
	assert.ErrorIs(t, err, context.Canceled)
}

// TestFlowServiceAcquireCapacity_ConcurrentContention drives several goroutines
// at a budget that admits only one at a time and asserts the cap sheds the
// surplus: at least one succeeds, at least one is shed, and every attempt is
// accounted for.
func TestFlowServiceAcquireCapacity_ConcurrentContention(t *testing.T) {
	fs := newCappedFlowService(100, 40*time.Millisecond)

	const goroutines = 4
	var (
		wg       sync.WaitGroup
		admitted int32
		shed     int32
		other    int32
	)
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// 60 > budget/2, so at most one holder fits at a time.
			release, err := fs.AcquireCapacity(context.Background(), 60)
			switch {
			case err == nil:
				atomic.AddInt32(&admitted, 1)
				time.Sleep(120 * time.Millisecond) // hold past the others' shed-wait
				release()
			case err == ErrCapacityExhausted:
				atomic.AddInt32(&shed, 1)
			default:
				atomic.AddInt32(&other, 1)
			}
		}()
	}
	wg.Wait()

	assert.Zero(t, other, "no acquire should fail for a reason other than capacity")
	assert.GreaterOrEqual(t, admitted, int32(1), "at least one flow must be admitted")
	assert.GreaterOrEqual(t, shed, int32(1), "contention must shed at least one flow")
	assert.Equal(t, int32(goroutines), admitted+shed, "every attempt must be accounted for")
}

// TestExecuteFlowShedsWhenSaturated proves ExecuteFlow returns
// ErrCapacityExhausted (and never runs the executor) when the budget is already
// fully held by another in-flight request.
func TestExecuteFlowShedsWhenSaturated(t *testing.T) {
	fs := newCappedFlowService(100, 30*time.Millisecond)

	// Occupy the whole budget.
	release, err := fs.AcquireCapacity(context.Background(), 100)
	require.NoError(t, err)
	defer release()

	f := &flow.Flow{Name: "noop"}
	items := []*flow.Item{{Input: &model.RawDocument{URI: "mem://doc"}}}
	err = fs.ExecuteFlow(context.Background(), f, items, "")
	require.ErrorIs(t, err, ErrCapacityExhausted)
}

// TestFlowItemsWeightUnknownInputs proves in-memory/stdin inputs draw the
// conservative unknown-file weight rather than zero, so a batch always reserves
// some budget.
func TestFlowItemsWeightUnknownInputs(t *testing.T) {
	got := flowItemsWeight([]*flow.Item{
		{Input: &model.RawDocument{URI: "mem://a"}},
		{Input: &model.RawDocument{URI: "-"}},
		{Input: nil},
	})
	assert.Equal(t, 2*safeio.UnknownFileWeight, got)
}
