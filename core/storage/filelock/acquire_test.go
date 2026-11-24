package filelock_test

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/storage/filelock"
)

// The variables a locker process reads.
const (
	stressPathEnv   = "FILELOCK_STRESS_PATH"
	stressRoundsEnv = "FILELOCK_STRESS_ROUNDS"
)

// stressRounds is how many times each locker takes the lock.
const stressRounds = 1500

// errOverlap is what a locker reports when it finds another holder inside.
var errOverlap = errors.New("two holders of one lock at once")

// holdOnce takes the lock on path through Acquire and, while it is held,
// creates the marker beside it exclusively and removes it again. A second
// holder inside at the same time finds the marker there.
func holdOnce(ctx context.Context, path string) error {
	l, err := filelock.Acquire(ctx, path)
	if err != nil {
		return err
	}
	defer l.Close()
	defer l.Unlock()
	marker := path + ".held"
	f, err := os.OpenFile(marker, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, fs.ErrExist) {
		return errOverlap
	}
	if err != nil {
		return err
	}
	_ = f.Close()
	return os.Remove(marker)
}

// lockerLoop takes the lock rounds times.
func lockerLoop(ctx context.Context, path string, rounds int) error {
	for i := range rounds {
		if err := holdOnce(ctx, path); err != nil {
			return fmt.Errorf("round %d: %w", i, err)
		}
	}
	return nil
}

// TestAcquire_AndSweepNeverLetTwoHoldersIn races lockers against a sweep
// that removes the lock file whenever it can, on one path. Two lockers run in
// this process and two in processes of their own, each taking the lock
// stressRounds times, while the sweep runs with an age of zero, so every
// lock file no one holds is a candidate. No two holders may ever be inside
// at once, on every platform.
//
// Where lockers run in processes of their own, the sweep must also have
// removed the file while they ran, which proves the race was exercised. The
// browser build (js/wasm) starts no process and runs one goroutine at a time,
// so there the run proves only that no interleaving it produced let two
// holders in. The removal itself is pinned on every platform by
// TestSweep_RemovesAStaleFileAndAcquireMakesANewOne.
func TestAcquire_AndSweepNeverLetTwoHoldersIn(t *testing.T) {
	if path := os.Getenv(stressPathEnv); path != "" {
		rounds, err := strconv.Atoi(os.Getenv(stressRoundsEnv))
		require.NoError(t, err)
		require.NoError(t, lockerLoop(context.Background(), path, rounds))
		return
	}
	if testing.Short() {
		t.Skip("takes the lock some thousands of times")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.lock")
	ctx := context.Background()

	var lockers sync.WaitGroup
	errs := make(chan error, 4)
	for range 2 {
		lockers.Go(func() { errs <- lockerLoop(ctx, path, stressRounds) })
	}
	if runtime.GOOS != "js" {
		exe, err := os.Executable()
		require.NoError(t, err)
		for range 2 {
			lockers.Go(func() {
				cmd := exec.Command(exe, "-test.run=^TestAcquire_AndSweepNeverLetTwoHoldersIn$")
				cmd.Env = append(os.Environ(), stressPathEnv+"="+path, stressRoundsEnv+"="+strconv.Itoa(stressRounds))
				out, err := cmd.CombinedOutput()
				if err != nil {
					err = fmt.Errorf("locker process: %w: %s", err, out)
				}
				errs <- err
			})
		}
	}

	var removed atomic.Int64
	done := make(chan struct{})
	var sweeper sync.WaitGroup
	sweeper.Go(func() {
		for {
			select {
			case <-done:
				return
			default:
			}
			n, _, _ := filelock.Sweep(dir, 0, 0, 100)
			removed.Add(int64(n))
		}
	})
	lockers.Wait()
	close(done)
	sweeper.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	if runtime.GOOS != "js" {
		assert.Positive(t, removed.Load(), "the sweep removed the lock file while the lockers ran")
	}
	t.Logf("the sweep removed the lock file %d times", removed.Load())
}

// TestSweep_RemovesAStaleFileAndAcquireMakesANewOne pins, without relying on
// a scheduler, the steps the stress test races: a sweep removes a stale lock
// file no one holds; Acquire then makes a new one and holds it; a sweep with
// no age leaves the held file; once released, the sweep removes it.
func TestSweep_RemovesAStaleFileAndAcquireMakesANewOne(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.lock")
	require.NoError(t, os.WriteFile(path, nil, 0o600))
	old := time.Now().Add(-48 * time.Hour)
	require.NoError(t, os.Chtimes(path, old, old))

	n, _, err := filelock.Sweep(dir, 24*time.Hour, 0, 100)
	require.NoError(t, err)
	require.Equal(t, 1, n, "the stale file no one holds is removed")
	require.NoFileExists(t, path)

	l, err := filelock.Acquire(context.Background(), path)
	require.NoError(t, err)
	require.FileExists(t, path, "Acquire makes the lock file again")
	n, _, err = filelock.Sweep(dir, 0, 0, 100)
	require.NoError(t, err)
	assert.Zero(t, n, "a held file stays, whatever its age")
	assert.FileExists(t, path)

	l.Unlock()
	require.NoError(t, l.Close())
	n, _, err = filelock.Sweep(dir, 0, 0, 100)
	require.NoError(t, err)
	assert.Equal(t, 1, n, "once released, it is removed")
	assert.NoFileExists(t, path)
}

// TestSweep_LeavesAFileInUse pins what a sweep removes: a lock file older
// than the age that no one holds. A held file, a file used within the age,
// and a file that is not a lock file stay.
func TestSweep_LeavesAFileInUse(t *testing.T) {
	dir := t.TempDir()
	old := time.Now().Add(-48 * time.Hour)
	mk := func(name string, mtime time.Time) string {
		p := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(p, nil, 0o600))
		require.NoError(t, os.Chtimes(p, mtime, mtime))
		return p
	}
	stale := mk("stale.lock", old)
	fresh := mk("fresh.lock", time.Now())
	other := mk("notes.txt", old)
	held := mk("held.lock", old)
	l, err := filelock.Open(held)
	require.NoError(t, err)
	defer l.Close()
	require.NoError(t, l.Lock(t.Context()))
	defer l.Unlock()

	n, _, err := filelock.Sweep(dir, 24*time.Hour, 0, 100)
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	assert.NoFileExists(t, stale, "a stale lock file no one holds is removed")
	assert.FileExists(t, fresh, "a lock file used within the age stays")
	assert.FileExists(t, other, "a file that is not a lock file stays")
	assert.FileExists(t, held, "a held lock file stays")
}

