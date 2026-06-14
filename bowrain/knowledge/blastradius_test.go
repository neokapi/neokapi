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

// ---------------------------------------------------------------------------
// Test doubles
// ---------------------------------------------------------------------------

// fakeBlockSource is an in-memory BlockSource (and CollectionResolver) for the
// engine tests. Blocks are keyed by project+stream; items map to collections so
// the per-collection grouping can be exercised.
type fakeBlockSource struct {
	projects []*store.Project
	streams  map[string][]*store.Stream      // projectID → streams (excludes implicit "main")
	blocks   map[string][]*store.StoredBlock // projectID|stream → blocks
	items    map[string]*store.Item          // projectID|stream|itemName → item
	cols     map[string]*store.Collection    // projectID|collectionID → collection
}

func newFakeBlockSource() *fakeBlockSource {
	return &fakeBlockSource{
		streams: map[string][]*store.Stream{},
		blocks:  map[string][]*store.StoredBlock{},
		items:   map[string]*store.Item{},
		cols:    map[string]*store.Collection{},
	}
}

func (f *fakeBlockSource) addProject(p *store.Project) { f.projects = append(f.projects, p) }

func (f *fakeBlockSource) addBlocks(projectID, stream string, blocks ...*store.StoredBlock) {
	key := projectID + "|" + stream
	for _, b := range blocks {
		b.ProjectID = projectID
	}
	f.blocks[key] = append(f.blocks[key], blocks...)
}

func (f *fakeBlockSource) addStream(projectID, name string) {
	f.streams[projectID] = append(f.streams[projectID], &store.Stream{ProjectID: projectID, Name: name})
}

func (f *fakeBlockSource) addItem(projectID, stream, itemName, collectionID string) {
	f.items[projectID+"|"+stream+"|"+itemName] = &store.Item{
		ProjectID: projectID, Name: itemName, CollectionID: collectionID,
	}
}

func (f *fakeBlockSource) addCollection(projectID, id, name string) {
	f.cols[projectID+"|"+id] = &store.Collection{ProjectID: projectID, ID: id, Name: name}
}

func (f *fakeBlockSource) ListProjects(context.Context) ([]*store.Project, error) {
	return f.projects, nil
}

func (f *fakeBlockSource) ListStreams(_ context.Context, projectID string, _ bool) ([]*store.Stream, error) {
	return f.streams[projectID], nil
}

func (f *fakeBlockSource) GetBlocks(_ context.Context, q store.BlockQuery) ([]*store.StoredBlock, error) {
	stream := q.Stream
	if stream == "" {
		stream = "main"
	}
	return f.blocks[q.ProjectID+"|"+stream], nil
}

func (f *fakeBlockSource) GetItem(_ context.Context, projectID, stream, itemName string) (*store.Item, error) {
	return f.items[projectID+"|"+stream+"|"+itemName], nil
}

func (f *fakeBlockSource) GetCollection(_ context.Context, projectID, collectionID string) (*store.Collection, error) {
	return f.cols[projectID+"|"+collectionID], nil
}

// fakeProfileStore is an in-memory ProfileStore.
type fakeProfileStore struct {
	profiles map[string]*corebrand.VoiceProfile
}

func newFakeProfileStore(ps ...*corebrand.VoiceProfile) *fakeProfileStore {
	m := map[string]*corebrand.VoiceProfile{}
	for _, p := range ps {
		m[p.ID] = p
	}
	return &fakeProfileStore{profiles: m}
}

func (f *fakeProfileStore) GetProfile(_ context.Context, id string) (*corebrand.VoiceProfile, error) {
	return f.profiles[id], nil
}

func (f *fakeProfileStore) ListProfiles(_ context.Context, workspaceID string) ([]*corebrand.VoiceProfile, error) {
	var out []*corebrand.VoiceProfile
	for _, p := range f.profiles {
		if workspaceID == "" || p.WorkspaceID == workspaceID {
			out = append(out, p)
		}
	}
	return out, nil
}

func (f *fakeProfileStore) UpdateProfile(_ context.Context, p *corebrand.VoiceProfile) error {
	f.profiles[p.ID] = p
	return nil
}

// srcBlock builds a translatable StoredBlock with a single source TextRun.
func srcBlock(id, itemName string, locale model.LocaleID, text string) *store.StoredBlock {
	b := &model.Block{ID: id, Translatable: true, SourceLocale: locale}
	b.SetSourceText(text)
	return &store.StoredBlock{Block: b, ItemName: itemName}
}

