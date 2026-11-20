package knowledge

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/termscope"
	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
)

func TestStartStopPilot_ConceptsAndRelations(t *testing.T) {
	ctx := context.Background()
	ws := "ws"
	pilotStream := "pilot/rebrand"

	tb := newSQLiteTB(t)
	require.NoError(t, tb.AddConcept(ctx, concept("old", term("kaputt", "en-US", model.TermDeprecated))))
	require.NoError(t, tb.AddConcept(ctx, concept("new", term("fixed", "en-US", model.TermPreferred))))

	store := newMemStore()
	cs := &ChangeSet{ID: "cs1", WorkspaceID: ws, Name: "Guide to new", CreatedBy: "alice"}
	require.NoError(t, store.CreateChangeSet(ctx, cs))
	appendOp(t, store, ws, cs.ID, 0, OpRelationAdd, RelationAddPayload{
		Relation: terms.ConceptRelation{ID: "r1", SourceID: "old", TargetID: "new", RelationType: graph.LabelUseInstead},
	})

	loaded, err := store.GetChangeSet(ctx, ws, cs.ID)
	require.NoError(t, err)

	e := NewEngine(nil, tb, store)
	pilot, err := e.StartPilot(ctx, ws, store, *loaded, "proj1", pilotStream)
	require.NoError(t, err)

	// Shadow concepts exist under namespaced IDs.
	oldID := pilot.shadowID("c", "old")
	newID := pilot.shadowID("c", "new")
	sc, ok, err := tb.GetConcept(ctx, oldID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Len(t, sc.Terms, 1)
	assert.Equal(t, "kaputt", sc.Terms[0].Text)
	_, ok, err = tb.GetConcept(ctx, newID)
	require.NoError(t, err)
	require.True(t, ok)

	// The shadow relation links the shadow concepts.
	rels, err := tb.RelationsForStream(ctx, oldID, pilotStream, nil, nil)
	require.NoError(t, err)
	require.Len(t, rels, 1)
	assert.Equal(t, graph.LabelUseInstead, rels[0].RelationType)
	assert.Equal(t, oldID, rels[0].SourceID)
	assert.Equal(t, newID, rels[0].TargetID)

	rels, err = tb.RelationsOf(ctx, oldID, nil)
	require.NoError(t, err)
	assert.Empty(t, rels)
	rels, err = tb.RelationsForStream(ctx, oldID, "main", nil, nil)
	require.NoError(t, err)
	assert.Empty(t, rels)

	// The live graph is untouched.
	liveOld, ok, err := tb.GetConcept(ctx, "old")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, model.TermDeprecated, liveOld.Terms[0].Status)
	liveRels, err := tb.RelationsOf(ctx, "old", nil)
	require.NoError(t, err)
	assert.Empty(t, liveRels, "the change-set's relation is not in the live graph")

	pilots, err := store.ListPilots(ctx, ws, cs.ID)
	require.NoError(t, err)
	require.Len(t, pilots, 1)

	// Stop the pilot — shadow concepts and relations are removed.
	require.NoError(t, e.StopPilot(ctx, ws, store, *loaded, "proj1", pilotStream))
	rels, err = tb.RelationsForStream(ctx, oldID, pilotStream, nil, nil)
	require.NoError(t, err)
	assert.Empty(t, rels)
	_, ok, err = tb.GetConcept(ctx, oldID)
	require.NoError(t, err)
	assert.False(t, ok)
	_, ok, err = tb.GetConcept(ctx, newID)
	require.NoError(t, err)
	assert.False(t, ok)
	liveRels, err = tb.RelationsOf(ctx, "old", nil)
	require.NoError(t, err)
	assert.Empty(t, liveRels)
	pilots, err = store.ListPilots(ctx, ws, cs.ID)
	require.NoError(t, err)
	assert.Empty(t, pilots)

	// The live concepts survive the pilot teardown.
	_, ok, err = tb.GetConcept(ctx, "old")
	require.NoError(t, err)
	assert.True(t, ok)

	// StopPilot is idempotent.
	require.NoError(t, e.StopPilot(ctx, ws, store, *loaded, "proj1", pilotStream))
}

