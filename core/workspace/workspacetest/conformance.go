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
		{"the log head follows the last operation", headFollowsTheLastOperation},
		{"an operation needs a kind", operationNeedsAKind},
		{"the registry records a project", registryRecordsAProject},
		{"re-registering updates the name and adds the checkout", reRegisteringUpdates},
		{"re-opening an unchanged project moves nothing", reopeningRecordsNothing},
		{"a project registers with no checkout", registersWithNoCheckout},
		{"two projects register in one workspace", twoProjectsRegister},
		{"a project with no registration is not found", unregisteredProjectIsNotFound},
		{"forgetting a project removes it and its store", forgettingRemovesTheProject},
		{"forgetting a project needs a key", forgettingNeedsAKey},
		{"a widened rule is held for the whole workspace", widenedRulesAreHeldForTheWorkspace},
		{"an agent session is noted and ages out", agentSessionsAreNotedAndAgeOut},
		{"an import stamp is kept per checkout", importStampsAreKeptPerCheckout},
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

// headFollowsTheLastOperation covers both readers of the head: a surface that
// polls it to learn something changed, and a retrieval answer that reports it
// as the revision it was read at.
func headFollowsTheLastOperation(t *testing.T, b workspace.Backend) {
	ctx := t.Context()
	start, err := b.Head(ctx)
	require.NoError(t, err)
	assert.Zero(t, start, "an empty log has no head")

	recorded, err := b.Record(ctx, workspace.Op{Kind: "one"}, workspace.Op{Kind: "two"})
	require.NoError(t, err)
	require.Len(t, recorded, 2)

	head, err := b.Head(ctx)
	require.NoError(t, err)
	assert.Equal(t, recorded[1].Seq, head, "the head is the last sequence number assigned")

	again, err := b.Head(ctx)
	require.NoError(t, err)
	assert.Equal(t, head, again, "reading the head records nothing")

	after, err := b.Since(ctx, start, 0)
	require.NoError(t, err)
	assert.Equal(t, []string{"one", "two"}, kinds(after),
		"a reader that held the old head reads exactly what it missed")
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

	require.NoError(t, w.Forget(ctx, "prj_reopened"))
	forgotten, err := b.Head(ctx)
	require.NoError(t, err)
	assert.Greater(t, forgotten, renamed, "forgetting a project is recorded")
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

func forgettingRemovesTheProject(t *testing.T, b workspace.Backend) {
	ctx := t.Context()
	w, err := workspace.Open(ctx, b)
	require.NoError(t, err)

	_, err = w.Register(ctx, "prj_gone", "Gone", "/fakehome/src/gone")
	require.NoError(t, err)
	_, err = w.Register(ctx, "prj_kept", "Kept", "/fakehome/src/kept")
	require.NoError(t, err)

	db, err := w.Context(ctx, "prj_gone")
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `CREATE TABLE decided (v TEXT)`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO decided (v) VALUES ('yes')`)
	require.NoError(t, err)

	require.NoError(t, w.Forget(ctx, "prj_gone"))

	_, ok, err := w.Lookup(ctx, "prj_gone")
	require.NoError(t, err)
	assert.False(t, ok, "a forgotten project is no longer registered")

	projects, err := w.Projects(ctx)
	require.NoError(t, err)
	require.Len(t, projects, 1, "only the forgotten project goes")
	assert.Equal(t, workspace.ProjectKey("prj_kept"), projects[0].Key)

	// The store behind it is gone too: opening the key again yields an empty one.
	fresh, err := w.Context(ctx, "prj_gone")
	require.NoError(t, err)
	var tables int
	require.NoError(t, fresh.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'decided'`).Scan(&tables))
	assert.Zero(t, tables, "the context the project had is discarded with it")

	ops, err := w.Ops(ctx, 0, 0)
	require.NoError(t, err)
	require.NotEmpty(t, ops)
	last := ops[len(ops)-1]
	assert.Equal(t, workspace.OpForgetProject, last.Kind, "the removal is in the log")
	assert.Equal(t, workspace.ProjectKey("prj_gone"), last.Project)

	assert.NoError(t, w.Forget(ctx, "prj_absent"),
		"forgetting a project the workspace never held is not an error")
}

func forgettingNeedsAKey(t *testing.T, b workspace.Backend) {
	ctx := t.Context()
	w, err := workspace.Open(ctx, b)
	require.NoError(t, err)
	require.ErrorIs(t, w.Forget(ctx, ""), workspace.ErrNoProjectKey)
	require.ErrorIs(t, b.Forget(ctx, ""), workspace.ErrNoProjectKey)
}

