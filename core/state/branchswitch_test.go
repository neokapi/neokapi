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

// The committed record is git-tracked, so it is replaced wholesale by a branch
// switch while the working set, which is not tracked, keeps the rows of the
// branch left behind. These tests drive that with a real repository: two
// branches whose records name different documents, and one working store that
// outlives the switch between them.

// branchRepo is a git repository with a committed record on each of two
// branches, plus the path of the record directory and of the working store that
// spans them.
type branchRepo struct {
	t       *testing.T
	root    string
	record  string // the committed record directory, .kapi/state
	storeDB string // the working store, .kapi/work/store.db
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
// record mentions the other's document, which is what makes a commit from the
// wrong set visible as both a stray shard and a pruned one.
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

// open returns a working store over the record as this checkout holds it. The
// database file is the same one every time, which is the whole point: it is not
// tracked, so it survives the switch.
func (r *branchRepo) open() *state.WorkStore {
	r.t.Helper()
	w, err := state.OpenWork(r.t.Context(), r.storeDB, r.record)
	require.NoError(r.t, err)
	return w
}

func scopesOf(units []state.UnitState) []string {
	out := make([]string, 0, len(units))
	for _, u := range units {
		out = append(out, u.Scope)
	}
	return out
}

// (a) After a switch, reads reflect the record the checkout holds rather than
// the one the set was seeded from.
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
		"the set answers for the record this checkout holds")
}

// (b) Committing after a switch writes into the checked-out branch's record and
// leaves its shards alone: no shard of this branch is pruned, and no shard of
// the other branch is written here.
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
	scopes := scopesOf(onDisk)
	assert.Contains(t, scopes, "doc-main",
		"the record this branch holds survives the commit")

	// The staged decision travels with the person who made it, so it lands
	// here. What must not land is the OTHER branch's record: the seeded rows.
	var stray int
	for _, u := range onDisk {
		if u.Scope == "doc-feature" && u.Unit == "u-feature" {
			stray++
		}
	}
	assert.Zero(t, stray, "rows seeded from the other branch's record are not published here")
}

// (c) A staged decision crosses the rebuild untouched and the crossing is
// reported, because it was made against a record this checkout does not hold.
func TestWorkStore_StagedDecisionsCrossTheRebuildAndAreReported(t *testing.T) {
	r := newBranchRepo(t)

	w := r.open()
	require.NoError(t, w.Put(t.Context(), unit("u-decided", "doc-feature", "Charlie")))
	pending, err := w.Pending(t.Context())
	require.NoError(t, err)
	require.Equal(t, 1, pending)
	require.NoError(t, w.Close())

	r.git("switch", "main")

	w = r.open()
	defer func() { _ = w.Close() }()

	reseed := w.Reseed()
	assert.True(t, reseed.Reseeded, "the record moved under the set")
	assert.Equal(t, 1, reseed.Carried, "the staged decision is counted across")

	pending, err = w.Pending(t.Context())
	require.NoError(t, err)
	assert.Equal(t, 1, pending, "the staged decision is still staged")

	got, found := w.Get(t.Context(), state.Key{
		Scope: "doc-feature", Unit: "u-decided", Variant: unit("", "", "").Variant,
	})
	require.True(t, found, "the staged decision survived the rebuild")
	assert.Equal(t, "approved", got.Decision.ReviewState)
}

// (d) A record found where the set left it is not rebuilt, so the ordinary open
// costs a digest and nothing else, and reports nothing.
func TestWorkStore_AnUnchangedRecordIsNotReseeded(t *testing.T) {
	r := newBranchRepo(t)

	w := r.open()
	require.NoError(t, w.Close())

	w = r.open()
	defer func() { _ = w.Close() }()
	assert.False(t, w.Reseed().Reseeded, "nothing moved, so nothing was rebuilt")

	all, err := w.All(t.Context())
	require.NoError(t, err)
	assert.Equal(t, []string{"doc-feature"}, scopesOf(all))
}

// A commit re-stamps the record it just wrote, so the next open reads the set as
// current. Without that every commit would make the following command announce
// a record that moved.
func TestWorkStore_CommitLeavesTheSetAgreedWithTheRecord(t *testing.T) {
	r := newBranchRepo(t)

	w := r.open()
	require.NoError(t, w.Put(t.Context(), unit("u-decided", "doc-feature", "Charlie")))
	require.NoError(t, w.Commit(t.Context()))
	require.NoError(t, w.Close())

	w = r.open()
	defer func() { _ = w.Close() }()
	assert.False(t, w.Reseed().Reseeded)
}
