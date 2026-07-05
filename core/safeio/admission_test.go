package safeio

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdmissionOversizedFileTakesWholeBudget(t *testing.T) {
	a := NewAdmission(100)

	// A want far above the total is clamped: it acquires the full budget
	// without deadlocking or being rejected.
	release, err := a.Acquire(context.Background(), 1_000_000)
	require.NoError(t, err)
	require.NotNil(t, release)

	// While the oversized acquisition holds the budget, nothing else fits —
	// even a 1-byte want must wait.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = a.Acquire(ctx, 1)
	require.ErrorIs(t, err, context.DeadlineExceeded)

	// Releasing frees the whole budget again.
	release()
	release2, err := a.Acquire(context.Background(), 100)
	require.NoError(t, err)
	release2()
}

func TestAdmissionManySmallFilesRunConcurrently(t *testing.T) {
	a := NewAdmission(1000)

	// 10 × 100 bytes fill the budget exactly; all must be admitted while
	// held concurrently (none released yet).
	var releases []func()
	for range 10 {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		release, err := a.Acquire(ctx, 100)
		cancel()
		require.NoError(t, err)
		releases = append(releases, release)
	}

	// The 11th does not fit until one releases.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := a.Acquire(ctx, 100)
	require.ErrorIs(t, err, context.DeadlineExceeded)

	releases[0]()
	release, err := a.Acquire(context.Background(), 100)
	require.NoError(t, err)
	release()
	for _, r := range releases[1:] {
		r()
	}
}

func TestAdmissionConcurrentWaiters(t *testing.T) {
	// Budget for 2 concurrent units of 10; 8 goroutines contend. All must
	// complete, and the observed peak concurrency must never exceed 2.
	a := NewAdmission(20)
	var inFlight, peak atomic.Int64
	errs := make(chan error, 8) // asserted after Wait: FailNow must stay on the test goroutine
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			release, err := a.Acquire(context.Background(), 10)
			if err != nil {
				errs <- err
				return
			}
			cur := inFlight.Add(1)
			for {
				p := peak.Load()
				if cur <= p || peak.CompareAndSwap(p, cur) {
					break
				}
			}
			time.Sleep(5 * time.Millisecond)
			inFlight.Add(-1)
			release()
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	assert.LessOrEqual(t, peak.Load(), int64(2))
}

func TestAdmissionCtxCancelWhileWaiting(t *testing.T) {
	a := NewAdmission(10)
	release, err := a.Acquire(context.Background(), 10)
	require.NoError(t, err)
	defer release()

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, err := a.Acquire(ctx, 5)
		errCh <- err
	}()
	// Let the goroutine park as a waiter, then cancel.
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case err := <-errCh:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(2 * time.Second):
		t.Fatal("waiter did not unblock on ctx cancel")
	}
}

func TestAdmissionNilNoOps(t *testing.T) {
	var a *Admission
	release, err := a.Acquire(context.Background(), 1<<40)
	require.NoError(t, err)
	require.NotNil(t, release)
	release()
	release() // double release on the no-op is safe too
	assert.Zero(t, a.Total())
}

func TestAdmissionDoubleReleaseIsSafe(t *testing.T) {
	a := NewAdmission(10)
	release, err := a.Acquire(context.Background(), 10)
	require.NoError(t, err)
	release()
	release() // must not over-release the semaphore (would panic)

	// The budget is back to exactly 10: a full acquire succeeds, an
	// over-full one (had the double release counted) would too — so assert
	// the full acquire blocks a second 1-byte acquire.
	r2, err := a.Acquire(context.Background(), 10)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = a.Acquire(ctx, 1)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	r2()
}

func TestAdmissionZeroAndNegativeWant(t *testing.T) {
	a := NewAdmission(10)
	release, err := a.Acquire(context.Background(), 0)
	require.NoError(t, err)
	release()
	release2, err := a.Acquire(context.Background(), -5)
	require.NoError(t, err)
	release2()
}

func TestNewAdmissionNonPositiveIsNil(t *testing.T) {
	assert.Nil(t, NewAdmission(0))
	assert.Nil(t, NewAdmission(-1))
}

func TestNewAdmissionFromEnv(t *testing.T) {
	tests := []struct {
		name      string
		value     string // "" = unset
		wantTotal int64  // 0 = nil Admission (unlimited)
	}{
		{name: "unset", value: "", wantTotal: DefaultMaxInflightBytes},
		{name: "zero", value: "0", wantTotal: DefaultMaxInflightBytes},
		{name: "unparsable", value: "lots", wantTotal: DefaultMaxInflightBytes},
		{name: "negative disables", value: "-1", wantTotal: 0},
		{name: "explicit bytes", value: "12345", wantTotal: 12345},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value == "" {
				// t.Setenv registers cleanup; set-then-unset keeps the
				// "unset" case honest even when the var leaks in from CI.
				t.Setenv(MaxInflightBytesEnv, "x")
				require.NoError(t, os.Unsetenv(MaxInflightBytesEnv))
			} else {
				t.Setenv(MaxInflightBytesEnv, tt.value)
			}
			a := NewAdmissionFromEnv()
			if tt.wantTotal == 0 {
				assert.Nil(t, a)
			} else {
				require.NotNil(t, a)
				assert.Equal(t, tt.wantTotal, a.Total())
			}
		})
	}
}

func TestFileWeight(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	require.NoError(t, os.WriteFile(path, make([]byte, 1234), 0o644))

	assert.Equal(t, int64(1234), FileWeight(path))
	assert.Equal(t, UnknownFileWeight, FileWeight("-"), "stdin")
	assert.Equal(t, UnknownFileWeight, FileWeight(""), "empty path")
	assert.Equal(t, UnknownFileWeight, FileWeight(filepath.Join(dir, "missing")), "stat error")
	assert.Equal(t, UnknownFileWeight, FileWeight(dir), "non-regular file")
}
