package termbase_test

import (
	"context"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/termbase"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// relationBackends returns the framework TermBase implementations so every
// relation and validity test runs against memory and SQLite alike.
func relationBackends(t *testing.T) map[string]termbase.TermBase {
	t.Helper()
	sqliteTB, err := termbase.NewSQLiteTermBase(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqliteTB.Close() })
	return map[string]termbase.TermBase{
		"memory": termbase.NewInMemoryTermBase(),
		"sqlite": sqliteTB,
	}
}

// seedRelationConcepts adds three plain concepts to relate.
func seedRelationConcepts(t *testing.T, tb termbase.TermBase) {
	t.Helper()
	for _, id := range []string{"c1", "c2", "c3"} {
		require.NoError(t, tb.AddConcept(context.Background(), termbase.Concept{
			ID:     id,
			Domain: "software",
			Terms: []termbase.Term{
				{Text: "term-" + id, Locale: model.LocaleEnglish, Status: model.TermApproved},
			},
		}))
	}
}

// mustRelationsOf returns the relations of a concept, failing the test on error.
func mustRelationsOf(t *testing.T, tb termbase.TermBase, conceptID string, scope *graph.Scope) []termbase.ConceptRelation {
	t.Helper()
	rels, err := tb.RelationsOf(context.Background(), conceptID, scope)
	require.NoError(t, err)
	return rels
}

// mustListRelations returns all relations, failing the test on error.
func mustListRelations(t *testing.T, tb termbase.TermBase, scope *graph.Scope) []termbase.ConceptRelation {
	t.Helper()
	rels, err := tb.ListRelations(context.Background(), scope)
	require.NoError(t, err)
	return rels
}

func relationIDs(rels []termbase.ConceptRelation) []string {
	ids := make([]string, len(rels))
	for i, r := range rels {
		ids[i] = r.ID
	}
	return ids
}

func TestTermBase_RelationsCRUD(t *testing.T) {
	for name, tb := range relationBackends(t) {
		t.Run(name, func(t *testing.T) {
			seedRelationConcepts(t, tb)
			ctx := context.Background()

			created := time.Now().UTC().Truncate(time.Second)
			require.NoError(t, tb.AddRelation(ctx, termbase.ConceptRelation{
				ID: "r1", SourceID: "c1", TargetID: "c2",
				RelationType: graph.LabelBroader, CreatedAt: created,
			}))
			require.NoError(t, tb.AddRelation(ctx, termbase.ConceptRelation{
				ID: "r2", SourceID: "c2", TargetID: "c3",
				RelationType: graph.LabelReplacedBy, Note: "renamed at launch",
			}))

			rels := mustListRelations(t, tb, nil)
			assert.Equal(t, []string{"r1", "r2"}, relationIDs(rels))
			assert.Equal(t, "renamed at launch", rels[1].Note)
			assert.False(t, rels[0].CreatedAt.IsZero())
			assert.False(t, rels[1].CreatedAt.IsZero())

			// Both directions: c2 is the target of r1 and the source of r2.
			assert.Equal(t, []string{"r1", "r2"}, relationIDs(mustRelationsOf(t, tb, "c2", nil)))
			assert.Equal(t, []string{"r1"}, relationIDs(mustRelationsOf(t, tb, "c1", nil)))
			assert.Equal(t, []string{"r2"}, relationIDs(mustRelationsOf(t, tb, "c3", nil)))
			assert.Empty(t, mustRelationsOf(t, tb, "missing", nil))

			// Upsert by ID: the relation is updated in place and the original
			// creation time is preserved.
			require.NoError(t, tb.AddRelation(ctx, termbase.ConceptRelation{
				ID: "r1", SourceID: "c1", TargetID: "c3",
				RelationType: graph.LabelRelated, Note: "retyped",
			}))
			rels = mustListRelations(t, tb, nil)
			require.Len(t, rels, 2)
			assert.Equal(t, "c3", rels[0].TargetID)
			assert.Equal(t, graph.LabelRelated, rels[0].RelationType)
			assert.Equal(t, "retyped", rels[0].Note)
			assert.True(t, rels[0].CreatedAt.Equal(created), "upsert must preserve CreatedAt")

			// Delete.
			require.NoError(t, tb.DeleteRelation(ctx, "r2"))
			assert.Equal(t, []string{"r1"}, relationIDs(mustListRelations(t, tb, nil)))
			assert.Error(t, tb.DeleteRelation(ctx, "r2"))
		})
	}
}

