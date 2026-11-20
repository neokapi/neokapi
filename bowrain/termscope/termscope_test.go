package termscope_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sqlterms "github.com/neokapi/neokapi/bowrain/terms"
	"github.com/neokapi/neokapi/bowrain/termscope"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
)

func TestLimitAndWiden(t *testing.T) {
	cp := terms.Concept{ID: "c", Properties: map[string]string{"keep": "me"}}
	assert.True(t, termscope.Workspace(cp))
	assert.True(t, termscope.AppliesIn(cp, "a"))

	shared := cp.Properties
	termscope.Limit(&cp, "b", "a", "a", " ")
	assert.Equal(t, []string{"a", "b"}, termscope.Projects(cp))
	assert.Equal(t, "a,b", cp.Properties[termscope.PropProjects])
	assert.Empty(t, cp.ProjectID, "the column names one project or none")
	assert.Equal(t, "me", cp.Properties["keep"])
	assert.NotContains(t, shared, termscope.PropProjects, "the caller's map is left as it was")
	assert.True(t, termscope.AppliesIn(cp, "a"))
	assert.False(t, termscope.AppliesIn(cp, "c"))
	assert.True(t, termscope.AppliesIn(cp, ""), "no project to narrow to")

	termscope.Limit(&cp, "a")
	assert.Equal(t, "a", cp.ProjectID)
	assert.True(t, termscope.Widen(&cp))
	assert.True(t, termscope.Workspace(cp))
	assert.Empty(t, cp.ProjectID)
	assert.Equal(t, "me", cp.Properties["keep"])
	assert.False(t, termscope.Widen(&cp))

	// The SQLite store's project column limits a concept too.
	col := terms.Concept{ProjectID: "a"}
	assert.Equal(t, []string{"a"}, termscope.Projects(col))
	assert.True(t, termscope.Widen(&col))
	assert.Empty(t, col.ProjectID)
}

var backends = []struct {
	name string
	open func(t *testing.T) terms.Store
}{
	{"sqlite", func(t *testing.T) terms.Store {
		tb, err := terms.NewSQLiteStore(filepath.Join(t.TempDir(), "terms.db"))
		require.NoError(t, err)
		t.Cleanup(func() { _ = tb.Close() })
		return tb
	}},
	{"postgres", func(t *testing.T) terms.Store {
		tb, err := sqlterms.NewPostgresStoreFromDB(pgtest.NewTestDB(t), "ws-termscope-"+id.New())
		require.NoError(t, err)
		return tb
	}},
}

// TestStoreView: a project's view of the store holds the concepts of the
// whole workspace and its own, and every read agrees on that.
func TestStoreView(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := t.Context()
			tb := backend.open(t)
			add := func(cid, text string, projects ...string) {
				cp := terms.Concept{ID: cid, Terms: []terms.Term{{Text: text, Locale: "en", Status: model.TermPreferred}}}
				termscope.Limit(&cp, projects...)
				require.NoError(t, tb.AddConcept(ctx, cp))
			}
			add("c-all", "dashboard")
			add("c-a", "flowgraph", "proj-a")
			add("c-b", "workbench", "proj-b")
			add("c-ab", "pipeline", "proj-a", "proj-b")
			require.NoError(t, tb.AddRelation(ctx, terms.ConceptRelation{ID: "r-ab", SourceID: "c-a", TargetID: "c-b", RelationType: graph.LabelRelated}))
			require.NoError(t, tb.AddRelation(ctx, terms.ConceptRelation{ID: "r-a", SourceID: "c-a", TargetID: "c-all", RelationType: graph.LabelRelated}))

			stored, ok, err := tb.GetConcept(ctx, "c-ab")
			require.NoError(t, err)
			require.True(t, ok)
			assert.Equal(t, []string{"proj-a", "proj-b"}, termscope.Projects(stored), "the limit survives the store")

			v := termscope.StoreView(tb, "proj-a")
			ids := func(cs []terms.Concept) []string {
				out := []string{}
				for _, c := range cs {
					out = append(out, c.ID)
				}
				return out
			}
			all, err := v.Concepts(ctx)
			require.NoError(t, err)
			assert.ElementsMatch(t, []string{"c-all", "c-a", "c-ab"}, ids(all))
			n, err := v.Count(ctx)
			require.NoError(t, err)
			assert.Equal(t, 3, n)

			_, ok, err = v.GetConcept(ctx, "c-b")
			require.NoError(t, err)
			assert.False(t, ok, "another project's concept is not found")

			matches, err := v.LookupAll(ctx, "The workbench shows the flowgraph", terms.LookupOptions{SourceLocale: "en"})
			require.NoError(t, err)
			require.Len(t, matches, 1)
			assert.Equal(t, "c-a", matches[0].Concept.ID)

			page, total, err := v.Search(ctx, "", "", "", 1, 1)
			require.NoError(t, err)
			assert.Equal(t, 3, total, "the total counts what the project sees")
			assert.Len(t, page, 1)
			page, _, err = v.SearchForStream(ctx, "", "", "", "main", []string{""}, 0, 10)
			require.NoError(t, err)
			assert.ElementsMatch(t, []string{"c-all", "c-a", "c-ab"}, ids(page))

			rels, err := v.ListRelations(ctx, nil)
			require.NoError(t, err)
			require.Len(t, rels, 1, "a relation to a hidden concept is hidden")
			assert.Equal(t, "r-a", rels[0].ID)
			rels, err = v.RelationsOf(ctx, "c-a", nil)
			require.NoError(t, err)
			assert.Len(t, rels, 1)

			same := termscope.StoreView(tb, "")
			n, err = same.Count(ctx)
			require.NoError(t, err)
			assert.Equal(t, 4, n, "no project reads the whole store")
		})
	}
}

