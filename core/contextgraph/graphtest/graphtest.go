// Package graphtest is the shared query-shape suite for the context graph.
//
// AD-039's contract is that a local project and the server answer the SAME
// questions in the same shapes, with the dimensions pinned in one case and free
// in the other. A contract asserted only in prose drifts, so it is asserted
// here: one fixture, one table of queries and expected answers, run against
// every store that claims to hold a context graph — the project's SQLite store
// and the server's Postgres one.
//
// The fixture is deliberately a workspace with TWO projects sharing a concept,
// because that is the case a project-scoped store cannot represent and a
// workspace one must: the local arm runs it with the dimensions carrying real
// values anyway, which is what proves the two stores differ in scope rather
// than in kind.
package graphtest

import (
	"context"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/contextgraph"
	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Store is what a suite arm supplies: the read surface the queries use, plus
// the bulk writes the fixture is loaded through.
type Store interface {
	contextgraph.Reader
	BulkUpsertNodes(ctx context.Context, nodes []*graph.Node) error
	BulkUpsertEdges(ctx context.Context, edges []*graph.Edge) error
}

// The fixture's dimensions.
const (
	Workspace = "ws-acme"
	ProjectA  = "pr-docs"
	ProjectB  = "pr-app"
	Stream    = "main"

	// ConceptShared is used by both projects — the two-hop case.
	ConceptShared = "c-memory"
	// ConceptLocal is used by one.
	ConceptLocal = "c-ship"
	// ConceptWindowed is held in two spellings, one of them deprecated from a
	// date and only in the Nordics. It is the case where "is it discouraged
	// here" has a different answer at two coordinates.
	ConceptWindowed = "c-signin"

	// SharedContentKey is held by both projects: identical wording, two
	// instances, two nodes.
	SharedContentKey = "k1_shared"
	// WindowedContentKey is the block that reaches for both spellings.
	WindowedContentKey = "k1_b2"
)

// DeprecatedFrom is the instant the windowed concept's second spelling becomes
// the discouraged one. Before it, the same block is a block using an ordinary
// word; after it, the same block is a block breaking a rule.
var DeprecatedFrom = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

// WindowedMarket is the market the deprecation applies in. The same instant
// answers differently inside it and outside it.
const WindowedMarket = "nordics"

// ScopeA and ScopeB are the two projects' scopes; ScopeWorkspace pins only the
// workspace, which is the rollup view.
var (
	ScopeA         = contextgraph.Scope{Workspace: Workspace, Project: ProjectA, Stream: Stream}
	ScopeB         = contextgraph.Scope{Workspace: Workspace, Project: ProjectB, Stream: Stream}
	ScopeWorkspace = contextgraph.Scope{Workspace: Workspace}
)

// Fixture returns the nodes and edges the suite queries. It is written through
// the store's bulk upserts, so a store that cannot express the shape fails at
// load rather than in an assertion.
func Fixture() ([]*graph.Node, []*graph.Edge) {
	var nodes []*graph.Node
	var edges []*graph.Edge
	add := func(n graph.Node) { nodes = append(nodes, &n) }
	link := func(e graph.Edge) { edges = append(edges, &e) }

	// Workspace vocabulary: one node each, however many projects use it.
	add(contextgraph.ConceptNode(ScopeWorkspace, ConceptShared, "content memory"))
	add(contextgraph.ConceptNode(ScopeWorkspace, ConceptLocal, "ship state"))
	add(contextgraph.CoordinateNode(ScopeWorkspace, "bowrain", "web"))
	add(contextgraph.CoordinateNode(ScopeWorkspace, "bowrain", "docs"))

	// Project A: two blocks in one collection, both using the shared concept;
	// one of them also uses the project-local one.
	add(contextgraph.CollectionNode(ScopeA, "docs"))
	add(contextgraph.BlockNode(ScopeA, contextgraph.Block{
		ContentKey: SharedContentKey, BlockID: "intro", Document: "docs/guide.md", Collection: "docs",
	}))
	add(contextgraph.BlockNode(ScopeA, contextgraph.Block{
		ContentKey: "k1_a2", BlockID: "body", Document: "docs/guide.md", Collection: "docs",
	}))
	link(contextgraph.UsesTermEdge(ScopeA, contextgraph.UsesTerm{
		ContentKey: SharedContentKey, ConceptID: ConceptShared, Term: "content memory",
		Status: string(model.TermPreferred), Collection: "docs", Count: 2,
	}))
	link(contextgraph.UsesTermEdge(ScopeA, contextgraph.UsesTerm{
		ContentKey: "k1_a2", ConceptID: ConceptShared, Term: "content memory",
		Status: string(model.TermPreferred), Collection: "docs",
	}))
	link(contextgraph.UsesTermEdge(ScopeA, contextgraph.UsesTerm{
		ContentKey: "k1_a2", ConceptID: ConceptLocal, Term: "ship state",
		Status: string(model.TermApproved), Collection: "docs",
	}))
	link(contextgraph.InCollectionEdge(ScopeA, SharedContentKey, "docs"))
	link(contextgraph.InCollectionEdge(ScopeA, "k1_a2", "docs"))
	link(contextgraph.GovernedByEdge(ScopeA, "docs", "bowrain", "docs", nil))

	// Project B: one block, the same wording as project A's first — a second
	// node carrying the same content key, never a shared one.
	add(contextgraph.CollectionNode(ScopeB, "web"))
	add(contextgraph.BlockNode(ScopeB, contextgraph.Block{
		ContentKey: SharedContentKey, BlockID: "hero", Document: "src/home.tsx", Collection: "web",
	}))
	link(contextgraph.UsesTermEdge(ScopeB, contextgraph.UsesTerm{
		ContentKey: SharedContentKey, ConceptID: ConceptShared, Term: "content memory",
		Status: string(model.TermPreferred), Collection: "web",
	}))
	link(contextgraph.InCollectionEdge(ScopeB, SharedContentKey, "web"))
	link(contextgraph.GovernedByEdge(ScopeB, "web", "bowrain", "web", nil))

	// One block reaching for two spellings of one concept. The second is
	// deprecated from a date and only in one market, so the same block answers
	// "is this discouraged here" differently at different coordinates — and the
	// two spellings are two edges, because a term is part of what a use IS.
	add(contextgraph.ConceptNode(ScopeWorkspace, ConceptWindowed, "sign in"))
	add(contextgraph.BlockNode(ScopeB, contextgraph.Block{
		ContentKey: WindowedContentKey, BlockID: "cta", Document: "src/home.tsx", Collection: "web",
	}))
	link(contextgraph.UsesTermEdge(ScopeB, contextgraph.UsesTerm{
		ContentKey: WindowedContentKey, ConceptID: ConceptWindowed, Term: "sign in", TermLocale: "en",
		Status: string(model.TermPreferred), Collection: "web",
	}))
	link(contextgraph.UsesTermEdge(ScopeB, contextgraph.UsesTerm{
		ContentKey: WindowedContentKey, ConceptID: ConceptWindowed, Term: "log in", TermLocale: "en",
		Status: string(model.TermDeprecated), Collection: "web", Count: 2,
		Validity: &graph.Validity{
			ValidFrom: &DeprecatedFrom,
			Tags:      map[string]string{"market": WindowedMarket},
		},
	}))
	link(contextgraph.InCollectionEdge(ScopeB, WindowedContentKey, "web"))

	// A decision blessing project B's block.
	blessing := contextgraph.UnitState{
		Unit: "hero", Variant: "nb", Status: "reviewed", ReviewState: "approved", TargetHash: "th-b1",
	}
	add(contextgraph.UnitStateNode(ScopeB, blessing))
	link(contextgraph.BlessesEdge(ScopeB, blessing, SharedContentKey))

	return nodes, edges
}

// Load writes the fixture into the store.
func Load(t *testing.T, store Store) {
	t.Helper()
	ctx := context.Background()
	nodes, edges := Fixture()
	require.NoError(t, store.BulkUpsertNodes(ctx, nodes))
	require.NoError(t, store.BulkUpsertEdges(ctx, edges))
}

// RunQueryShapes loads the fixture and runs every shared query against the
// store. A store that passes answers the explorer's questions — and a query
// added for one surface is expressible on the other, which is the point.
func RunQueryShapes(t *testing.T, store Store) {
	t.Helper()
	Load(t, store)
	ctx := context.Background()

	t.Run("which projects use this concept", func(t *testing.T) {
		got, err := contextgraph.ProjectsUsingConcept(ctx, store, ScopeWorkspace, ConceptShared)
		require.NoError(t, err)
		assert.Equal(t, []contextgraph.Scope{
			{Workspace: Workspace, Project: ProjectB},
			{Workspace: Workspace, Project: ProjectA},
		}, sortedScopes(got), "both projects reach the one workspace concept node")

		only, err := contextgraph.ProjectsUsingConcept(ctx, store, ScopeWorkspace, ConceptLocal)
		require.NoError(t, err)
		assert.Equal(t, []contextgraph.Scope{{Workspace: Workspace, Project: ProjectA}}, only)
	})

	t.Run("pinning the project narrows the same query", func(t *testing.T) {
		got, err := contextgraph.ProjectsUsingConcept(ctx, store, ScopeA, ConceptShared)
		require.NoError(t, err)
		assert.Equal(t, []contextgraph.Scope{{Workspace: Workspace, Project: ProjectA}}, got,
			"the local surface asks the workspace query with its one dimension pinned")
	})

	t.Run("how much of the concept sits in each project", func(t *testing.T) {
		got, err := contextgraph.UsesByProject(ctx, store, ScopeWorkspace, ConceptShared, graph.Scope{})
		require.NoError(t, err)
		require.Len(t, got, 2, "both projects, each with its own numbers")

		byProject := map[string]contextgraph.ProjectUse{}
		for _, p := range got {
			byProject[p.Scope.Project] = p
		}

		a := byProject[ProjectA]
		assert.Equal(t, 2, a.Blocks, "two blocks in project A use the concept")
		assert.Equal(t, 3, a.Occurrences, "one block uses it twice, the other once")
		assert.Equal(t, []string{"docs"}, a.Collections)
		require.Len(t, a.Terms, 1)
		assert.Equal(t, "content memory", a.Terms[0].Term)
		assert.Equal(t, 3, a.Terms[0].Occurrences)
		assert.Equal(t, 2, a.Terms[0].Blocks, "a term used twice in one block is one block")
		require.Len(t, a.Uses, 2)
		assert.Equal(t, Stream, a.Uses[0].Scope.Stream, "a use names the stream it sits on")

		b := byProject[ProjectB]
		assert.Equal(t, 1, b.Blocks)
		assert.Equal(t, 1, b.Occurrences)
		assert.Equal(t, []string{"web"}, b.Collections)

		// The rollup drops the stream for the same reason ProjectsUsingConcept
		// does: a project that uses a concept on one stream uses it.
		assert.Empty(t, a.Scope.Stream)

		// Pinning the project narrows the rollup rather than needing a second
		// query, exactly as it narrows the projects list.
		pinned, err := contextgraph.UsesByProject(ctx, store, ScopeB, ConceptShared, graph.Scope{})
		require.NoError(t, err)
		require.Len(t, pinned, 1)
		assert.Equal(t, ProjectB, pinned[0].Scope.Project)
	})

	t.Run("how the concept stands in each project", func(t *testing.T) {
		got, err := contextgraph.UsesByProject(ctx, store, ScopeWorkspace, ConceptShared, graph.Scope{})
		require.NoError(t, err)
		require.NotEmpty(t, got)
		for _, p := range got {
			assert.Equal(t, string(model.TermPreferred), p.Status,
				"every project uses the preferred spelling, so that is how it stands there")
			assert.False(t, p.Discouraged)
		}
	})

	// The point of carrying the window: "is it discouraged" is not a property of
	// the concept, it is a property of the concept AT A COORDINATE. The same
	// block, the same graph, two coordinates, two answers.
	t.Run("is it discouraged here depends on here", func(t *testing.T) {
		before := graph.ScopeAt(DeprecatedFrom.Add(-24 * time.Hour))
		after := graph.ScopeAt(DeprecatedFrom.Add(24 * time.Hour))

		early, err := contextgraph.UsesByProject(ctx, store, ScopeWorkspace, ConceptWindowed, before)
		require.NoError(t, err)
		require.Len(t, early, 1)
		assert.False(t, early[0].Discouraged,
			"before the window opens the deprecated spelling is not in force")
		assert.Equal(t, string(model.TermPreferred), early[0].Status)
		require.Len(t, early[0].Terms, 1, "only the spelling in force is counted")
		assert.Equal(t, "sign in", early[0].Terms[0].Term)
		assert.Equal(t, 1, early[0].Occurrences)

		late, err := contextgraph.UsesByProject(ctx, store, ScopeWorkspace, ConceptWindowed, after)
		require.NoError(t, err)
		require.Len(t, late, 1)
		assert.True(t, late[0].Discouraged,
			"after it opens the same block is a block still saying the discouraged word")
		assert.Equal(t, string(model.TermDeprecated), late[0].Status)
		require.Len(t, late[0].Terms, 2, "a term is part of what a use is, so two spellings are two uses")
		assert.Equal(t, 3, late[0].Occurrences, "one use of the preferred spelling, two of the other")

		// The other coordinate is the market. The deprecation is scoped to one,
		// so a reader standing outside it is not breaking a rule.
		outside := graph.Scope{At: after.At, Tags: map[string]string{"market": "us"}}
		elsewhere, err := contextgraph.UsesByProject(ctx, store, ScopeWorkspace, ConceptWindowed, outside)
		require.NoError(t, err)
		require.Len(t, elsewhere, 1)
		assert.False(t, elsewhere[0].Discouraged,
			"the same word, the same instant, a market the deprecation does not reach")

		inside := graph.Scope{At: after.At, Tags: map[string]string{"market": WindowedMarket}}
		here, err := contextgraph.UsesByProject(ctx, store, ScopeWorkspace, ConceptWindowed, inside)
		require.NoError(t, err)
		require.Len(t, here, 1)
		assert.True(t, here[0].Discouraged)

		// The as-declared view applies no window at all, which is the answer to a
		// different question: what does the workspace hold, rather than what is
		// in force here.
		declared, err := contextgraph.UsesByProject(ctx, store, ScopeWorkspace, ConceptWindowed, graph.Scope{})
		require.NoError(t, err)
		require.Len(t, declared, 1)
		assert.True(t, declared[0].Discouraged)
		assert.Len(t, declared[0].Terms, 2)
	})

	t.Run("term to blocks to collection", func(t *testing.T) {
		wide, err := contextgraph.Uses(ctx, store, ScopeWorkspace, ConceptShared)
		require.NoError(t, err)
		assert.Len(t, wide, 3, "two blocks in project A, one in project B")

		narrow, err := contextgraph.Uses(ctx, store, ScopeB, ConceptShared)
		require.NoError(t, err)
		require.Len(t, narrow, 1)
		assert.Equal(t, "web", narrow[0].Collection)
		assert.Equal(t, ProjectB, narrow[0].Scope.Project)
	})

	t.Run("same wording across projects is a content-key query", func(t *testing.T) {
		got, err := contextgraph.BlocksWithContentKey(ctx, store, ScopeWorkspace, SharedContentKey)
		require.NoError(t, err)
		assert.Equal(t, []contextgraph.Scope{ScopeB, ScopeA}, sortedScopes(got),
			"identical wording is two instances, not one shared node")
	})

	t.Run("what is governed here", func(t *testing.T) {
		got, err := contextgraph.CollectionsAtCoordinate(ctx, store, ScopeWorkspace, "bowrain", "web", graph.Scope{})
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "web", got[0].Collection)
		assert.Equal(t, ProjectB, got[0].Scope.Project)

		docs, err := contextgraph.CollectionsAtCoordinate(ctx, store, ScopeWorkspace, "bowrain", "docs", graph.Scope{})
		require.NoError(t, err)
		require.Len(t, docs, 1)
		assert.Equal(t, ProjectA, docs[0].Scope.Project)
	})

	t.Run("which decision covers this unit", func(t *testing.T) {
		got, err := contextgraph.BlessingsOfBlock(ctx, store, ScopeB, SharedContentKey)
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, "hero", got[0].Unit)
		assert.Equal(t, "th-b1", got[0].TargetHash)

		// The identical wording in the other project is not blessed by it.
		none, err := contextgraph.BlessingsOfBlock(ctx, store, ScopeA, SharedContentKey)
		require.NoError(t, err)
		assert.Empty(t, none, "a decision holds one instance, not every copy of the words")
	})

	t.Run("no project relates to a project directly", func(t *testing.T) {
		edges, err := store.EdgesOf(ctx, contextgraph.BlockNodeID(ScopeA, SharedContentKey), graph.Both)
		require.NoError(t, err)
		for _, e := range edges {
			other := e.Target
			if e.Target == contextgraph.BlockNodeID(ScopeA, SharedContentKey) {
				other = e.Source
			}
			parsed, ok := contextgraph.ParseNodeID(other)
			require.True(t, ok)
			if parsed.Scope.Project != "" {
				assert.Equal(t, ProjectA, parsed.Scope.Project,
					"an instance node never links to another project's instance")
			}
		}
	})
}

// sortedScopes puts scopes in a stable order for comparison. Query results are
// already ordered by scope key; this makes the expected values readable.
func sortedScopes(in []contextgraph.Scope) []contextgraph.Scope {
	out := append([]contextgraph.Scope(nil), in...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Key() < out[j-1].Key(); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