func TestStopAllPilots_StopsEveryBoundStream(t *testing.T) {
	ctx := context.Background()
	ws := "ws"

	tb := newSQLiteTB(t)
	require.NoError(t, tb.AddConcept(ctx, concept("c1", term("widget", "en-US", model.TermApproved))))

	store := newMemStore()
	cs := &ChangeSet{ID: "cs1", WorkspaceID: ws, Name: "Add term", CreatedBy: "alice"}
	require.NoError(t, store.CreateChangeSet(ctx, cs))
	appendOp(t, store, ws, cs.ID, 0, OpTermAdd, TermAddPayload{
		ConceptID: "c1", Term: term("widgets", "en-GB", model.TermAdmitted),
	})

	loaded, err := store.GetChangeSet(ctx, ws, cs.ID)
	require.NoError(t, err)

	e := NewEngine(nil, tb, store)
	pilotA, err := e.StartPilot(ctx, ws, store, *loaded, "projA", "pilot/a")
	require.NoError(t, err)
	_, err = e.StartPilot(ctx, ws, store, *loaded, "projB", "pilot/b")
	require.NoError(t, err)

	stopped, events, err := e.StopAllPilots(ctx, ws, store, *loaded)
	require.NoError(t, err)
	assert.Equal(t, 2, stopped)
	require.Len(t, events, 2)
	for _, ev := range events {
		assert.Equal(t, EventPilotStopped, ev.Type)
	}

	pilots, err := store.ListPilots(ctx, ws, cs.ID)
	require.NoError(t, err)
	assert.Empty(t, pilots)
	_, ok, err := tb.GetConcept(ctx, pilotA.shadowID("c", "c1"))
	require.NoError(t, err)
	assert.False(t, ok)
}

// A pilot is bound to one stream. Its shadow entries must therefore be invisible
// to every read that does not name that stream — and the reads that matter most
// are the stream-blind ones the checks and the sync go through
// (Terminology.LookupAll, ConceptStore.Concepts). Without this, starting a pilot
// on a side branch makes the draft's forbidden terms bite on main, and drags
// machine-written duplicate concepts into every `kapi pull`.
func TestStartPilot_ShadowsAreInvisibleToStreamBlindReads(t *testing.T) {
	ctx := context.Background()
	ws := "ws"
	pilotStream := "pilot/rebrand"

	tb := newSQLiteTB(t)
	require.NoError(t, tb.AddConcept(ctx, concept("old", term("kaputt", "en-US", model.TermAdmitted))))

	store := newMemStore()
	cs := &ChangeSet{ID: "cs1", WorkspaceID: ws, Name: "Ban kaputt", CreatedBy: "alice"}
	require.NoError(t, store.CreateChangeSet(ctx, cs))
	appendOp(t, store, ws, cs.ID, 0, OpTermStatus, TermStatusPayload{
		ConceptID: "old", Locale: "en-US", Text: "kaputt",
		From: model.TermAdmitted, To: model.TermForbidden,
	})
	loaded, err := store.GetChangeSet(ctx, ws, cs.ID)
	require.NoError(t, err)

	before, err := tb.Concepts(ctx)
	require.NoError(t, err)
	beforeMatches, err := tb.LookupAll(ctx, "the kaputt thing", terms.LookupOptions{SourceLocale: "en-US"})
	require.NoError(t, err)

	e := NewEngine(nil, tb, store)
	pilot, err := e.StartPilot(ctx, ws, store, *loaded, "proj1", pilotStream)
	require.NoError(t, err)

	// The shadow was written — the pilot did its job.
	_, ok, err := tb.GetConcept(ctx, pilot.shadowID("c", "old"))
	require.NoError(t, err)
	require.True(t, ok, "the pilot must write its stream shadow")

	after, err := tb.Concepts(ctx)
	require.NoError(t, err)
	assert.Len(t, after, len(before), "a pilot adds nothing to the workspace's concepts")
	for _, c := range after {
		assert.False(t, terms.IsShadowID(c.ID), "shadow %q escaped into a stream-blind listing", c.ID)
	}

	afterMatches, err := tb.LookupAll(ctx, "the kaputt thing", terms.LookupOptions{SourceLocale: "en-US"})
	require.NoError(t, err)
	assert.Len(t, afterMatches, len(beforeMatches),
		"a pilot must not change what a stream-blind lookup — every check's terms read — finds")
	for _, m := range afterMatches {
		assert.NotEqual(t, model.TermForbidden, m.Term.Status,
			"the draft's ban reached a lookup nobody bound the pilot to")
	}
}

func TestPilotProjectsSharingAStream(t *testing.T) {
	t.Run("SQLite", func(t *testing.T) {
		testPilotProjectsSharingAStream(t, newSQLiteTB(t), newMemStore())
	})
	t.Run("Store", func(t *testing.T) {
		testPilotProjectsSharingAStream(t, struct{ terms.Store }{newSQLiteTB(t)}, newMemStore())
	})
}