// TestAcquire_MarksTheFileAsUsed pins that taking the lock sets the lock
// file's time stamp, which keeps a sweep away from a file in use.
func TestAcquire_MarksTheFileAsUsed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc.lock")
	require.NoError(t, os.WriteFile(path, nil, 0o600))
	old := time.Now().Add(-48 * time.Hour)
	require.NoError(t, os.Chtimes(path, old, old))
	l, err := filelock.Acquire(t.Context(), path)
	require.NoError(t, err)
	l.Unlock()
	require.NoError(t, l.Close())
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now(), info.ModTime(), time.Hour)
}

// TestAcquire_RefusesALockItsOwnGoroutineHolds pins that a goroutine asking
// again for a lock it holds is refused at once, naming the file, instead of
// waiting for itself forever.
func TestAcquire_RefusesALockItsOwnGoroutineHolds(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc.lock")
	done := make(chan error, 1)
	go func() {
		l, err := filelock.Acquire(context.Background(), path)
		if err != nil {
			done <- err
			return
		}
		defer l.Close()
		defer l.Unlock()
		again, err := filelock.Acquire(context.Background(), path)
		if err == nil {
			again.Unlock()
			_ = again.Close()
		}
		done <- err
	}()
	select {
	case err := <-done:
		require.Error(t, err)
		require.ErrorIs(t, err, filelock.ErrHeldHere)
		assert.Contains(t, err.Error(), path, "the refusal names the file")
		assert.Contains(t, err.Error(), "already being written by this process")
	case <-time.After(10 * time.Second):
		t.Fatal("a nested acquire of a held lock waited instead of being refused")
	}

	l, err := filelock.Acquire(context.Background(), path)
	require.NoError(t, err, "once released, the lock is free to take again")
	l.Unlock()
	require.NoError(t, l.Close())
}

