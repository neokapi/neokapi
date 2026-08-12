package graph

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	coresync "github.com/neokapi/neokapi/bowrain/core/sync"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	bterms "github.com/neokapi/neokapi/bowrain/terms"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/contextgraph"
	"github.com/neokapi/neokapi/core/model"
	fwterms "github.com/neokapi/neokapi/terms"
)

const (
	writerWorkspace = "ws-writer"
	writerStream    = "main"
)

type writerFixture struct {
	content platstore.ContentStore
	terms   fwterms.Terminology
	graph   *SQLGraphStore
}

// newWriterFixture wires the real stores the server writer reads: the Postgres
// content store, the workspace's Postgres terminology, and the graph.
func newWriterFixture(t *testing.T) *writerFixture {
	t.Helper()
	db := pgtest.NewTestDB(t)
	cs, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	tb, err := bterms.NewPostgresStoreFromDB(db, writerWorkspace)
	require.NoError(t, err)
	pool := db.Pool()
	require.NotNil(t, pool)
	g := NewSQLGraphStore(pool)
	require.NoError(t, g.EnsureGraph(t.Context()))
	return &writerFixture{content: cs, terms: tb, graph: g}
}

// seedProject creates a project with one collection at a coordinate and one
// block carrying the given text.
func (f *writerFixture) seedProject(t *testing.T, ctx context.Context, name, collection, channel, text string) *platstore.Project {
	t.Helper()
	proj := &platstore.Project{
		Name:                  name,
		DefaultSourceLanguage: "en",
		WorkspaceID:           writerWorkspace,
		DefaultStream:         writerStream,
		Properties:            map[string]string{},
	}
	require.NoError(t, f.content.CreateProject(ctx, proj))
	require.NoError(t, f.content.CreateCollection(ctx, &platstore.Collection{
		ID:        proj.ID + "-" + collection,
		ProjectID: proj.ID,
		Name:      collection,
		Kind:      platstore.CollectionUploaded,
		Stream:    writerStream,
		Context:   map[string]string{"product": "acme", "channel": channel},
		Owner:     coresync.ContextOwnerRecipe,
	})) //nolint:exhaustruct // the writer reads name, context and stream
	item := collection + ".json"
	require.NoError(t, f.content.StoreItem(ctx, proj.ID, writerStream, &platstore.Item{
		ProjectID: proj.ID, Name: item, Format: "json", ItemType: "file",
		CollectionID: proj.ID + "-" + collection,
	}))
	b := &model.Block{ID: "intro", Translatable: true}
	b.SetSourceText(text)
	require.NoError(t, f.content.StoreBlocksForItem(ctx, proj.ID, writerStream, item, []*model.Block{b}))
	return proj
}

func (f *writerFixture) materialize(t *testing.T, ctx context.Context, proj *platstore.Project) int {
	t.Helper()
	n, err := MaterializeProjectContextGraph(ctx, f.graph, ProjectSubgraph{
		Content:     f.content,
		Terms:       f.terms,
		WorkspaceID: proj.WorkspaceID,
		ProjectID:   proj.ID,
		ProjectName: proj.Name,
		Stream:      writerStream,
	})
	require.NoError(t, err)
	return n
}

// The wave's acceptance: two projects in one workspace using one concept, and
// the workspace graph answering which projects those are.
//
// It is two hops through the workspace-scoped concept node. Nothing joins the
// projects to each other — there is no edge between them, and the answer comes
// out of the vocabulary they share.
func TestWhichProjectsUseThisConcept(t *testing.T) {
	f := newWriterFixture(t)
	ctx := t.Context()

	require.NoError(t, f.terms.AddConcept(ctx, fwterms.Concept{
		ID: "c-memory", Domain: "product",
		Terms: []fwterms.Term{{Text: "content memory", Locale: "en", Status: model.TermApproved}},
	}))

	docs := f.seedProject(t, ctx, "docs-site", "docs", "docs",
		"The content memory recycles approved wording.")
	app := f.seedProject(t, ctx, "app", "web", "web",
		"Content memory is what the app reuses.")
	other := f.seedProject(t, ctx, "unrelated", "misc", "misc",
		"Nothing relevant here at all.")

	for _, p := range []*platstore.Project{docs, app, other} {
		assert.GreaterOrEqual(t, f.materialize(t, ctx, p), 1, "every project writes at least its governance edge")
	}

	workspace := contextgraph.Scope{Workspace: writerWorkspace}
	projects, err := contextgraph.ProjectsUsingConcept(ctx, f.graph, workspace, "c-memory")
	require.NoError(t, err)
	require.Len(t, projects, 2, "only the two projects whose content uses the concept")
	using := map[string]bool{}
	for _, p := range projects {
		using[p.Project] = true
		assert.Equal(t, writerWorkspace, p.Workspace)
	}
	assert.True(t, using[docs.ID])
	assert.True(t, using[app.ID])
	assert.False(t, using[other.ID])

	// Pinning the project narrows the same query rather than needing another.
	pinned, err := contextgraph.ProjectsUsingConcept(ctx,
		f.graph, contextgraph.Scope{Workspace: writerWorkspace, Project: docs.ID}, "c-memory")
	require.NoError(t, err)
	assert.Equal(t, []contextgraph.Scope{{Workspace: writerWorkspace, Project: docs.ID}}, pinned)

	// The two-hop walk reaches the collections the blocks sit in, so a use is
	// placed here exactly as it is placed locally.
	uses, err := contextgraph.Uses(ctx, f.graph, workspace, "c-memory")
	require.NoError(t, err)
	require.Len(t, uses, 2)
	placed := map[string]string{}
	for _, u := range uses {
		placed[u.Scope.Project] = u.Collection
	}
	assert.Equal(t, "docs", placed[docs.ID])
	assert.Equal(t, "web", placed[app.ID])

	// And a block node names the document it was read from.
	blocks, err := f.graph.FindNodes(ctx, contextgraph.NodeBlock,
		map[string]string{contextgraph.PropProject: docs.ID})
	require.NoError(t, err)
	require.Len(t, blocks, 1)
	assert.Equal(t, "docs.json", blocks[0].Properties[contextgraph.PropDocument])
	assert.Equal(t, "intro", blocks[0].Properties[contextgraph.PropBlockID],
		"a block node names its structural key, the same one a decision addresses it by")
}