func TestTermBase_AddRelationValidation(t *testing.T) {
	for name, tb := range relationBackends(t) {
		t.Run(name, func(t *testing.T) {
			seedRelationConcepts(t, tb)
			ctx := context.Background()

			tests := []struct {
				name string
				rel  termbase.ConceptRelation
			}{
				{"missing ID", termbase.ConceptRelation{SourceID: "c1", TargetID: "c2", RelationType: graph.LabelRelated}},
				{"missing source", termbase.ConceptRelation{ID: "r1", TargetID: "c2", RelationType: graph.LabelRelated}},
				{"missing target", termbase.ConceptRelation{ID: "r1", SourceID: "c1", RelationType: graph.LabelRelated}},
				{"unknown relation type", termbase.ConceptRelation{ID: "r1", SourceID: "c1", TargetID: "c2", RelationType: "FRIENDS_WITH"}},
				{"concept-to-term label rejected", termbase.ConceptRelation{ID: "r1", SourceID: "c1", TargetID: "c2", RelationType: graph.LabelHasTerm}},
				{"source concept not found", termbase.ConceptRelation{ID: "r1", SourceID: "ghost", TargetID: "c2", RelationType: graph.LabelRelated}},
				{"target concept not found", termbase.ConceptRelation{ID: "r1", SourceID: "c1", TargetID: "ghost", RelationType: graph.LabelRelated}},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					assert.Error(t, tb.AddRelation(ctx, tt.rel))
				})
			}
			assert.Empty(t, mustListRelations(t, tb, nil))
		})
	}
}

func TestTermBase_RelationValidityScoping(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)

	for name, tb := range relationBackends(t) {
		t.Run(name, func(t *testing.T) {
			seedRelationConcepts(t, tb)
			ctx := context.Background()

			require.NoError(t, tb.AddRelation(ctx, termbase.ConceptRelation{
				ID: "r-always", SourceID: "c1", TargetID: "c2", RelationType: graph.LabelRelated,
			}))
			require.NoError(t, tb.AddRelation(ctx, termbase.ConceptRelation{
				ID: "r-dach", SourceID: "c1", TargetID: "c2", RelationType: graph.LabelUseInstead,
				Validity: &graph.Validity{Tags: map[string]string{"market": "dach"}},
			}))
			require.NoError(t, tb.AddRelation(ctx, termbase.ConceptRelation{
				ID: "r-future", SourceID: "c1", TargetID: "c2", RelationType: graph.LabelReplacedBy,
				Validity: &graph.Validity{ValidFrom: &future},
			}))
			require.NoError(t, tb.AddRelation(ctx, termbase.ConceptRelation{
				ID: "r-retired", SourceID: "c1", TargetID: "c2", RelationType: graph.LabelCloseMatch,
				Validity: &graph.Validity{ValidTo: &past},
			}))

			// nil scope = no filtering.
			assert.Len(t, mustListRelations(t, tb, nil), 4)

			// "Now" excludes the not-yet-active and the retired edge.
			nowScope := graph.ScopeAt(now)
			assert.Equal(t, []string{"r-always", "r-dach"},
				relationIDs(mustListRelations(t, tb, &nowScope)))

			// As-of a past date, the retired edge is active again.
			asOf := graph.ScopeAt(past.Add(-time.Minute))
			assert.Equal(t, []string{"r-always", "r-dach", "r-retired"},
				relationIDs(mustListRelations(t, tb, &asOf)))

			// Market scoping: the dach edge holds in dach, not in us.
			dach := graph.Scope{At: now, Tags: map[string]string{"market": "dach"}}
			assert.Equal(t, []string{"r-always", "r-dach"},
				relationIDs(mustRelationsOf(t, tb, "c1", &dach)))
			us := graph.Scope{At: now, Tags: map[string]string{"market": "us"}}
			assert.Equal(t, []string{"r-always"},
				relationIDs(mustRelationsOf(t, tb, "c1", &us)))

			// The validity itself round-trips through the backend.
			rels := mustListRelations(t, tb, nil)
			byID := make(map[string]termbase.ConceptRelation, len(rels))
			for _, r := range rels {
				byID[r.ID] = r
			}
			require.NotNil(t, byID["r-dach"].Validity)
			assert.Equal(t, map[string]string{"market": "dach"}, byID["r-dach"].Validity.Tags)
			require.NotNil(t, byID["r-future"].Validity)
			require.NotNil(t, byID["r-future"].Validity.ValidFrom)
			assert.True(t, byID["r-future"].Validity.ValidFrom.Equal(future))
			assert.Nil(t, byID["r-always"].Validity)
		})
	}
}