// ---------------------------------------------------------------------------
// EvaluateChangeSet
// ---------------------------------------------------------------------------

func TestEvaluateChangeSet_VoiceRuleAddFlagsMatchingBlocks(t *testing.T) {
	ctx := context.Background()

	bs := newFakeBlockSource()
	bs.addProject(&store.Project{ID: "proj1", Name: "Site", WorkspaceID: "ws"})
	bs.addBlocks("proj1", "main",
		srcBlock("b1", "home.json", "en-US", "Embrace synergy across teams"), // contains forbidden term
		srcBlock("b2", "home.json", "en-US", "Welcome to our site"),          // clean
	)

	profile := &corebrand.VoiceProfile{ID: "p1", Name: "Acme", WorkspaceID: "ws"}
	ps := newFakeProfileStore(profile)
	e := NewEngine(bs, termbase.NewInMemoryTermBase(), ps, nil)

	ops := []ChangeSetOp{
		mustOp(t, 0, OpVoiceRuleAdd, VoiceRuleAddPayload{ProfileID: "p1", List: VoiceListForbidden, Rule: corebrand.TermRule{Term: "synergy", Replacement: "teamwork"}}),
	}

	imp, err := e.EvaluateChangeSet(ctx, "ws", ChangeSet{}, ops, EvalOptions{})
	require.NoError(t, err)

	assert.Equal(t, 2, imp.TotalBlocks)
	assert.Equal(t, 1, imp.AffectedBlocks)
	assert.Equal(t, 1, imp.NewViolations)
	assert.Equal(t, 0, imp.Resolved)
	assert.Equal(t, 4, imp.Words, "word count of the affected block's source text")

	require.Len(t, imp.Projects, 1)
	pi := imp.Projects[0]
	assert.Equal(t, "proj1", pi.ProjectID)
	assert.Equal(t, 1, pi.AffectedBlocks)
	require.Len(t, pi.Collections, 1)
	require.Len(t, pi.Collections[0].Locales, 1)
	assert.Equal(t, model.LocaleID("en-US"), pi.Collections[0].Locales[0].Locale)

	require.Len(t, imp.Samples, 1)
	assert.Equal(t, "b1", imp.Samples[0].BlockID)
	assert.Equal(t, 1, imp.Samples[0].NewViolations)
}

func TestEvaluateChangeSet_VoiceRuleRemoveResolves(t *testing.T) {
	ctx := context.Background()

	bs := newFakeBlockSource()
	bs.addProject(&store.Project{ID: "proj1", Name: "Site", WorkspaceID: "ws"})
	bs.addBlocks("proj1", "main",
		srcBlock("b1", "home.json", "en-US", "Embrace synergy across teams"),
	)

	profile := &corebrand.VoiceProfile{
		ID: "p1", Name: "Acme", WorkspaceID: "ws",
		Vocabulary: corebrand.VocabularyRules{ForbiddenTerms: []corebrand.TermRule{{Term: "synergy"}}},
	}
	e := NewEngine(bs, termbase.NewInMemoryTermBase(), newFakeProfileStore(profile), nil)

	ops := []ChangeSetOp{
		mustOp(t, 0, OpVoiceRuleRemove, VoiceRuleRemovePayload{ProfileID: "p1", List: VoiceListForbidden, Term: "synergy"}),
	}
	imp, err := e.EvaluateChangeSet(ctx, "ws", ChangeSet{}, ops, EvalOptions{})
	require.NoError(t, err)

	assert.Equal(t, 1, imp.AffectedBlocks)
	assert.Equal(t, 0, imp.NewViolations)
	assert.Equal(t, 1, imp.Resolved)
}

func TestEvaluateChangeSet_TermStatusForbiddenFlagsBlocks(t *testing.T) {
	ctx := context.Background()

	tb := termbase.NewInMemoryTermBase()
	require.NoError(t, tb.AddConcept(ctx, concept("c1", term("foobar", "en-US", model.TermAdmitted))))

	bs := newFakeBlockSource()
	bs.addProject(&store.Project{ID: "proj1", Name: "Docs", WorkspaceID: "ws"})
	bs.addBlocks("proj1", "main",
		srcBlock("b1", "guide.md", "en-US", "Please use foobar here"),
		srcBlock("b2", "guide.md", "en-US", "Nothing to see"),
	)

	e := NewEngine(bs, tb, newFakeProfileStore(), nil)
	ops := []ChangeSetOp{
		mustOp(t, 0, OpTermStatus, TermStatusPayload{ConceptID: "c1", Locale: "en-US", Text: "foobar", From: model.TermAdmitted, To: model.TermForbidden}),
	}

	imp, err := e.EvaluateChangeSet(ctx, "ws", ChangeSet{}, ops, EvalOptions{})
	require.NoError(t, err)

	assert.Equal(t, 2, imp.TotalBlocks)
	assert.Equal(t, 1, imp.AffectedBlocks)
	assert.Equal(t, 1, imp.NewViolations)
	assert.Equal(t, 0, imp.Resolved)
	require.Len(t, imp.Samples, 1)
	assert.Equal(t, "b1", imp.Samples[0].BlockID)
}