// widenedRulesAreHeldForTheWorkspace covers the one piece of context that
// belongs to no project: a rule a person deliberately put in force everywhere.
//
// Everything else a project learns lives in that project's own context store.
// A widened rule has nowhere there to live, so the workspace holds it, keeps
// the project its evidence came from, and hands it back to whatever knows what
// the bytes mean.
func widenedRulesAreHeldForTheWorkspace(t *testing.T, b workspace.Backend) {
	ctx := t.Context()
	w, err := workspace.Open(ctx, b)
	require.NoError(t, err)

	at := time.Now().UTC().Truncate(time.Second)
	require.NoError(t, w.WidenRule(ctx, workspace.Rule{
		ID:      "rule_vocabulary",
		Kind:    "vocabulary",
		Origin:  "prj_docs",
		Payload: []byte(`{"term":"utilise"}`),
		At:      at,
	}))
	require.NoError(t, w.WidenRule(ctx, workspace.Rule{
		ID:      "rule_other",
		Kind:    "something-else",
		Payload: []byte(`{}`),
		At:      at.Add(time.Second),
	}))

	held, err := w.WidenedRules(ctx, "vocabulary")
	require.NoError(t, err)
	require.Len(t, held, 1, "a listing narrowed to one kind holds only that kind")
	assert.Equal(t, "rule_vocabulary", held[0].ID)
	assert.Equal(t, workspace.ProjectKey("prj_docs"), held[0].Origin, "provenance survives widening")
	assert.JSONEq(t, `{"term":"utilise"}`, string(held[0].Payload))
	assert.Equal(t, at, held[0].At)

	all, err := w.WidenedRules(ctx, "")
	require.NoError(t, err)
	assert.Len(t, all, 2, "an unnarrowed listing holds every kind")

	require.NoError(t, w.WidenRule(ctx, workspace.Rule{
		ID: "rule_vocabulary", Kind: "vocabulary", Payload: []byte(`{"term":"leverage"}`), At: at,
	}))
	held, err = w.WidenedRules(ctx, "vocabulary")
	require.NoError(t, err)
	require.Len(t, held, 1, "widening the same rule again replaces it")
	assert.JSONEq(t, `{"term":"leverage"}`, string(held[0].Payload))

	require.NoError(t, w.NarrowRule(ctx, "rule_vocabulary"))
	held, err = w.WidenedRules(ctx, "vocabulary")
	require.NoError(t, err)
	assert.Empty(t, held, "narrowing takes the rule back out")
	require.NoError(t, w.NarrowRule(ctx, "rule_vocabulary"), "narrowing a rule the workspace does not hold is not an error")

	assert.ErrorIs(t, w.WidenRule(ctx, workspace.Rule{Kind: "vocabulary"}), workspace.ErrNoRuleID)
}

// agentSessionsAreNotedAndAgeOut covers what one process tells the others:
// that an agent is at work in a project right now.
//
// The note carries no work of its own, so a session that stops leaves a row
// that stops moving. The next write prunes whatever has gone quiet for longer
// than the retention window, which is what keeps the listing a picture of who
// is working rather than a history of everyone who ever did.
func agentSessionsAreNotedAndAgeOut(t *testing.T, b workspace.Backend) {
	ctx := t.Context()
	w, err := workspace.Open(ctx, b)
	require.NoError(t, err)

	started := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	require.NoError(t, w.NoteAgentSession(ctx, workspace.AgentSession{
		ID: "ses_writing", Project: "prj_docs", Agent: "claude-code",
		Started: started, LastSeen: started,
	}))

	held, err := w.AgentSessions(ctx, "prj_docs", 0)
	require.NoError(t, err)
	require.Len(t, held, 1)
	assert.Equal(t, "ses_writing", held[0].ID)
	assert.Equal(t, "claude-code", held[0].Agent, "the session says which client it belongs to")
	assert.Equal(t, started, held[0].Started)

	// Noting the same session again moves last-seen forward and keeps the
	// moment it began, so a surface can say how long it has been working.
	seen := started.Add(30 * time.Minute)
	require.NoError(t, w.NoteAgentSession(ctx, workspace.AgentSession{
		ID: "ses_writing", Project: "prj_docs", LastSeen: seen,
	}))
	held, err = w.AgentSessions(ctx, "prj_docs", 0)
	require.NoError(t, err)
	require.Len(t, held, 1, "one session in one project is one row")
	assert.Equal(t, seen, held[0].LastSeen)
	assert.Equal(t, started, held[0].Started, "the start is kept")
	assert.Equal(t, "claude-code", held[0].Agent, "a note with no name keeps the one already recorded")

	// One session working in two projects is two rows, because what a surface
	// shows is who is working here.
	require.NoError(t, w.NoteAgentSession(ctx, workspace.AgentSession{
		ID: "ses_writing", Project: "prj_site", LastSeen: seen,
	}))
	held, err = w.AgentSessions(ctx, "prj_docs", 0)
	require.NoError(t, err)
	assert.Len(t, held, 1, "a listing narrowed to one project holds only that project")
	all, err := w.AgentSessions(ctx, "", 0)
	require.NoError(t, err)
	assert.Len(t, all, 2)

	// A window keeps the sessions seen inside it and nothing else.
	active, err := w.AgentSessions(ctx, "", time.Minute)
	require.NoError(t, err)
	assert.Empty(t, active, "a session last seen half an hour ago is not working now")

	// A later write prunes whatever has been quiet past the retention window.
	require.NoError(t, w.NoteAgentSession(ctx, workspace.AgentSession{
		ID: "ses_later", Project: "prj_docs",
		LastSeen: seen.Add(workspace.AgentSessionRetention + time.Hour),
	}))
	all, err = w.AgentSessions(ctx, "", 0)
	require.NoError(t, err)
	require.Len(t, all, 1, "the quiet sessions were pruned by the write")
	assert.Equal(t, "ses_later", all[0].ID)

	assert.ErrorIs(t, w.NoteAgentSession(ctx, workspace.AgentSession{Project: "prj_docs"}), workspace.ErrNoSessionID)
}