func testPilotProjectsSharingAStream(t *testing.T, tb terms.Store, store Store) {
	t.Helper()
	ctx := t.Context()
	const stream = "pilot:copy"
	cs := ChangeSet{ID: "cs:1", WorkspaceID: "ws", Name: "New guidance", CreatedBy: "alice"}
	require.NoError(t, store.CreateChangeSet(ctx, &cs))
	replacement := concept("new", term("replacement", "en-US", model.TermPreferred))
	replacement.Properties = map[string]string{termscope.PropProjects: "project:A,project:B"}
	restricted := concept("local", term("restricted", "en-US", model.TermApproved))
	restricted.Properties = map[string]string{termscope.PropProjects: "project:A"}
	for _, cp := range []terms.Concept{concept("old:term", term("original", "en-US", model.TermDeprecated)), replacement, restricted} {
		require.NoError(t, tb.AddConcept(ctx, cp))
	}
	appendOp(t, store, cs.WorkspaceID, cs.ID, 0, OpRelationAdd, RelationAddPayload{
		Relation: terms.ConceptRelation{ID: "r:1", SourceID: "old:term", TargetID: "new", RelationType: graph.LabelUseInstead},
	})
	appendOp(t, store, cs.WorkspaceID, cs.ID, 0, OpRelationAdd, RelationAddPayload{
		Relation: terms.ConceptRelation{ID: "local", SourceID: "old:term", TargetID: "local", RelationType: graph.LabelUseInstead},
	})
	appendOp(t, store, cs.WorkspaceID, cs.ID, 0, OpConceptUpdate, ConceptUpdatePayload{ConceptID: "local", Properties: map[string]string{}})

	engine := NewEngine(nil, tb, store)
	start := func(projectID string) *Pilot {
		p, err := engine.StartPilot(ctx, cs.WorkspaceID, store, cs, projectID, stream)
		require.NoError(t, err)
		return p
	}
	check := func(projectID string, pilot *Pilot, want []string, numRelations int) {
		t.Helper()
		view := termscope.StoreView(tb, projectID)
		page, total, err := view.SearchForStream(ctx, "", "", "", stream, nil, 0, 20)
		require.NoError(t, err)
		assert.Equal(t, len(want), total)
		var ids []string
		for _, cp := range page {
			assert.Equal(t, []string{projectID}, termscope.Projects(cp))
			ids = append(ids, cp.ID)
		}
		assert.ElementsMatch(t, want, ids)
		rels, err := view.RelationsForStream(ctx, pilot.shadowID("c", "old:term"), stream, nil, nil)
		require.NoError(t, err)
		assert.Len(t, rels, numRelations)
		for _, rel := range rels {
			assert.Contains(t, ids, rel.TargetID)
		}
	}
	a := start("project:A")
	b := start("project:B")
	aIDs := []string{a.shadowID("c", "old:term"), a.shadowID("c", "new"), a.shadowID("c", "local")}
	bIDs := []string{b.shadowID("c", "old:term"), b.shadowID("c", "new")}
	if bulk, ok := tb.(interface {
		DeleteShadowConcepts(context.Context, string) error
	}); ok {
		require.Error(t, bulk.DeleteShadowConcepts(ctx, ""))
	}
	check(a.ProjectID, a, aIDs, 2)
	check(b.ProjectID, b, bIDs, 1)
	check("unbound", a, nil, 0)
	live, ok, err := tb.GetConcept(ctx, "new")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, []string{a.ProjectID, b.ProjectID}, termscope.Projects(live))

	ops, err := store.ListOps(ctx, cs.WorkspaceID, cs.ID)
	require.NoError(t, err)
	for _, op := range ops {
		require.NoError(t, store.RemoveOp(ctx, cs.WorkspaceID, cs.ID, op.Seq))
	}
	start(a.ProjectID)
	check(a.ProjectID, a, nil, 0)
	check(b.ProjectID, b, bIDs, 1)
	for range 2 {
		require.NoError(t, engine.StopPilot(ctx, cs.WorkspaceID, store, cs, a.ProjectID, stream))
		check(b.ProjectID, b, bIDs, 1)
	}
	require.NoError(t, engine.StopPilot(ctx, cs.WorkspaceID, store, cs, b.ProjectID, stream))
	check(b.ProjectID, b, nil, 0)
}

func TestPilotUsesPersistedChangeSetStatus(t *testing.T) {
	for _, status := range []ChangeSetStatus{ChangeSetDraft, ChangeSetInReview, ChangeSetApproved, ChangeSetMerged, ChangeSetAbandoned, ChangeSetSuperseded} {
		t.Run(string(status), func(t *testing.T) {
			ctx := t.Context()
			store := newMemStore()
			cs := ChangeSet{ID: "draft", WorkspaceID: "ws", Status: status}
			require.NoError(t, store.CreateChangeSet(ctx, &cs))
			engine := NewEngine(nil, newSQLiteTB(t), store)
			switch status {
			case ChangeSetDraft, ChangeSetInReview, ChangeSetApproved:
				cs.Status = ChangeSetMerged
				_, err := engine.StartPilot(ctx, cs.WorkspaceID, store, cs, "project", "main")
				require.NoError(t, err)
			default:
				cs.Status = ChangeSetDraft
				_, err := engine.StartPilot(ctx, cs.WorkspaceID, store, cs, "project", "main")
				require.ErrorIs(t, err, ErrPilotChangeSetClosed)
				_, err = engine.AbandonChangeSet(ctx, cs.WorkspaceID, store, cs)
				require.ErrorIs(t, err, ErrPilotChangeSetClosed)
				pilots, err := store.ListPilots(ctx, cs.WorkspaceID, cs.ID)
				require.NoError(t, err)
				assert.Empty(t, pilots)
			}
		})
	}
}
