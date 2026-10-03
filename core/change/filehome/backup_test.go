package filehome_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/filehome"
	"github.com/neokapi/neokapi/core/model"
)

// A home with a backup suffix keeps a copy of each file a commit replaces,
// holding the bytes the change was applied to and the file's mode. A preview,
// a change that leaves the file as it is, and a refused change write none.
func TestFileHome_KeepsABackupOfWhatACommitReplaces(t *testing.T) {
	const src = `{"greeting": "Hello there", "farewell": "Goodbye now"}` + "\n"
	ctx := context.Background()
	edit := func(f *fixture, mode change.Mode, text, ifMatch string) *change.Result {
		t.Helper()
		set := change.Set{Mode: mode, Gate: change.GateEnforce, Ops: []change.Op{{
			Kind: change.KindSetContent, At: change.Ref{Doc: "a.json", Block: "greeting"}, IfMatch: ifMatch,
			Body: &change.SetContent{Text: &text},
		}}}
		res, err := f.svc.Apply(ctx, set, person)
		require.NoError(t, err)
		return res
	}
	rev := func(f *fixture) string {
		t.Helper()
		page, err := f.svc.Read(ctx, change.ReadRequest{Doc: "a.json", Blocks: []string{"greeting"}})
		require.NoError(t, err)
		require.Len(t, page.Blocks, 1)
		return page.Blocks[0].Rev
	}

	f := newFixture(t, map[string]string{"a.json": src}, filehome.Options{BackupSuffix: ".bak"})
	backup := filepath.Join(f.dir, "a.json.bak")

	res := edit(f, change.ModePreview, "Hi", rev(f))
	assert.Equal(t, change.SetPreviewed, res.Status)
	assert.NoFileExists(t, backup, "a preview writes no backup")

	res = edit(f, change.ModeApply, "Hi", "r:0000000000000000")
	assert.Equal(t, change.SetRefused, res.Status)
	assert.NoFileExists(t, backup, "a refused change writes no backup")

	res = edit(f, change.ModeApply, "Hello there", rev(f))
	assert.Equal(t, change.OpUnchanged, res.Ops[0].Status)
	assert.NoFileExists(t, backup, "a change that leaves the file as it is writes no backup")

	res = edit(f, change.ModeApply, "Hi", rev(f))
	require.Equal(t, change.SetApplied, res.Status, "%+v", res.Ops[0].Error)
	got, err := os.ReadFile(backup)
	require.NoError(t, err)
	assert.Equal(t, src, string(got))
	assert.Contains(t, f.read(t, "a.json"), `"Hi"`)
	if runtime.GOOS != "windows" {
		info, err := os.Stat(backup)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o640), info.Mode().Perm(), "the backup keeps the file's mode")
	}
}

// ReadEach reads every block of a document in one pass, past the most a page
// holds, and hands each over with the block it was read from.
func TestService_ReadEachReadsTheWholeDocument(t *testing.T) {
	const n = change.MaxReadLimit + 250
	var b strings.Builder
	b.WriteString("{")
	for i := range n {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `"k%04d": "Text %d"`, i, i)
	}
	b.WriteString("}\n")
	f := newFixture(t, map[string]string{"big.json": b.String()})
	ctx := context.Background()

	page, err := f.svc.Read(ctx, change.ReadRequest{Doc: "big.json", Limit: n})
	require.NoError(t, err)
	assert.Len(t, page.Blocks, change.MaxReadLimit, "a page holds at most MaxReadLimit blocks")

	count := 0
	head, err := f.svc.ReadEach(ctx, change.ReadRequest{Doc: "big.json"}, func(blk *model.Block, r change.BlockRead) error {
		assert.Equal(t, fmt.Sprintf("k%04d", count), r.Ref.Block)
		assert.Equal(t, model.EditionRevision(blk, model.EditionKey{}), r.Rev, "the read record is the block's")
		count++
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, n, count)
	assert.Equal(t, page.Head, head.Head)
	assert.Empty(t, head.Blocks)

	stopped := 0
	_, err = f.svc.ReadEach(ctx, change.ReadRequest{Doc: "big.json"}, func(*model.Block, change.BlockRead) error {
		stopped++
		if stopped == 3 {
			return change.ErrStop
		}
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 3, stopped, "ErrStop ends the read")
}

// A home calls PrepareLocks before it opens a lock file, which a commit does
// and a read never does, so a host creates the directory its locks live in
// only when a change lands.
func TestFileHome_PreparesTheLockDirectoryOnlyToCommit(t *testing.T) {
	ctx := context.Background()
	text := "Hi"
	setOf := func(rev string) change.Set {
		return change.Set{Mode: change.ModeApply, Gate: change.GateEnforce, Ops: []change.Op{{
			Kind: change.KindSetContent, At: change.Ref{Doc: "a.json", Block: "greeting"}, IfMatch: rev,
			Body: &change.SetContent{Text: &text},
		}}}
	}
	locks := filepath.Join(t.TempDir(), "work", "locks")
	prepared := 0
	f := newFixture(t, map[string]string{"a.json": `{"greeting": "Hello"}` + "\n"}, filehome.Options{
		LockDir:      locks,
		PrepareLocks: func() error { prepared++; return nil },
	})

	page, err := f.svc.Read(ctx, change.ReadRequest{Doc: "a.json"})
	require.NoError(t, err)
	require.Len(t, page.Blocks, 1)
	_, err = f.svc.ReadEach(ctx, change.ReadRequest{Doc: "a.json"}, func(*model.Block, change.BlockRead) error { return nil })
	require.NoError(t, err)
	assert.Zero(t, prepared, "a read takes no lock")
	assert.NoDirExists(t, locks, "a read creates no lock directory")

	res, err := f.svc.Apply(ctx, setOf(page.Blocks[0].Rev), person)
	require.NoError(t, err)
	require.Equal(t, change.SetApplied, res.Status)
	assert.Positive(t, prepared, "a commit prepares the lock directory before it locks")
	assert.DirExists(t, locks)

	f = newFixture(t, map[string]string{"a.json": `{"greeting": "Hello"}` + "\n"}, filehome.Options{
		PrepareLocks: func() error { return errors.New("no room for locks") },
	})
	page, err = f.svc.Read(ctx, change.ReadRequest{Doc: "a.json"})
	require.NoError(t, err)
	_, err = f.svc.Apply(ctx, setOf(page.Blocks[0].Rev), person)
	require.ErrorContains(t, err, "no room for locks", "a lock directory the host cannot prepare stops the commit")
	assert.Contains(t, f.read(t, "a.json"), "Hello", "nothing is written")
}
