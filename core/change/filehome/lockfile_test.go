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
	"github.com/neokapi/neokapi/core/dataroot"
	"github.com/neokapi/neokapi/core/storage/filelock"
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

// TestLockFile_EverySpellingOfANameTakesOneLock pins that the lock of a file
// does not depend on how a caller spells its name. Foo.txt and foo.txt, and
// the two Unicode forms of an accented name, reach one file on a
// case-insensitive or normalization-insensitive file system. They name one
// lock file, whether the file exists yet or not.
func TestLockFile_EverySpellingOfANameTakesOneLock(t *testing.T) {
	t.Setenv(dataroot.EnvDir, t.TempDir())
	dir := t.TempDir()
	lockOf := func(name string) string {
		t.Helper()
		lock, err := filehome.LockFile(filepath.Join(dir, name))
		require.NoError(t, err)
		return lock
	}
	const composed, decomposed = "résumé.txt", "résumé.txt"

	assert.Equal(t, lockOf("Foo.txt"), lockOf("foo.txt"), "a name not there yet, in either case")
	assert.Equal(t, lockOf(composed), lockOf(decomposed), "an accented name not there yet, in either form")

	require.NoError(t, os.WriteFile(filepath.Join(dir, "Foo.txt"), nil, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, composed), nil, 0o644))
	assert.Equal(t, lockOf("Foo.txt"), lockOf("FOO.TXT"), "a name that exists, in either case")
	assert.Equal(t, lockOf(composed), lockOf(decomposed), "an accented name that exists, in either form")
	assert.NotEqual(t, lockOf("Foo.txt"), lockOf("Bar.txt"), "two names stay two locks")
}

// TestFileHome_CreatingAndReplacingUnderTwoSpellingsTakeOneLock pins the
// commit that creates Report.json and a later commit that replaces it under
// the name report.json, which on a case-insensitive file system is the same
// file: they take one lock file.
func TestFileHome_CreatingAndReplacingUnderTwoSpellingsTakeOneLock(t *testing.T) {
	data := t.TempDir()
	t.Setenv(dataroot.EnvDir, data)
	dir := t.TempDir()
	home := filehome.New(nil, filehome.Options{})
	write := func(name, body string) {
		t.Helper()
		path := filepath.Join(dir, name)
		before, err := filehome.Digest(path)
		require.NoError(t, err)
		p, err := home.Produce(context.Background(), path, before, produceBytes(body))
		require.NoError(t, err)
		require.NoError(t, p.Commit(context.Background()))
		require.NoError(t, p.Release())
	}
	write("Report.json", `{"a": "1"}`+"\n")
	write("report.json", `{"a": "2"}`+"\n")

	locks, err := filepath.Glob(filepath.Join(data, "locks", "*.lock"))
	require.NoError(t, err)
	assert.Len(t, locks, 1, "both commits take one lock file")
}

// TestFileHome_SweepsLockFilesUnusedForAMonth pins that a commit removes
// lock files nobody has used for a month, at most once a day: the first
// commit sweeps, and a stale file that appears before a day has passed waits
// for the next sweep.
func TestFileHome_SweepsLockFilesUnusedForAMonth(t *testing.T) {
	data := t.TempDir()
	t.Setenv(dataroot.EnvDir, data)
	locks := filehome.LockDir()
	require.NoError(t, os.MkdirAll(locks, 0o700))
	stale := func(name string) string {
		t.Helper()
		p := filepath.Join(locks, name)
		require.NoError(t, os.WriteFile(p, nil, 0o600))
		old := time.Now().Add(-40 * 24 * time.Hour)
		require.NoError(t, os.Chtimes(p, old, old))
		return p
	}
	first := stale("first.lock")

	dir := t.TempDir()
	home := filehome.New(nil, filehome.Options{})
	write := func(body string) {
		t.Helper()
		path := filepath.Join(dir, "a.json")
		before, err := filehome.Digest(path)
		require.NoError(t, err)
		p, err := home.Produce(context.Background(), path, before, produceBytes(body))
		require.NoError(t, err)
		require.NoError(t, p.Commit(context.Background()))
		require.NoError(t, p.Release())
	}
	write(`{"a": "1"}` + "\n")
	assert.NoFileExists(t, first, "the first commit sweeps a lock file unused for a month")
	own, err := filehome.LockFile(filepath.Join(dir, "a.json"))
	require.NoError(t, err)
	assert.FileExists(t, own, "and keeps the one it took")

	second := stale("second.lock")
	write(`{"a": "2"}` + "\n")
	assert.FileExists(t, second, "a second commit the same day does not sweep again")
}

// TestFileHome_ACommitInsideACommitOfTheSameFileFailsAtOnce pins that a
// commit started while the same goroutine is committing the same file, here
// from inside the first commit's lock, is refused at once with a message
// naming the file, instead of waiting for itself. The first commit lands.
func TestFileHome_ACommitInsideACommitOfTheSameFileFailsAtOnce(t *testing.T) {
	t.Setenv(dataroot.EnvDir, t.TempDir())
	path := filepath.Join(t.TempDir(), "a.json")
	commit := func(body string) error {
		home := filehome.New(nil, filehome.Options{})
		before, err := filehome.Digest(path)
		if err != nil {
			return err
		}
		p, err := home.Produce(context.Background(), path, before, produceBytes(body))
		if err != nil {
			return err
		}
		defer func() { _ = p.Release() }()
		return p.Commit(context.Background())
	}
	var nested error
	var once sync.Once
	filehome.SetBeforeCommit(func(string) {
		once.Do(func() { nested = commit(`{"a": "inner"}` + "\n") })
	})
	t.Cleanup(func() { filehome.SetBeforeCommit(nil) })

	done := make(chan error, 1)
	go func() { done <- commit(`{"a": "outer"}` + "\n") }()
	select {
	case err := <-done:
		require.NoError(t, err, "the outer commit lands")
	case <-time.After(10 * time.Second):
		t.Fatal("a commit inside a commit of the same file waited for itself")
	}
	require.Error(t, nested)
	require.ErrorIs(t, nested, filelock.ErrHeldHere)
	assert.Contains(t, nested.Error(), "already being written by this process")
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, `{"a": "outer"}`+"\n", string(body))
}
