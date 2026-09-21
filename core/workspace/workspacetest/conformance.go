// Package workspacetest is the conformance suite every workspace backend must
// pass.
//
// A backend decides where a workspace's authoritative copy lives. The callers
// above it (core/projectdb, the host) are written against the interface and
// nothing else, so what an adapter owes them has to be stated somewhere both an
// adapter author and a reviewer can run. This is that statement: one table of
// behaviours, driven against whatever backend the factory hands back.
//
// The local adapter runs it. An adapter that keeps the authoritative copy
// elsewhere runs the same suite, which is how "it behaves like a workspace"
// stops being a claim and becomes a test result.
package workspacetest

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/workspace"
)

// Factory builds a fresh, empty backend for one test. It is called once per
// case, and the suite closes what it returns.
type Factory func(t *testing.T) workspace.Backend

// RunConformance drives every behaviour a workspace backend owes its callers.
//
// Each case gets its own backend, so a case that leaves a workspace in some
// state cannot decide what the next one sees.
func RunConformance(t *testing.T, newBackend Factory) {
	t.Helper()
	cases := []struct {
		name string
		run  func(t *testing.T, b workspace.Backend)
	}{
		{"describes itself", describesItself},
		{"the registry is one handle", registryIsOneHandle},
		{"a project store is one handle per key", projectStoreIsOneHandlePerKey},
		{"projects do not share a store", projectsDoNotShareAStore},
		{"a project store needs a key", projectStoreNeedsAKey},
		{"a project store keeps what is written to it", projectStoreKeepsWrites},
		{"operations are numbered in the order they arrive", operationsAreNumbered},
		{"operations read back from a position", operationsReadBackFromAPosition},
		{"the head position follows what was recorded", headFollowsWhatWasRecorded},
		{"an operation needs a kind", operationNeedsAKind},
		{"the registry records a project", registryRecordsAProject},
		{"re-registering updates the name and adds the checkout", reRegisteringUpdates},
		{"re-opening an unchanged project moves nothing", reopeningRecordsNothing},
		{"a project registers with no checkout", registersWithNoCheckout},
		{"two projects register in one workspace", twoProjectsRegister},
		{"a project with no registration is not found", unregisteredProjectIsNotFound},
		{"close is idempotent", closeIsIdempotent},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := newBackend(t)
			t.Cleanup(func() { _ = b.Close() })
			tc.run(t, b)
		})
	}
}

func describesItself(t *testing.T, b workspace.Backend) {
	d := b.Describe()
	assert.NotEmpty(t, d.Kind, "a backend names its kind")
	assert.NotEmpty(t, d.Location, "a backend says where the workspace is")
	assert.False(t, d.ReadOnly, "a fresh workspace accepts writes")
}

func registryIsOneHandle(t *testing.T, b workspace.Backend) {
	ctx := t.Context()
	first, err := b.Registry(ctx)
	require.NoError(t, err)
	require.NotNil(t, first)
	second, err := b.Registry(ctx)
	require.NoError(t, err)
	assert.Same(t, first, second, "the workspace database is opened once")
}

func projectStoreIsOneHandlePerKey(t *testing.T, b workspace.Backend) {
	ctx := t.Context()
	first, err := b.Project(ctx, "prj_conformance")
	require.NoError(t, err)
	require.NotNil(t, first)
	second, err := b.Project(ctx, "prj_conformance")
	require.NoError(t, err)
	assert.Same(t, first, second, "one key is one pool")
}

func projectsDoNotShareAStore(t *testing.T, b workspace.Backend) {
	ctx := t.Context()
	one, err := b.Project(ctx, "prj_one")
	require.NoError(t, err)
	two, err := b.Project(ctx, "prj_two")
	require.NoError(t, err)
	assert.NotSame(t, one, two, "two projects are two stores")

	_, err = one.ExecContext(ctx, `CREATE TABLE conformance (v TEXT)`)
	require.NoError(t, err)
	_, err = one.ExecContext(ctx, `INSERT INTO conformance (v) VALUES ('one')`)
	require.NoError(t, err)

	var count int
	err = two.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'conformance'`).Scan(&count)
	require.NoError(t, err)
	assert.Zero(t, count, "what one project holds is not visible in another")
}

func projectStoreNeedsAKey(t *testing.T, b workspace.Backend) {
	_, err := b.Project(t.Context(), "")
	assert.ErrorIs(t, err, workspace.ErrNoProjectKey)
}

func projectStoreKeepsWrites(t *testing.T, b workspace.Backend) {
	ctx := t.Context()
	db, err := b.Project(ctx, "prj_durable")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `CREATE TABLE kept (v TEXT)`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO kept (v) VALUES ('yes')`)
	require.NoError(t, err)

	var v string
	require.NoError(t, db.QueryRowContext(ctx, `SELECT v FROM kept`).Scan(&v))
	assert.Equal(t, "yes", v)
}

