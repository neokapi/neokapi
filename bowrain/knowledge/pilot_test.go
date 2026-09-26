package knowledge

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	_, err = e.StartPilot(ctx, ws, store, *loaded, "proj1", pilotStream)
	require.NoError(t, err)

	// Shadow concepts exist under namespaced IDs.
	oldID := pilotConceptID(cs.ID, pilotStream, "old")
	newID := pilotConceptID(cs.ID, pilotStream, "new")
	sc, ok, err := tb.GetConcept(ctx, oldID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Len(t, sc.Terms, 1)
	assert.Equal(t, "kaputt", sc.Terms[0].Text)
	_, ok, err = tb.GetConcept(ctx, newID)
	require.NoError(t, err)
	require.True(t, ok)

	// The shadow relation links the shadow concepts.
	rels, err := tb.RelationsOf(ctx, oldID, nil)
	require.NoError(t, err)
	require.Len(t, rels, 1)
	assert.Equal(t, graph.LabelUseInstead, rels[0].RelationType)
	assert.Equal(t, oldID, rels[0].SourceID)
	assert.Equal(t, newID, rels[0].TargetID)

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
	_, err = e.StartPilot(ctx, ws, store, *loaded, "projA", "pilot/a")
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
	_, ok, err := tb.GetConcept(ctx, pilotConceptID(cs.ID, "pilot/a", "c1"))
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
	_, err = e.StartPilot(ctx, ws, store, *loaded, "proj1", pilotStream)
	require.NoError(t, err)

	// The shadow was written — the pilot did its job.
	_, ok, err := tb.GetConcept(ctx, pilotConceptID(cs.ID, pilotStream, "old"))
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