// TestAcquire_TwoGoroutinesWaitForEachOther pins the other half of the rule:
// a second goroutine of the process asking for a held lock waits for it, as
// another process does, and takes it once it is released. The record of
// holders refuses only the holder's own goroutine.
func TestAcquire_TwoGoroutinesWaitForEachOther(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc.lock")
	first, err := filelock.Acquire(context.Background(), path)
	require.NoError(t, err)

	got := make(chan error, 1)
	var second *filelock.Lock
	go func() {
		l, err := filelock.Acquire(context.Background(), path)
		second = l
		got <- err
	}()
	select {
	case err := <-got:
		t.Fatalf("the second goroutine returned while the first held the lock: %v", err)
	case <-time.After(200 * time.Millisecond):
	}
	first.Unlock()
	require.NoError(t, first.Close())
	select {
	case err := <-got:
		require.NoError(t, err, "the second goroutine takes the lock once it is released")
	case <-time.After(10 * time.Second):
		t.Fatal("the second goroutine never took the released lock")
	}
	second.Unlock()
	require.NoError(t, second.Close())
}

// TestSweep_ReachesEveryFileInTurn pins that sweeps bounded by a limit start
// where the last one stopped, so a stale file past the first limit entries
// is reached by a later sweep. Thirty files, three of them stale, are swept
// ten at a time.
func TestSweep_ReachesEveryFileInTurn(t *testing.T) {
	dir := t.TempDir()
	old := time.Now().Add(-48 * time.Hour)
	for i := range 30 {
		p := filepath.Join(dir, fmt.Sprintf("f%02d.lock", i))
		require.NoError(t, os.WriteFile(p, nil, 0o600))
		if i%10 == 9 {
			require.NoError(t, os.Chtimes(p, old, old))
		}
	}
	start, total := 0, 0
	for range 3 {
		n, next, err := filelock.Sweep(dir, 24*time.Hour, start, 10)
		require.NoError(t, err)
		total += n
		start = next
	}
	assert.Equal(t, 3, total, "three sweeps of ten reach all three stale files")
	left, err := filepath.Glob(filepath.Join(dir, "*.lock"))
	require.NoError(t, err)
	assert.Len(t, left, 27, "and leave every file in use")
}

// TestParseGoroutineID pins the parser of a stack's first line, and that a
// line of any other shape reads as 0, the number of a goroutine that cannot
// be told.
func TestParseGoroutineID(t *testing.T) {
	assert.Equal(t, uint64(18), filelock.ParseGoroutineID([]byte("goroutine 18 [running]:\nmain.main()")))
	for _, line := range []string{"", "goroutine", "goroutine x [running]:", "thread 18 [running]:", "goroutine -1 [running]:"} {
		assert.Zero(t, filelock.ParseGoroutineID([]byte(line)), "%q", line)
	}
}

// TestAcquire_WaitsWhenTheGoroutineCannotBeTold pins what Acquire does when
// the goroutine number cannot be read, so every goroutine reads as 0: it
// records nothing, and a second goroutine waits for a held lock as it would
// otherwise, instead of being refused as though it were the holder.
func TestAcquire_WaitsWhenTheGoroutineCannotBeTold(t *testing.T) {
	restore := filelock.BreakGoroutineIDs()
	defer restore()
	path := filepath.Join(t.TempDir(), "doc.lock")
	first, err := filelock.Acquire(context.Background(), path)
	require.NoError(t, err)

	got := make(chan error, 1)
	var second *filelock.Lock
	go func() {
		l, err := filelock.Acquire(context.Background(), path)
		second = l
		got <- err
	}()
	select {
	case err := <-got:
		t.Fatalf("the second goroutine returned while the first held the lock: %v", err)
	case <-time.After(200 * time.Millisecond):
	}
	first.Unlock()
	require.NoError(t, first.Close())
	select {
	case err := <-got:
		require.NoError(t, err, "the second goroutine takes the lock once it is released")
	case <-time.After(10 * time.Second):
		t.Fatal("the second goroutine never took the released lock")
	}
	second.Unlock()
	require.NoError(t, second.Close())
}