// A rebuild replaces its own project stream's rows and leaves the others
// standing, which is what makes the writer safe to run on every push.
func TestServerRebuildIsScopedAndIdempotent(t *testing.T) {
	f := newWriterFixture(t)
	ctx := t.Context()
	require.NoError(t, f.terms.AddConcept(ctx, fwterms.Concept{
		ID:    "c-memory",
		Terms: []fwterms.Term{{Text: "content memory", Locale: "en", Status: model.TermApproved}},
	}))
	docs := f.seedProject(t, ctx, "docs-site", "docs", "docs", "The content memory recycles wording.")
	app := f.seedProject(t, ctx, "app", "web", "web", "Content memory again.")

	first := f.materialize(t, ctx, docs)
	f.materialize(t, ctx, app)
	second := f.materialize(t, ctx, docs)
	assert.Equal(t, first, second, "a rebuild writes the same edges, not duplicates")

	blocks, err := f.graph.FindNodes(ctx, contextgraph.NodeBlock,
		map[string]string{contextgraph.PropProject: app.ID})
	require.NoError(t, err)
	assert.Len(t, blocks, 1, "rebuilding one project leaves the other's nodes alone")

	projects, err := contextgraph.ProjectsUsingConcept(ctx,
		f.graph, contextgraph.Scope{Workspace: writerWorkspace}, "c-memory")
	require.NoError(t, err)
	assert.Len(t, projects, 2)
}

// The project's display name is a property, so a rename is an update here and
// never a re-key: every id, and therefore every edge, survives untouched.
func TestServerRenameIsAPropertyUpdate(t *testing.T) {
	f := newWriterFixture(t)
	ctx := t.Context()
	require.NoError(t, f.terms.AddConcept(ctx, fwterms.Concept{
		ID:    "c-memory",
		Terms: []fwterms.Term{{Text: "content memory", Locale: "en", Status: model.TermApproved}},
	}))
	proj := f.seedProject(t, ctx, "docs-site", "docs", "docs", "The content memory recycles wording.")
	f.materialize(t, ctx, proj)

	before, err := f.graph.FindEdges(ctx, contextgraph.EdgeUsesTerm, nil)
	require.NoError(t, err)
	require.NotEmpty(t, before)

	proj.Name = "the-documentation-site"
	require.NoError(t, f.content.UpdateProject(ctx, proj))
	f.materialize(t, ctx, proj)

	after, err := f.graph.FindEdges(ctx, contextgraph.EdgeUsesTerm, nil)
	require.NoError(t, err)
	require.Len(t, after, len(before))
	assert.Equal(t, before[0].ID, after[0].ID, "a rename does not re-key the server's graph")

	blocks, err := f.graph.FindNodes(ctx, contextgraph.NodeBlock,
		map[string]string{contextgraph.PropProject: proj.ID})
	require.NoError(t, err)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "the-documentation-site", blocks[0].Properties[contextgraph.PropProjectName])
}

// A decision blesses the block it decided, joined by the (item, unit) pair the
// server's ledger records — the same pair a local record names its block with.
func TestServerWritesBlessings(t *testing.T) {
	f := newWriterFixture(t)
	ctx := t.Context()
	proj := f.seedProject(t, ctx, "docs-site", "docs", "docs", "Some wording nobody has termed.")

	decisions, ok := f.content.(platstore.DecisionStore)
	require.True(t, ok)
	_, err := decisions.UpsertUnitDecisions(ctx, proj.ID, writerStream, []platstore.UnitDecision{{
		ItemName: "docs.json", Unit: "intro", Variant: "nb",
		Status: "reviewed", ReviewState: "approved", TargetHash: "th-1",
	}})
	require.NoError(t, err)

	f.materialize(t, ctx, proj)

	scope := contextgraph.Scope{Workspace: writerWorkspace, Project: proj.ID, Stream: writerStream}
	blocks, err := f.graph.FindNodes(ctx, contextgraph.NodeBlock,
		map[string]string{contextgraph.PropProject: proj.ID})
	require.NoError(t, err)
	require.Len(t, blocks, 1, "a decision's block becomes a node even with no term use")

	blessings, err := contextgraph.BlessingsOfBlock(ctx, f.graph, scope,
		blocks[0].Properties[contextgraph.PropContentKey])
	require.NoError(t, err)
	require.Len(t, blessings, 1)
	assert.Equal(t, "intro", blessings[0].Unit)
	assert.Equal(t, "th-1", blessings[0].TargetHash)
}

// An unscoped write is refused rather than allowed to clear a label across
// every tenant.
func TestServerWriterRequiresAScope(t *testing.T) {
	f := newWriterFixture(t)
	_, err := MaterializeProjectContextGraph(t.Context(), f.graph, ProjectSubgraph{
		Content: f.content, ProjectID: "p1", Stream: writerStream,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no workspace")
}