func TestEvaluateChangeSet_RelationGuidanceChangeAffectsWithoutViolation(t *testing.T) {
	ctx := context.Background()

	tb := termbase.NewInMemoryTermBase()
	require.NoError(t, tb.AddConcept(ctx, concept("old", term("kaputt", "en-US", model.TermDeprecated))))
	require.NoError(t, tb.AddConcept(ctx, concept("new", term("fixed", "en-US", model.TermPreferred))))

	bs := newFakeBlockSource()
	bs.addProject(&store.Project{ID: "proj1", Name: "Docs", WorkspaceID: "ws"})
	bs.addBlocks("proj1", "main", srcBlock("b1", "g.md", "en-US", "the kaputt thing"))

	e := NewEngine(bs, tb, newFakeProfileStore(), nil)
	ops := []ChangeSetOp{
		mustOp(t, 0, OpRelationAdd, RelationAddPayload{Relation: termbase.ConceptRelation{
			ID: "r1", SourceID: "old", TargetID: "new", RelationType: graph.LabelUseInstead,
		}}),
	}

	imp, err := e.EvaluateChangeSet(ctx, "ws", ChangeSet{}, ops, EvalOptions{})
	require.NoError(t, err)

	assert.Equal(t, 1, imp.AffectedBlocks, "USE_INSTEAD guidance change affects the block")
	assert.Equal(t, 0, imp.NewViolations)
	assert.Equal(t, 0, imp.Resolved)
}

func TestEvaluateChangeSet_GroupingAndWordSums(t *testing.T) {
	ctx := context.Background()

	bs := newFakeBlockSource()
	// Project 1: en-US, two collections.
	bs.addProject(&store.Project{ID: "p1", Name: "Marketing", WorkspaceID: "ws"})
	bs.addCollection("p1", "col-a", "Landing")
	bs.addCollection("p1", "col-b", "Blog")
	bs.addItem("p1", "main", "a.json", "col-a")
	bs.addItem("p1", "main", "b.json", "col-b")
	bs.addBlocks("p1", "main",
		srcBlock("a1", "a.json", "en-US", "pure synergy here now"), // 4 words, affected
		srcBlock("b1", "b.json", "en-US", "more synergy today"),    // 3 words, affected
		srcBlock("b2", "b.json", "en-US", "clean copy"),            // clean
	)
	// Project 2: de-DE, one collection.
	bs.addProject(&store.Project{ID: "p2", Name: "Site DE", WorkspaceID: "ws"})
	bs.addCollection("p2", "col-c", "Home DE")
	bs.addItem("p2", "main", "c.json", "col-c")
	bs.addBlocks("p2", "main",
		srcBlock("c1", "c.json", "de-DE", "wir lieben synergy hier"), // 4 words, affected
	)
	// A project in another workspace must be ignored.
	bs.addProject(&store.Project{ID: "other", Name: "Other", WorkspaceID: "ws2"})
	bs.addBlocks("other", "main", srcBlock("o1", "o.json", "en-US", "synergy synergy"))

	profile := &corebrand.VoiceProfile{ID: "p1prof", Name: "Acme", WorkspaceID: "ws"}
	e := NewEngine(bs, termbase.NewInMemoryTermBase(), newFakeProfileStore(profile), nil)
	ops := []ChangeSetOp{
		mustOp(t, 0, OpVoiceRuleAdd, VoiceRuleAddPayload{ProfileID: "p1prof", List: VoiceListForbidden, Rule: corebrand.TermRule{Term: "synergy"}}),
	}

	imp, err := e.EvaluateChangeSet(ctx, "ws", ChangeSet{}, ops, EvalOptions{})
	require.NoError(t, err)

	assert.Equal(t, 4, imp.TotalBlocks, "the ws2 project is excluded")
	assert.Equal(t, 3, imp.AffectedBlocks)
	assert.Equal(t, 3, imp.NewViolations)
	assert.Equal(t, 4+3+4, imp.Words)

	require.Len(t, imp.Projects, 2)

	p1 := findProject(imp.Projects, "p1")
	require.NotNil(t, p1)
	assert.Equal(t, 2, p1.AffectedBlocks)
	require.Len(t, p1.Collections, 2)
	colA := findCollection(p1.Collections, "col-a")
	colB := findCollection(p1.Collections, "col-b")
	require.NotNil(t, colA)
	require.NotNil(t, colB)
	assert.Equal(t, "Landing", colA.CollectionName)
	assert.Equal(t, 4, colA.Words)
	assert.Equal(t, 3, colB.Words)
	require.Len(t, colA.Locales, 1)
	assert.Equal(t, model.LocaleID("en-US"), colA.Locales[0].Locale)

	p2 := findProject(imp.Projects, "p2")
	require.NotNil(t, p2)
	require.Len(t, p2.Collections, 1)
	require.Len(t, p2.Collections[0].Locales, 1)
	assert.Equal(t, model.LocaleID("de-DE"), p2.Collections[0].Locales[0].Locale)
}