func operationsAreNumbered(t *testing.T, b workspace.Backend) {
	ctx := t.Context()
	recorded, err := b.Record(ctx,
		workspace.Op{Project: "prj_a", Kind: "first", Payload: []byte(`{"n":1}`)},
		workspace.Op{Project: "prj_b", Kind: "second"},
	)
	require.NoError(t, err)
	require.Len(t, recorded, 2)
	assert.Less(t, recorded[0].Seq, recorded[1].Seq, "sequence numbers rise in arrival order")
	assert.Equal(t, workspace.ProjectKey("prj_a"), recorded[0].Project)
	assert.Equal(t, "first", recorded[0].Kind)
	assert.JSONEq(t, `{"n":1}`, string(recorded[0].Payload))
	assert.False(t, recorded[0].At.IsZero(), "an operation carries when it was accepted")
	assert.Equal(t, time.UTC, recorded[0].At.Location())

	empty, err := b.Record(ctx)
	require.NoError(t, err)
	assert.Empty(t, empty, "recording nothing is not an error")
}

func operationsReadBackFromAPosition(t *testing.T, b workspace.Backend) {
	ctx := t.Context()
	recorded, err := b.Record(ctx,
		workspace.Op{Kind: "one"}, workspace.Op{Kind: "two"}, workspace.Op{Kind: "three"})
	require.NoError(t, err)
	require.Len(t, recorded, 3)

	all, err := b.Since(ctx, 0, 0)
	require.NoError(t, err)
	require.Len(t, all, 3)
	assert.Equal(t, []string{"one", "two", "three"}, kinds(all))

	after, err := b.Since(ctx, recorded[0].Seq, 0)
	require.NoError(t, err)
	assert.Equal(t, []string{"two", "three"}, kinds(after))

	limited, err := b.Since(ctx, 0, 2)
	require.NoError(t, err)
	assert.Equal(t, []string{"one", "two"}, kinds(limited))

	none, err := b.Since(ctx, recorded[2].Seq, 0)
	require.NoError(t, err)
	assert.Empty(t, none, "reading past the end returns nothing")
}

func headFollowsWhatWasRecorded(t *testing.T, b workspace.Backend) {
	ctx := t.Context()
	start, err := b.Head(ctx)
	require.NoError(t, err)
	assert.Zero(t, start, "a workspace with an empty log is at position zero")

	recorded, err := b.Record(ctx, workspace.Op{Kind: "one"}, workspace.Op{Kind: "two"})
	require.NoError(t, err)
	require.Len(t, recorded, 2)

	head, err := b.Head(ctx)
	require.NoError(t, err)
	assert.Equal(t, recorded[1].Seq, head, "the head is the last sequence number assigned")

	again, err := b.Head(ctx)
	require.NoError(t, err)
	assert.Equal(t, head, again, "reading the head records nothing")
}

func operationNeedsAKind(t *testing.T, b workspace.Backend) {
	_, err := b.Record(t.Context(), workspace.Op{Project: "prj_a"})
	assert.Error(t, err, "an operation with no kind is refused")
}

func registryRecordsAProject(t *testing.T, b workspace.Backend) {
	ctx := t.Context()
	w, err := workspace.Open(ctx, b)
	require.NoError(t, err)

	before := time.Now().UTC().Add(-time.Second)
	reg, err := w.Register(ctx, "prj_registered", "Docs", "/fakehome/src/docs")
	require.NoError(t, err)
	assert.Equal(t, workspace.ProjectKey("prj_registered"), reg.Key)
	assert.Equal(t, "Docs", reg.Name)
	assert.Equal(t, []string{"/fakehome/src/docs"}, reg.Checkouts)
	assert.False(t, reg.LastActive.Before(before), "the registration is stamped with now")

	found, ok, err := w.Lookup(ctx, "prj_registered")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, reg, found)

	ops, err := w.Ops(ctx, 0, 0)
	require.NoError(t, err)
	require.NotEmpty(t, ops)
	assert.Equal(t, workspace.OpRegisterProject, ops[len(ops)-1].Kind,
		"registering is recorded in the operation log")
}

func reRegisteringUpdates(t *testing.T, b workspace.Backend) {
	ctx := t.Context()
	w, err := workspace.Open(ctx, b)
	require.NoError(t, err)

	_, err = w.Register(ctx, "prj_moving", "Docs", "/fakehome/src/docs")
	require.NoError(t, err)
	reg, err := w.Register(ctx, "prj_moving", "Documentation", "/fakehome/work/docs")
	require.NoError(t, err)
	assert.Equal(t, "Documentation", reg.Name, "the display name follows the recipe")
	assert.Equal(t, []string{"/fakehome/src/docs", "/fakehome/work/docs"}, reg.Checkouts,
		"a checkout is added, never replaced")

	again, err := w.Register(ctx, "prj_moving", "", "/fakehome/src/docs")
	require.NoError(t, err)
	assert.Equal(t, "Documentation", again.Name, "registering without a name keeps the one on record")
	assert.Equal(t, []string{"/fakehome/src/docs", "/fakehome/work/docs"}, again.Checkouts,
		"a checkout already on record is not duplicated")
}

