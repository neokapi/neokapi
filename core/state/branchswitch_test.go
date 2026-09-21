package state_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/state"
)

// The committed shards are git-tracked, so a branch switch replaces them
// wholesale while the store, which is not tracked, keeps what it held. These
// tests drive that with a real repository: two branches whose records name
// different documents, and one store that outlives the switch between them.
//
// Reading a record is explicit: opening a store reads nothing, and
// `kapi context import` calls Import. The fixture's open() does both, which is
// what a person who has just switched branches and read the record in has.

// branchRepo is a git repository with a committed record on each of two
// branches, plus the path of the record directory and of the store that spans
// them.
type branchRepo struct {
	t       *testing.T
	root    string
	record  string // the committed record directory, .kapi/state
	storeDB string // the store, .kapi/work/store.db
}

func (r *branchRepo) git(args ...string) {
	r.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = r.root
	out, err := cmd.CombinedOutput()
	require.NoErrorf(r.t, err, "git %v: %s", args, out)
}

// newBranchRepo builds the fixture: branch `main` holds a record naming
// `doc-main`, branch `feature` holds one naming `doc-feature`. Neither branch's
// record mentions the other's document, which is what makes a write from the
// wrong view visible as both a stray shard and a pruned one.
func newBranchRepo(t *testing.T) *branchRepo {
	t.Helper()
	root := t.TempDir()
	r := &branchRepo{
		t:       t,
		root:    root,
		record:  filepath.Join(root, ".kapi", "state"),
		storeDB: filepath.Join(root, ".kapi", "work", "store.db"),
	}
	r.git("init", "--initial-branch=main")
	r.git("config", "user.email", "fixture@example.test")
	r.git("config", "user.name", "Fixture")
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".kapi"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".kapi", ".gitignore"),
		[]byte("work/\n"), 0o644))

	require.NoError(t, state.WriteCommitted(r.record, []state.UnitState{
		unit("u-main", "doc-main", "Alpha"),
	}))
	r.git("add", ".kapi")
	r.git("commit", "-m", "record on main")

	r.git("switch", "-c", "feature")
	require.NoError(t, state.WriteCommitted(r.record, []state.UnitState{
		unit("u-feature", "doc-feature", "Bravo"),
	}))
	r.git("add", "-A", ".kapi")
	r.git("commit", "-m", "record on feature")
	return r
}

// open returns a store over the record as this checkout holds it, with that
// record read in. The database file is the same one every time, which is the
// whole point: it is not tracked, so it survives the switch.
func (r *branchRepo) open() *state.WorkStore {
	r.t.Helper()
	w, err := state.OpenWork(r.t.Context(), r.storeDB, r.record)
	require.NoError(r.t, err)
	require.NoError(r.t, w.Import(r.t.Context()))
	return w
}

func scopesOf(units []state.UnitState) []string {
	out := make([]string, 0, len(units))
	for _, u := range units {
		out = append(out, u.Scope)
	}
	return out
}

// After a switch, reads answer for the record the checkout holds rather than
// the one the view was built from.
func TestWorkStore_ReadsFollowTheCheckedOutRecord(t *testing.T) {
	r := newBranchRepo(t)

	w := r.open()
	all, err := w.All(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"doc-feature"}, scopesOf(all))
	require.NoError(t, w.Close())

	r.git("switch", "main")

	w = r.open()
	defer func() { _ = w.Close() }()
	all, err = w.All(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"doc-main"}, scopesOf(all),
		"the view answers for the record this checkout holds")
}

// Writing the record after a switch writes into the checked-out branch's
// shards and leaves them alone: the shard this branch holds is not pruned, and
// the other branch's record is not published here.
func TestWorkStore_CommitDoesNotCarryTheOtherBranchesShards(t *testing.T) {
	r := newBranchRepo(t)

	w := r.open()
	require.NoError(t, w.Put(t.Context(), unit("u-decided", "doc-feature", "Charlie")))
	require.NoError(t, w.Close())

	r.git("switch", "main")

	w = r.open()
	defer func() { _ = w.Close() }()
	require.NoError(t, w.Commit(t.Context()))

	assert.FileExists(t, filepath.Join(r.record, "doc-main.jsonl"),
		"the checked-out branch keeps the shard its record holds")

	onDisk, err := state.ReadCommitted(r.record)
	require.NoError(t, err)
	assert.Contains(t, scopesOf(onDisk), "doc-main",
		"the record this branch holds survives the write")

	// A decision recorded here and not yet written out follows the person who
	// made it. What must not follow is the OTHER branch's record: the lines the
	// import supplied.
	var stray int
	for _, u := range onDisk {
		if u.Unit == "u-feature" {
			stray++
		}
	}
	assert.Zero(t, stray, "lines imported from the other branch's record are not published here")
}

// A decision made on one branch is not lost by switching away and back: the
// ledger keeps the entry, and the branch's own shards bring the pairing back,
// so the entry applies again.
func TestWorkStore_ADecisionReturnsWithItsBranch(t *testing.T) {
	r := newBranchRepo(t)

	decided := unit("u-feature", "doc-feature", "Bravo")
	decided.Decision.Note = "reviewed here"

	w := r.open()
	require.NoError(t, w.Put(t.Context(), decided))
	require.NoError(t, w.Commit(t.Context()))
	require.NoError(t, w.Close())
	r.git("add", "-A", ".kapi")
	r.git("commit", "-m", "review on feature")

	r.git("switch", "main")
	w = r.open()
	_, found := w.Get(t.Context(), nbKey("doc-feature", "u-feature"))
	assert.False(t, found, "the other branch's unit does not answer here")
	require.NoError(t, w.Close())

	r.git("switch", "feature")
	w = r.open()
	defer func() { _ = w.Close() }()
	got, found := w.Get(t.Context(), nbKey("doc-feature", "u-feature"))
	require.True(t, found, "the branch that holds the pairing gets its decision back")
	assert.Equal(t, "reviewed here", got.Decision.Note)
}

// A record found where the view left it is not re-imported, so the ordinary
// open costs a digest and nothing else.
func TestWorkStore_AnUnchangedRecordIsNotReimported(t *testing.T) {
	r := newBranchRepo(t)

	w := r.open()
	require.NoError(t, w.Close())

	w = r.open()
	defer func() { _ = w.Close() }()

	all, err := w.All(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"doc-feature"}, scopesOf(all))

	diff, err := w.RecordDiff(t.Context())
	require.NoError(t, err)
	assert.Zero(t, diff.Changed())
}

// Writing the record re-stamps what it just wrote, so the next open reads the
// view as current rather than importing all over again.
func TestWorkStore_CommitLeavesTheViewAgreedWithTheRecord(t *testing.T) {
	r := newBranchRepo(t)

	w := r.open()
	require.NoError(t, w.Put(t.Context(), unit("u-decided", "doc-feature", "Charlie")))
	require.NoError(t, w.Commit(t.Context()))
	require.NoError(t, w.Close())

	w = r.open()
	defer func() { _ = w.Close() }()
	diff, err := w.RecordDiff(t.Context())
	require.NoError(t, err)
	assert.Zero(t, diff.Changed())
}
