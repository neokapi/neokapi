package filehome_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change/filehome"
	"github.com/neokapi/neokapi/core/dataroot"
	"github.com/neokapi/neokapi/core/storage/filelock"
)

// commitWaitsFor commits a new body to path through ctx while the test holds
// the lock of hold, and fails unless the commit waits for that lock: it may
// not finish while the lock is held, and must finish once it is released.
func commitWaitsFor(t *testing.T, ctx context.Context, path, hold string) {
	t.Helper()
	key, err := filehome.LockFile(hold)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(key), 0o700))
	held, err := filelock.Acquire(context.Background(), key)
	require.NoError(t, err)

	home := filehome.New(nil, filehome.Options{})
	before, err := filehome.Digest(path)
	require.NoError(t, err)
	p, err := home.Produce(context.Background(), path, before, produceBytes(`{"a": "later"}`+"\n"))
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() { done <- p.Commit(ctx) }()
	select {
	case err := <-done:
		t.Fatalf("the commit landed while the lock was held elsewhere: %v", err)
	case <-time.After(200 * time.Millisecond):
	}
	held.Unlock()
	require.NoError(t, held.Close())
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("the commit never took the released lock")
	}
}

// TestWithLock_AContextKeptPastTheHoldTakesTheLock pins that the context
// WithLock hands its function claims the lock only while the hold lasts. A
// context kept after WithLock returned commits like any other caller: it
// takes the lock, and waits for it while someone else holds it.
func TestWithLock_AContextKeptPastTheHoldTakesTheLock(t *testing.T) {
	t.Setenv(dataroot.EnvDir, t.TempDir())
	path := filepath.Join(t.TempDir(), "a.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"a": "1"}`+"\n"), 0o644))

	var kept context.Context
	require.NoError(t, filehome.WithLock(context.Background(), path, func(ctx context.Context) error {
		kept = ctx
		return nil
	}))
	commitWaitsFor(t, kept, path, path)
}

// TestWithLock_AHoldOfOneFileSkipsNoOtherLock pins that the context
// WithLock hands its function claims only the lock it holds: a commit of
// another file through it takes that file's lock.
func TestWithLock_AHoldOfOneFileSkipsNoOtherLock(t *testing.T) {
	t.Setenv(dataroot.EnvDir, t.TempDir())
	dir := t.TempDir()
	held, other := filepath.Join(dir, "held.json"), filepath.Join(dir, "other.json")
	for _, p := range []string{held, other} {
		require.NoError(t, os.WriteFile(p, []byte(`{"a": "1"}`+"\n"), 0o644))
	}
	require.NoError(t, filehome.WithLock(context.Background(), held, func(ctx context.Context) error {
		commitWaitsFor(t, ctx, other, other)
		return nil
	}))
}
