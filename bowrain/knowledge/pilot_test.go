package knowledge

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corebrand "github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/termbase"

	"github.com/neokapi/neokapi/bowrain/core/store"
)

// pilotContentStore is a minimal BlockSource that also implements
// StreamBindingStore, so the pilot lifecycle can bind a candidate voice profile
// to a content stream. Blocks are never walked by the pilot path, so the
// BlockSource methods are trivial.
type pilotContentStore struct {
	streams map[string]*store.Stream // key: projectID|name
}

func newPilotContentStore() *pilotContentStore {
	return &pilotContentStore{streams: map[string]*store.Stream{}}
}

func (c *pilotContentStore) seedStream(projectID, name string) {
	c.streams[projectID+"|"+name] = &store.Stream{ProjectID: projectID, Name: name}
}

func (c *pilotContentStore) ListProjects(context.Context) ([]*store.Project, error) { return nil, nil }

func (c *pilotContentStore) ListStreams(context.Context, string, bool) ([]*store.Stream, error) {
	return nil, nil
}

func (c *pilotContentStore) GetBlocks(context.Context, store.BlockQuery) ([]*store.StoredBlock, error) {
	return nil, nil
}

func (c *pilotContentStore) GetStream(_ context.Context, projectID, name string) (*store.Stream, error) {
	return c.streams[projectID+"|"+name], nil
}

func (c *pilotContentStore) UpdateStream(_ context.Context, s *store.Stream) error {
	c.streams[s.ProjectID+"|"+s.Name] = s
	return nil
}

// CreateProfile and DeleteProfile extend fakeProfileStore into a
// PilotProfileStore so pilots can materialize and retire candidate profiles.
func (f *fakeProfileStore) CreateProfile(_ context.Context, p *corebrand.VoiceProfile) error {
	f.profiles[p.ID] = p
	return nil
}

func (f *fakeProfileStore) DeleteProfile(_ context.Context, id string) error {
	delete(f.profiles, id)
	return nil
}

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
		Relation: termbase.ConceptRelation{ID: "r1", SourceID: "old", TargetID: "new", RelationType: graph.LabelUseInstead},
	})

	loaded, err := store.GetChangeSet(ctx, ws, cs.ID)
	require.NoError(t, err)

	e := NewEngine(nil, tb, newFakeProfileStore(), store)
	require.NoError(t, e.StartPilot(ctx, ws, store, *loaded, "proj1", pilotStream))

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

func TestStartStopPilot_VoiceBinding(t *testing.T) {
	ctx := context.Background()
	ws := "ws"
	pilotStream := "pilot/voice"

	profile := &corebrand.VoiceProfile{ID: "p1", Name: "Acme", WorkspaceID: ws, Version: 2}
	profiles := newFakeProfileStore(profile)

	content := newPilotContentStore()
	content.seedStream("proj1", pilotStream)

	store := newMemStore()
	cs := &ChangeSet{ID: "cs1", WorkspaceID: ws, Name: "Try forbidding synergy", CreatedBy: "alice"}
	require.NoError(t, store.CreateChangeSet(ctx, cs))
	appendOp(t, store, ws, cs.ID, 0, OpVoiceRuleAdd, VoiceRuleAddPayload{
		ProfileID: "p1", List: VoiceListForbidden,
		Rule: corebrand.TermRule{Term: "synergy", Replacement: "teamwork"},
	})

	loaded, err := store.GetChangeSet(ctx, ws, cs.ID)
	require.NoError(t, err)

	e := NewEngine(content, termbase.NewInMemoryTermBase(), profiles, store)
	require.NoError(t, e.StartPilot(ctx, ws, store, *loaded, "proj1", pilotStream))

	// A candidate profile was materialized with the rule applied.
	candID := pilotProfileID(cs.ID, pilotStream, "p1")
	cand, err := profiles.GetProfile(ctx, candID)
	require.NoError(t, err)
	require.NotNil(t, cand)
	require.Len(t, cand.Vocabulary.ForbiddenTerms, 1)
	assert.Equal(t, "synergy", cand.Vocabulary.ForbiddenTerms[0].Term)

	// The content stream is bound to the candidate profile.
	s, err := content.GetStream(ctx, "proj1", pilotStream)
	require.NoError(t, err)
	require.NotNil(t, s)
	assert.Equal(t, candID, s.Properties[corebrand.PropertyProfileID])

	// The baseline profile is untouched.
	base, err := profiles.GetProfile(ctx, "p1")
	require.NoError(t, err)
	assert.Empty(t, base.Vocabulary.ForbiddenTerms)
	assert.Equal(t, 2, base.Version)

	pilots, err := store.ListPilots(ctx, ws, cs.ID)
	require.NoError(t, err)
	require.Len(t, pilots, 1)

	// Stop the pilot — the binding is cleared and the candidate profile deleted.
	require.NoError(t, e.StopPilot(ctx, ws, store, *loaded, "proj1", pilotStream))
	s, err = content.GetStream(ctx, "proj1", pilotStream)
	require.NoError(t, err)
	_, bound := s.Properties[corebrand.PropertyProfileID]
	assert.False(t, bound, "the candidate voice binding is cleared")
	gone, err := profiles.GetProfile(ctx, candID)
	require.NoError(t, err)
	assert.Nil(t, gone, "the candidate profile is deleted")
	pilots, err = store.ListPilots(ctx, ws, cs.ID)
	require.NoError(t, err)
	assert.Empty(t, pilots)

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

	e := NewEngine(nil, tb, newFakeProfileStore(), store)
	require.NoError(t, e.StartPilot(ctx, ws, store, *loaded, "projA", "pilot/a"))
	require.NoError(t, e.StartPilot(ctx, ws, store, *loaded, "projB", "pilot/b"))

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
