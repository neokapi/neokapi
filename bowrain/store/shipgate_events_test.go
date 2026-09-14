package store

import (
	"sync"
	"sync/atomic"
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestShipGateFailures covers the record that decides whether a gate result is
// news: a failure is announced when it opens or when its not-checked flag
// changes, and a pass is announced only when an announced failure closes.
func TestShipGateFailures(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := seedShipGateProject(t, s)

	f := platstore.ShipGateFailure{ProjectID: p.ID, Stream: "main", Locale: "fr", Gate: "translated", Actual: 1, Required: 2}

	opened, err := s.OpenShipGateFailure(ctx, f)
	require.NoError(t, err)
	assert.True(t, opened, "a failure with no open record is news")

	opened, err = s.OpenShipGateFailure(ctx, f)
	require.NoError(t, err)
	assert.False(t, opened, "the same failure again is not")

	f.Actual = 0
	opened, err = s.OpenShipGateFailure(ctx, f)
	require.NoError(t, err)
	assert.False(t, opened, "a changed count under the same gate is not news")

	f.NotChecked = true
	opened, err = s.OpenShipGateFailure(ctx, f)
	require.NoError(t, err)
	assert.True(t, opened, "a failure that becomes not checked is announced again")

	other := platstore.ShipGateFailure{ProjectID: p.ID, Stream: "main", Locale: "fr", Gate: "checks", Actual: 1}
	opened, err = s.OpenShipGateFailure(ctx, other)
	require.NoError(t, err)
	assert.True(t, opened, "each gate is recorded on its own")

	closed, err := s.CloseShipGateFailure(ctx, p.ID, "main", "fr", "translated")
	require.NoError(t, err)
	assert.True(t, closed, "closing an announced failure is news")

	closed, err = s.CloseShipGateFailure(ctx, p.ID, "", "fr", "translated")
	require.NoError(t, err)
	assert.False(t, closed, "closing it again is not, and the empty stream is main")

	closed, err = s.CloseShipGateFailure(ctx, p.ID, "main", "de", "checks")
	require.NoError(t, err)
	assert.False(t, closed, "a gate never announced for a language has nothing to close")
}

// Two derivations of one project can run at once. The same failure is still
// announced once.
func TestShipGateFailures_ConcurrentOpenAnnouncesOnce(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := seedShipGateProject(t, s)
	f := platstore.ShipGateFailure{ProjectID: p.ID, Stream: "main", Locale: "fr", Gate: "stale", Actual: 3}

	var announced atomic.Int32
	errs := make([]error, 8)
	var wg sync.WaitGroup
	for i := range errs {
		wg.Go(func() {
			opened, err := s.OpenShipGateFailure(ctx, f)
			errs[i] = err
			if opened {
				announced.Add(1)
			}
		})
	}
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}
	assert.Equal(t, int32(1), announced.Load())
}