// reopeningRecordsNothing holds the log to what changed.
//
// Opening a project registers it, and opening one is the most frequent thing
// that happens to a workspace. The log's position is what an answer reports as
// the state of the context it was read from, so a re-open that changed nothing
// leaves it where it was, and two reads of one unchanged workspace report one
// revision.
func reopeningRecordsNothing(t *testing.T, b workspace.Backend) {
	ctx := t.Context()
	w, err := workspace.Open(ctx, b)
	require.NoError(t, err)

	_, err = w.Register(ctx, "prj_reopened", "Docs", "/fakehome/src/docs")
	require.NoError(t, err)
	first, err := b.Head(ctx)
	require.NoError(t, err)
	assert.NotZero(t, first, "registering a project for the first time is recorded")

	_, err = w.Register(ctx, "prj_reopened", "Docs", "/fakehome/src/docs")
	require.NoError(t, err)
	again, err := b.Head(ctx)
	require.NoError(t, err)
	assert.Equal(t, first, again, "re-opening the same project from the same checkout records nothing")

	_, err = w.Register(ctx, "prj_reopened", "Documentation", "/fakehome/src/docs")
	require.NoError(t, err)
	renamed, err := b.Head(ctx)
	require.NoError(t, err)
	assert.Greater(t, renamed, again, "a changed display name is recorded")
}

// registersWithNoCheckout covers the project a workspace knows and this
// machine has no working tree for.
//
// It is how a restored backup puts a project back: the bundle carries the
// identity and the display name, and the directories the project was worked in
// belong to the machine it came from. Such a project is listed and has a
// context store like any other, and it collects checkout paths when something
// here opens it.
func registersWithNoCheckout(t *testing.T, b workspace.Backend) {
	ctx := t.Context()
	w, err := workspace.Open(ctx, b)
	require.NoError(t, err)

	reg, err := w.Register(ctx, "prj_restored", "Restored", "")
	require.NoError(t, err)
	assert.Equal(t, "Restored", reg.Name)
	assert.Empty(t, reg.Checkouts, "a project with no working tree here records no path")

	projects, err := w.Projects(ctx)
	require.NoError(t, err)
	require.Len(t, projects, 1, "a project with no checkout is still a project the workspace holds")
	assert.Equal(t, workspace.ProjectKey("prj_restored"), projects[0].Key)

	_, err = w.Context(ctx, "prj_restored")
	require.NoError(t, err, "its context store opens like any other")

	found, err := w.Register(ctx, "prj_restored", "Restored", "/fakehome/src/restored")
	require.NoError(t, err)
	assert.Equal(t, []string{"/fakehome/src/restored"}, found.Checkouts,
		"opening it here adds the checkout to the one it already had")
}

func twoProjectsRegister(t *testing.T, b workspace.Backend) {
	ctx := t.Context()
	w, err := workspace.Open(ctx, b)
	require.NoError(t, err)

	_, err = w.Register(ctx, "prj_one", "One", "/fakehome/src/one")
	require.NoError(t, err)
	_, err = w.Register(ctx, "prj_two", "Two", "/fakehome/src/two")
	require.NoError(t, err)

	projects, err := w.Projects(ctx)
	require.NoError(t, err)
	require.Len(t, projects, 2)
	assert.ElementsMatch(t,
		[]workspace.ProjectKey{"prj_one", "prj_two"},
		[]workspace.ProjectKey{projects[0].Key, projects[1].Key})

	one, err := w.Context(ctx, "prj_one")
	require.NoError(t, err)
	two, err := w.Context(ctx, "prj_two")
	require.NoError(t, err)
	assert.NotSame(t, one, two, "each registered project has a context store of its own")
}

func unregisteredProjectIsNotFound(t *testing.T, b workspace.Backend) {
	ctx := t.Context()
	w, err := workspace.Open(ctx, b)
	require.NoError(t, err)

	_, ok, err := w.Lookup(ctx, "prj_absent")
	require.NoError(t, err)
	assert.False(t, ok)

	projects, err := w.Projects(ctx)
	require.NoError(t, err)
	assert.Empty(t, projects, "an empty workspace lists no projects")

	_, err = w.Register(ctx, "", "Nameless", "/fakehome/src/x")
	assert.ErrorIs(t, err, workspace.ErrNoProjectKey)
}

func closeIsIdempotent(t *testing.T, b workspace.Backend) {
	ctx := t.Context()
	_, err := b.Registry(ctx)
	require.NoError(t, err)
	_, err = b.Project(ctx, "prj_closing")
	require.NoError(t, err)
	require.NoError(t, b.Close())
	assert.NoError(t, b.Close(), "closing twice is not an error")
}

// kinds renders the operation kinds of a batch, which is what most of the log
// assertions compare.
func kinds(ops []workspace.Op) []string {
	out := make([]string, len(ops))
	for i, op := range ops {
		out[i] = op.Kind
	}
	return out
}
