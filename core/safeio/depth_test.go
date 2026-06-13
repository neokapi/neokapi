package safeio_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/safeio"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDepthGuard(t *testing.T) {
	t.Parallel()
	g := safeio.NewDepthGuard(3)

	require.NoError(t, g.Enter()) // depth 1
	require.NoError(t, g.Enter()) // depth 2
	require.NoError(t, g.Enter()) // depth 3
	assert.Equal(t, 3, g.Depth())

	err := g.Enter() // would be depth 4
	require.Error(t, err)
	assert.ErrorIs(t, err, safeio.ErrTooDeep)
	assert.Equal(t, 3, g.Depth(), "failed Enter must not change depth")

	g.Leave()
	assert.Equal(t, 2, g.Depth())
	require.NoError(t, g.Enter(), "should fit again after Leave")
	assert.Equal(t, 3, g.Depth())
}

func TestDepthGuard_LeaveNeverNegative(t *testing.T) {
	t.Parallel()
	g := safeio.NewDepthGuard(5)
	g.Leave()
	g.Leave()
	assert.Equal(t, 0, g.Depth())
	require.NoError(t, g.Enter())
	assert.Equal(t, 1, g.Depth())
}

func TestDepthGuard_ZeroDisablesCap(t *testing.T) {
	t.Parallel()
	g := safeio.NewDepthGuard(0)
	for range 10000 {
		require.NoError(t, g.Enter())
	}
}

func TestDepthGuard_NilSafe(t *testing.T) {
	t.Parallel()
	var g *safeio.DepthGuard
	require.NoError(t, g.Enter())
	g.Leave()
	assert.Equal(t, 0, g.Depth())
	require.NoError(t, g.Do(func() error { return nil }))
}

func TestDepthGuard_Do(t *testing.T) {
	t.Parallel()

	// A recursive descent bounded by Do returns ErrTooDeep instead of
	// overflowing the stack.
	g := safeio.NewDepthGuard(4)
	var recurse func(n int) error
	calls := 0
	recurse = func(n int) error {
		return g.Do(func() error {
			calls++
			if n == 0 {
				return nil
			}
			return recurse(n - 1)
		})
	}

	// 4 levels fits exactly.
	require.NoError(t, recurse(3))
	assert.Equal(t, 4, calls)

	// 100 levels trips the guard, and depth is fully restored afterwards.
	calls = 0
	err := recurse(100)
	require.ErrorIs(t, err, safeio.ErrTooDeep)
	assert.Equal(t, 4, calls, "should stop descending at the cap")
	assert.Equal(t, 0, g.Depth(), "depth restored after unwinding")
}

func TestDepthGuard_DoPropagatesError(t *testing.T) {
	t.Parallel()
	g := safeio.NewDepthGuard(10)
	sentinel := assert.AnError
	err := g.Do(func() error { return sentinel })
	require.ErrorIs(t, err, sentinel)
	assert.Equal(t, 0, g.Depth())
}