func TestTermBase_DeleteConceptCascadesRelations(t *testing.T) {
	for name, tb := range relationBackends(t) {
		t.Run(name, func(t *testing.T) {
			seedRelationConcepts(t, tb)
			ctx := context.Background()

			require.NoError(t, tb.AddRelation(ctx, termbase.ConceptRelation{
				ID: "r1", SourceID: "c1", TargetID: "c2", RelationType: graph.LabelRelated,
			}))
			require.NoError(t, tb.AddRelation(ctx, termbase.ConceptRelation{
				ID: "r2", SourceID: "c2", TargetID: "c3", RelationType: graph.LabelBroader,
			}))
			require.NoError(t, tb.AddRelation(ctx, termbase.ConceptRelation{
				ID: "r3", SourceID: "c3", TargetID: "c1", RelationType: graph.LabelPartOf,
			}))

			require.NoError(t, tb.DeleteConcept(ctx, "c2"))

			// Every relation touching c2 is gone; the c3↔c1 edge survives.
			assert.Equal(t, []string{"r3"}, relationIDs(mustListRelations(t, tb, nil)))
		})
	}
}

func TestTermBase_TermValidityLookup(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	past := now.Add(-time.Hour)

	for name, tb := range relationBackends(t) {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, tb.AddConcept(context.Background(), termbase.Concept{
				ID:     "greeting",
				Domain: "software",
				Terms: []termbase.Term{
					{Text: "Hello", Locale: model.LocaleEnglish, Status: model.TermPreferred,
						Validity: &graph.Validity{Tags: map[string]string{"market": "us"}}},
					{Text: "Howdy", Locale: model.LocaleEnglish, Status: model.TermApproved,
						Validity: &graph.Validity{ValidTo: &past}},
					{Text: "Hi", Locale: model.LocaleEnglish, Status: model.TermApproved},
				},
			}))

			// Term validity round-trips through the backend.
			c, ok := mustGetConcept(t, tb, "greeting")
			require.True(t, ok)
			require.Len(t, c.Terms, 3)
			validities := make(map[string]*graph.Validity, len(c.Terms))
			for _, term := range c.Terms {
				validities[term.Text] = term.Validity
			}
			require.NotNil(t, validities["Hello"])
			assert.Equal(t, map[string]string{"market": "us"}, validities["Hello"].Tags)
			require.NotNil(t, validities["Howdy"])
			require.NotNil(t, validities["Howdy"].ValidTo)
			assert.True(t, validities["Howdy"].ValidTo.Equal(past))
			assert.Nil(t, validities["Hi"])

			// nil scope = no filtering: the retired term still matches.
			matches := mustLookup(t, tb, "Howdy", termbase.LookupOptions{SourceLocale: model.LocaleEnglish})
			assert.Len(t, matches, 1)

			// A "now" scope skips the retired term.
			nowScope := graph.ScopeAt(now)
			matches = mustLookup(t, tb, "Howdy", termbase.LookupOptions{
				SourceLocale: model.LocaleEnglish, Scope: &nowScope,
			})
			assert.Empty(t, matches)

			// Market scoping on Lookup.
			us := graph.Scope{At: now, Tags: map[string]string{"market": "us"}}
			matches = mustLookup(t, tb, "Hello", termbase.LookupOptions{
				SourceLocale: model.LocaleEnglish, Scope: &us,
			})
			assert.Len(t, matches, 1)
			dach := graph.Scope{At: now, Tags: map[string]string{"market": "dach"}}
			matches = mustLookup(t, tb, "Hello", termbase.LookupOptions{
				SourceLocale: model.LocaleEnglish, Scope: &dach,
			})
			assert.Empty(t, matches)

			// LookupAll applies the same scope: in dach, only the untagged,
			// unexpired "Hi" survives.
			matches = mustLookupAll(t, tb, "Hello Howdy Hi", termbase.LookupOptions{
				SourceLocale: model.LocaleEnglish, Scope: &dach,
			})
			require.Len(t, matches, 1)
			assert.Equal(t, "Hi", matches[0].Term.Text)

			// Without a scope, all three are found.
			matches = mustLookupAll(t, tb, "Hello Howdy Hi", termbase.LookupOptions{
				SourceLocale: model.LocaleEnglish,
			})
			assert.Len(t, matches, 3)
		})
	}
}

