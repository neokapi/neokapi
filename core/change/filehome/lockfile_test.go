package filehome_test

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

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/filehome"
)

// TestFileHome_TwoHomesOnOneFileWaitForEachOther pins that two homes built
// the way two different callers build them, one over a project's layout that
// prepares the project before it locks and one that only commits produced
// documents, exclude each other on one file. The first writer stops inside
// its commit, with its lock held, until the second has staged and gone for
// the lock. The second may not reach its own commit while the first is in
// there.
func TestFileHome_TwoHomesOnOneFileWaitForEachOther(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"greeting": "Hello there", "farewell": "Goodbye"}`+"\n"), 0o644))
	reg := newRegistry()

	var inside, calls atomic.Int32
	var overlapped atomic.Bool
	secondLocking := make(chan struct{})
	secondInside := make(chan struct{})
	var secondOnce, lockingOnce sync.Once
	signalLocking := func() { lockingOnce.Do(func() { close(secondLocking) }) }
	var second sync.WaitGroup
	var secondErr error

	startSecond := func() {
		svc := change.NewService(filehome.Formats{Registry: reg},
			change.OneHome(filehome.New(filehome.DirLayout{Root: dir, Formats: reg, SourceLocale: "en"}, filehome.Options{
				Root:         dir,
				PrepareLocks: func() error { return nil },
				BeforeSettle: func(string) { signalLocking() },
			})))
		second.Go(func() {
			ctx := context.Background()
			page, err := svc.Read(ctx, change.ReadRequest{Doc: "a.json"})
			if err != nil {
				secondErr = err
				signalLocking()
				return
			}
			b := page.Blocks[1]
			_, secondErr = svc.Apply(ctx, change.Set{Ops: []change.Op{setOp(b.Ref, b.Rev, "Bye")}}, person)
		})
	}

	filehome.SetBeforeCommit(func(string) {
		if inside.Add(1) > 1 {
			overlapped.Store(true)
		}
		defer inside.Add(-1)
		if calls.Add(1) != 1 {
			secondOnce.Do(func() { close(secondInside) })
			return
		}
		// The first writer holds its lock here. The second stages and goes
		// for the lock; a second that shares the lock cannot get past it, and
		// one that does not reaches this hook within the window.
		startSecond()
		<-secondLocking
		select {
		case <-secondInside:
		case <-time.After(time.Second):
		}
	})
	t.Cleanup(func() { filehome.SetBeforeCommit(nil) })

	first := filehome.New(nil, filehome.Options{})
	before, err := filehome.Digest(path)
	require.NoError(t, err)
	p, err := first.Produce(context.Background(), path, before, produceBytes(`{"greeting": "Bonjour", "farewell": "Goodbye"}`+"\n"))
	require.NoError(t, err)
	require.NoError(t, p.Commit(context.Background()))
	require.NoError(t, p.Release())
	second.Wait()

	require.NoError(t, secondErr)
	assert.False(t, overlapped.Load(), "the second writer committed while the first held the lock of the same file")
	assert.EqualValues(t, 2, calls.Load(), "both writers commit")
}