func TestProjectEligibilityPrecedesTermRanking(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := t.Context()
			tb := backend.open(t)
			for _, c := range []struct{ id, term, project string }{
				{"visible", "dashboard", "a"},
				{"plural", "dashboards", "b"},
				{"phrase", "dashboard settings", "b"},
			} {
				cp := terms.Concept{ID: c.id, Terms: []terms.Term{{Text: c.term, Locale: "en", Status: model.TermPreferred}}}
				termscope.Limit(&cp, c.project)
				require.NoError(t, tb.AddConcept(ctx, cp))
			}
			v := termscope.StoreView(tb, "a")
			opts := terms.LookupOptions{SourceLocale: "en", MinScore: .8}
			hits, err := v.Lookup(ctx, "dashboards", opts)
			require.NoError(t, err)
			require.Len(t, hits, 1)
			assert.Equal(t, "visible", hits[0].Concept.ID)
			assert.Equal(t, model.MatchStrategyFuzzy, hits[0].MatchType)
			hits, err = v.LookupAll(ctx, "dashboard settings", opts)
			require.NoError(t, err)
			require.Len(t, hits, 1)
			assert.Equal(t, "visible", hits[0].Concept.ID)
			opts.ConceptIDs = map[string]bool{"phrase": true}
			hits, err = v.LookupAll(ctx, "dashboard settings", opts)
			require.NoError(t, err)
			assert.Empty(t, hits)
			assert.True(t, opts.ConceptIDs["phrase"])
		})
	}
}

func TestProjectLookupPreservesTermGovernance(t *testing.T) {
	for _, backend := range backends {
		t.Run(backend.name, func(t *testing.T) {
			ctx := t.Context()
			tb := backend.open(t)
			for _, c := range []terms.Concept{
				{ID: "eligible", Domain: "Santé", Source: terms.TermSourceBrandVocabulary, Terms: []terms.Term{{Text: "Acme", Locale: "en", Status: model.TermForbidden, CompetitorTerm: true}}},
				{ID: "ordinary", Domain: "Santé", Terms: []terms.Term{{Text: "Acme", Locale: "en", Status: model.TermAdmitted}}},
				{ID: "other-domain", Domain: "Other", Source: terms.TermSourceBrandVocabulary, Terms: []terms.Term{{Text: "Acme", Locale: "en", Status: model.TermPreferred}}},
			} {
				require.NoError(t, tb.AddConcept(ctx, c))
			}
			project := termscope.StoreView(tb, "a")
			opts := terms.LookupOptions{
				SourceLocale: "en", Domains: []string{"SANTÉ"},
				SourceFilter: []terms.TermSource{terms.TermSourceBrandVocabulary},
			}
			for _, lookup := range []func(context.Context, string, terms.LookupOptions) ([]terms.TermMatch, error){project.Lookup, project.LookupAll} {
				hits, err := lookup(ctx, "Acme", opts)
				require.NoError(t, err)
				require.Len(t, hits, 1)
				assert.Equal(t, "eligible", hits[0].Concept.ID)
				assert.True(t, hits[0].Term.CompetitorTerm)
			}
		})
	}
}