func TestTermBase_AddConceptRejectsUnknownTermStatus(t *testing.T) {
	for name, tb := range relationBackends(t) {
		t.Run(name, func(t *testing.T) {
			err := tb.AddConcept(context.Background(), termbase.Concept{
				ID: "c-bad",
				Terms: []termbase.Term{
					{Text: "Save", Locale: model.LocaleEnglish, Status: "bogus"},
				},
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unknown status")

			// An empty status is allowed (unset).
			require.NoError(t, tb.AddConcept(context.Background(), termbase.Concept{
				ID: "c-ok",
				Terms: []termbase.Term{
					{Text: "Save", Locale: model.LocaleEnglish},
				},
			}))
		})
	}
}

func TestSQLiteTermBase_RelationsForStream(t *testing.T) {
	tb, err := termbase.NewSQLiteTermBase(":memory:")
	require.NoError(t, err)
	defer tb.Close()

	ctx := context.Background()
	require.NoError(t, tb.AddConceptWithStream(ctx, termbase.Concept{
		ID: "c1", Domain: "software",
		Terms: []termbase.Term{{Text: "File", Locale: "en-US", Status: model.TermApproved}},
	}, "main"))
	require.NoError(t, tb.AddConceptWithStream(ctx, termbase.Concept{
		ID: "c2", Domain: "software",
		Terms: []termbase.Term{{Text: "Document", Locale: "en-US", Status: model.TermApproved}},
	}, "main"))

	// One relation per stream level, mirroring the concept stream test:
	// workspace ("") → main → feature/rebrand.
	require.NoError(t, tb.AddRelation(ctx, termbase.ConceptRelation{
		ID: "r-ws", SourceID: "c1", TargetID: "c2", RelationType: graph.LabelExactMatch,
	}))
	require.NoError(t, tb.AddRelationWithStream(ctx, termbase.ConceptRelation{
		ID: "r-main", SourceID: "c1", TargetID: "c2", RelationType: graph.LabelRelated,
	}, "main"))
	require.NoError(t, tb.AddRelationWithStream(ctx, termbase.ConceptRelation{
		ID: "r-feat", SourceID: "c1", TargetID: "c2", RelationType: graph.LabelReplacedBy,
		Validity: &graph.Validity{Tags: map[string]string{"market": "dach"}},
	}, "feature/rebrand"))

	// The full chain sees all three, earliest stream first.
	rels, err := tb.RelationsForStream(ctx, "c1", "feature/rebrand", []string{"main", ""}, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"r-feat", "r-main", "r-ws"}, relationIDs(rels))

	// The workspace stream alone sees only the unstreamed relation.
	rels, err = tb.RelationsForStream(ctx, "c1", "", nil, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"r-ws"}, relationIDs(rels))

	// Main plus workspace does not see the feature shadow.
	rels, err = tb.RelationsForStream(ctx, "c1", "main", []string{""}, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"r-main", "r-ws"}, relationIDs(rels))

	// Validity scoping applies on top of stream inheritance.
	us := graph.ScopeWithTags(map[string]string{"market": "us"})
	rels, err = tb.RelationsForStream(ctx, "c1", "feature/rebrand", []string{"main", ""}, &us)
	require.NoError(t, err)
	assert.Equal(t, []string{"r-main", "r-ws"}, relationIDs(rels))

	// The plain (stream-agnostic) reads still see everything.
	assert.Len(t, mustListRelations(t, tb, nil), 3)
	assert.Len(t, mustRelationsOf(t, tb, "c1", nil), 3)
}
