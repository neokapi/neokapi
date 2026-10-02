package filelock_test

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/storage/filelock"
)

func TestLock_TwoLocksOnOnePathExcludeEachOther(t *testing.T) {
	if !filelock.Supported {
		t.Skip("this platform has no advisory lock")
	}
	path := filepath.Join(t.TempDir(), "doc.lock")
	a, err := filelock.Open(path)
	require.NoError(t, err)
	defer a.Close()
	b, err := filelock.Open(path)
	require.NoError(t, err)
	defer b.Close()
	assert.Equal(t, path, a.Path())

	require.NoError(t, a.Lock(t.Context()))
	var held atomic.Bool
	done := make(chan error, 1)
	go func() {
		err := b.Lock(context.Background())
		held.Store(true)
		done <- err
	}()
	time.Sleep(50 * time.Millisecond)
	assert.False(t, held.Load(), "the second lock waits while the first is held")
	a.Unlock()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("the second lock was never taken after the first was released")
	}
	b.Unlock()
}

func TestLock_ACancelledContextTakesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "doc.lock")
	l, err := filelock.Open(path)
	require.NoError(t, err)
	defer l.Close()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, l.Lock(ctx), context.Canceled)
	if l != nil {
		require.NoError(t, l.Lock(t.Context()), "the lock is free after a cancelled attempt")
		l.Unlock()
	}
}

func TestLock_NilIsALockThatDoesNothing(t *testing.T) {
	var l *filelock.Lock
	require.NoError(t, l.Lock(t.Context()))
	l.Unlock()
	require.NoError(t, l.Close())
	assert.Empty(t, l.Path())
}