func TestEvaluateChangeSet_EmptyChangeSetZeroImpact(t *testing.T) {
	ctx := context.Background()

	bs := newFakeBlockSource()
	bs.addProject(&store.Project{ID: "proj1", Name: "Site", WorkspaceID: "ws"})
	bs.addBlocks("proj1", "main", srcBlock("b1", "home.json", "en-US", "Embrace synergy"))

	e := NewEngine(bs, termbase.NewInMemoryTermBase(), newFakeProfileStore(), nil)

	imp, err := e.EvaluateChangeSet(ctx, "ws", ChangeSet{}, nil, EvalOptions{})
	require.NoError(t, err)

	assert.Equal(t, 1, imp.TotalBlocks)
	assert.Equal(t, 0, imp.AffectedBlocks)
	assert.Equal(t, 0, imp.NewViolations)
	assert.Equal(t, 0, imp.Resolved)
	assert.Equal(t, 0, imp.Words)
	assert.NotNil(t, imp.Projects)
	assert.Empty(t, imp.Projects)
	assert.NotNil(t, imp.Samples)
	assert.Empty(t, imp.Samples)
}

func TestEvaluateChangeSet_SampleCap(t *testing.T) {
	ctx := context.Background()

	bs := newFakeBlockSource()
	bs.addProject(&store.Project{ID: "proj1", Name: "Site", WorkspaceID: "ws"})
	var blocks []*store.StoredBlock
	for i := range 10 {
		blocks = append(blocks, srcBlock("b"+string(rune('0'+i)), "home.json", "en-US", "synergy everywhere"))
	}
	bs.addBlocks("proj1", "main", blocks...)

	profile := &corebrand.VoiceProfile{ID: "p1", Name: "Acme", WorkspaceID: "ws"}
	e := NewEngine(bs, termbase.NewInMemoryTermBase(), newFakeProfileStore(profile), nil)
	ops := []ChangeSetOp{
		mustOp(t, 0, OpVoiceRuleAdd, VoiceRuleAddPayload{ProfileID: "p1", List: VoiceListForbidden, Rule: corebrand.TermRule{Term: "synergy"}}),
	}

	imp, err := e.EvaluateChangeSet(ctx, "ws", ChangeSet{}, ops, EvalOptions{MaxSamples: 3})
	require.NoError(t, err)

	assert.Equal(t, 10, imp.AffectedBlocks)
	assert.Len(t, imp.Samples, 3, "samples are capped by MaxSamples")
}