func importStampsAreKeptPerCheckout(t *testing.T, b workspace.Backend) {
	ctx := t.Context()
	w, err := workspace.Open(ctx, b)
	require.NoError(t, err)

	read := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	require.NoError(t, w.NoteContextImports(ctx,
		workspace.ContextImportStamp{Project: "prj_docs", Checkout: "/w/main", Path: ".kapi/terms.json", Digest: "aa", At: read},
		workspace.ContextImportStamp{Project: "prj_docs", Checkout: "/w/main", Path: ".kapi/voice.yaml", Digest: "bb", At: read},
	))

	held, err := w.ContextImports(ctx, "prj_docs", "/w/main")
	require.NoError(t, err)
	require.Len(t, held, 2)
	assert.Equal(t, ".kapi/terms.json", held[0].Path, "stamps read back in path order")
	assert.Equal(t, "aa", held[0].Digest)
	assert.Equal(t, read, held[0].At)

	// A second checkout of the same project reads its own files, so a stamp one
	// clone wrote says nothing about another's.
	other, err := w.ContextImports(ctx, "prj_docs", "/w/branch")
	require.NoError(t, err)
	assert.Empty(t, other, "a checkout that has read nothing has no stamps")

	// A checkout whose recipe names another project reads its files into that
	// project's store rather than skipping them.
	elsewhere, err := w.ContextImports(ctx, "prj_site", "/w/main")
	require.NoError(t, err)
	assert.Empty(t, elsewhere, "a stamp belongs to the project it was read into")

	require.NoError(t, w.NoteContextImports(ctx,
		workspace.ContextImportStamp{Project: "prj_docs", Checkout: "/w/branch", Path: ".kapi/terms.json", Digest: "cc", At: read}))
	other, err = w.ContextImports(ctx, "prj_docs", "/w/branch")
	require.NoError(t, err)
	require.Len(t, other, 1)
	assert.Equal(t, "cc", other[0].Digest)

	// Reading the same file again at different bytes replaces the stamp.
	later := read.Add(time.Hour)
	require.NoError(t, w.NoteContextImports(ctx,
		workspace.ContextImportStamp{Project: "prj_docs", Checkout: "/w/main", Path: ".kapi/terms.json", Digest: "dd", At: later}))
	held, err = w.ContextImports(ctx, "prj_docs", "/w/main")
	require.NoError(t, err)
	require.Len(t, held, 2, "one file in one checkout is one row")
	assert.Equal(t, "dd", held[0].Digest)
	assert.Equal(t, later, held[0].At)

	require.NoError(t, w.NoteContextImports(ctx), "noting nothing is not an error")
	require.ErrorIs(t, w.NoteContextImports(ctx,
		workspace.ContextImportStamp{Path: ".kapi/terms.json"}), workspace.ErrNoImportStamp)
	require.ErrorIs(t, w.NoteContextImports(ctx,
		workspace.ContextImportStamp{Checkout: "/w/main"}), workspace.ErrNoImportStamp)
	_, err = w.ContextImports(ctx, "prj_docs", "")
	require.ErrorIs(t, err, workspace.ErrNoImportStamp)
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