func TestEvaluateChangeSet_PilotStream(t *testing.T) {
	ctx := context.Background()

	bs := newFakeBlockSource()
	bs.addProject(&store.Project{ID: "proj1", Name: "Site", WorkspaceID: "ws"})
	bs.addStream("proj1", "pilot/rebrand")
	bs.addBlocks("proj1", "main", srcBlock("m1", "home.json", "en-US", "main synergy copy"))
	bs.addBlocks("proj1", "pilot/rebrand", srcBlock("p1b", "home.json", "en-US", "pilot synergy copy"))

	profile := &corebrand.VoiceProfile{ID: "p1", Name: "Acme", WorkspaceID: "ws"}
	e := NewEngine(bs, termbase.NewInMemoryTermBase(), newFakeProfileStore(profile), nil)
	ops := []ChangeSetOp{
		mustOp(t, 0, OpVoiceRuleAdd, VoiceRuleAddPayload{ProfileID: "p1", List: VoiceListForbidden, Rule: corebrand.TermRule{Term: "synergy"}}),
	}

	// Without pilot streams, only "main" is walked.
	imp, err := e.EvaluateChangeSet(ctx, "ws", ChangeSet{}, ops, EvalOptions{})
	require.NoError(t, err)
	assert.Equal(t, 1, imp.TotalBlocks)

	// With the pilot stream included, both are walked.
	imp, err = e.EvaluateChangeSet(ctx, "ws", ChangeSet{}, ops, EvalOptions{
		PilotStreams: map[string][]string{"proj1": {"pilot/rebrand"}},
	})
	require.NoError(t, err)
	assert.Equal(t, 2, imp.TotalBlocks)
	assert.Equal(t, 2, imp.AffectedBlocks)

	streams := map[string]bool{}
	for _, s := range imp.Samples {
		streams[s.Stream] = true
	}
	assert.True(t, streams["main"])
	assert.True(t, streams["pilot/rebrand"])
}

// ---------------------------------------------------------------------------
// ConceptUsage
// ---------------------------------------------------------------------------

func TestConceptUsage(t *testing.T) {
	ctx := context.Background()

	tb := termbase.NewInMemoryTermBase()
	require.NoError(t, tb.AddConcept(ctx, concept("c1",
		term("widget", "en-US", model.TermPreferred),
		term("Widget", "de-DE", model.TermPreferred),
	)))

	bs := newFakeBlockSource()
	bs.addProject(&store.Project{ID: "p1", Name: "Docs", WorkspaceID: "ws"})
	bs.addCollection("p1", "col-a", "Guide")
	bs.addItem("p1", "main", "a.md", "col-a")
	bs.addBlocks("p1", "main",
		srcBlock("a1", "a.md", "en-US", "the widget and another widget"), // 2 occurrences
		srcBlock("a2", "a.md", "en-US", "nothing here"),                  // none
	)
	bs.addProject(&store.Project{ID: "p2", Name: "Docs DE", WorkspaceID: "ws"})
	bs.addBlocks("p2", "main", srcBlock("b1", "b.md", "de-DE", "ein Widget hier")) // 1 occurrence

	e := NewEngine(bs, tb, newFakeProfileStore(), nil)
	usage, err := e.ConceptUsage(ctx, "ws", "c1", EvalOptions{})
	require.NoError(t, err)

	assert.Equal(t, "c1", usage.ConceptID)
	assert.Equal(t, 3, usage.TotalBlocks)
	assert.Equal(t, 2, usage.Blocks, "two (block, locale) rows contain the concept")
	assert.Equal(t, 3, usage.Occurrences)
	require.Len(t, usage.Projects, 2)

	p1 := findProjectUsage(usage.Projects, "p1")
	require.NotNil(t, p1)
	assert.Equal(t, 2, p1.Occurrences)
	require.Len(t, p1.Collections, 1)
	assert.Equal(t, "Guide", p1.Collections[0].CollectionName)

	require.Len(t, usage.Samples, 2)
}

func TestConceptUsage_MissingConcept(t *testing.T) {
	ctx := context.Background()

	bs := newFakeBlockSource()
	bs.addProject(&store.Project{ID: "p1", Name: "Docs", WorkspaceID: "ws"})
	bs.addBlocks("p1", "main", srcBlock("a1", "a.md", "en-US", "the widget here"))

	e := NewEngine(bs, termbase.NewInMemoryTermBase(), newFakeProfileStore(), nil)
	usage, err := e.ConceptUsage(ctx, "ws", "ghost", EvalOptions{})
	require.NoError(t, err)

	assert.Equal(t, 1, usage.TotalBlocks)
	assert.Equal(t, 0, usage.Blocks)
	assert.Empty(t, usage.Projects)
	assert.NotNil(t, usage.Samples)
	assert.Empty(t, usage.Samples)
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func findProject(ps []ProjectImpact, id string) *ProjectImpact {
	for i := range ps {
		if ps[i].ProjectID == id {
			return &ps[i]
		}
	}
	return nil
}

func findCollection(cs []CollectionImpact, id string) *CollectionImpact {
	for i := range cs {
		if cs[i].CollectionID == id {
			return &cs[i]
		}
	}
	return nil
}

func findProjectUsage(ps []ProjectUsage, id string) *ProjectUsage {
	for i := range ps {
		if ps[i].ProjectID == id {
			return &ps[i]
		}
	}
	return nil
}
