package server

import (
	"context"
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/knowledge"
	corebrand "github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/termbase"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// linkTestServer wires a server whose workspace termbase is a fresh in-memory
// store, so linkRuleToConcept runs with no PostgreSQL. ContentStore is left nil,
// so the source locale defaults to English (the resolution-from-project path is
// covered by TestFirstWorkspaceSourceLocale).
func linkTestServer(t *testing.T) *Server {
	t.Helper()
	srv := NewServer(DefaultConfig())
	srv.wsStores.tbFactory = func() termbase.TBStore {
		return &testTermStore{termbase.NewInMemoryTermBase()}
	}
	return srv
}

// conceptByTerm finds the brand-vocabulary concept holding a term with the given
// text and status.
func conceptByTerm(t *testing.T, tb termbase.TBStore, text string, status model.TermStatus) (termbase.Concept, bool) {
	t.Helper()
	concepts, err := tb.Concepts(context.Background())
	require.NoError(t, err)
	for _, c := range concepts {
		for _, term := range c.Terms {
			if term.Text == text && term.Status == status {
				return c, true
			}
		}
	}
	return termbase.Concept{}, false
}

func eventTypes(events []knowledge.MergeEvent) []knowledge.EventType {
	out := make([]knowledge.EventType, len(events))
	for i, e := range events {
		out[i] = e.Type
	}
	return out
}

func TestLinkRuleToConcept_CreatesForbiddenReplacementAndRelation(t *testing.T) {
	srv := linkTestServer(t)
	ctx := context.Background()
	const wsSlug, wsID = "acme", "ws-acme"

	rule := corebrand.SuggestedRule{Term: "utilize", Replacement: "use"}
	forbiddenID, events, err := srv.linkRuleToConcept(ctx, wsSlug, wsID, rule)
	require.NoError(t, err)
	require.NotEmpty(t, forbiddenID)

	tb, err := srv.wsStores.getTB(wsSlug)
	require.NoError(t, err)

	// Forbidden concept: brand-vocabulary source, English (default) locale.
	forbidden, ok := conceptByTerm(t, tb, "utilize", model.TermForbidden)
	require.True(t, ok, "forbidden concept should exist")
	assert.Equal(t, forbiddenID, forbidden.ID)
	assert.Equal(t, termbase.TermSourceBrandVocabulary, forbidden.Source)
	require.Len(t, forbidden.Terms, 1)
	assert.Equal(t, defaultBrandConceptLocale, forbidden.Terms[0].Locale)

	// Replacement concept: preferred term, brand-vocabulary source.
	replacement, ok := conceptByTerm(t, tb, "use", model.TermPreferred)
	require.True(t, ok, "replacement concept should exist")
	assert.Equal(t, termbase.TermSourceBrandVocabulary, replacement.Source)

	// USE_INSTEAD edge forbidden → replacement.
	rels, err := tb.RelationsOf(ctx, forbiddenID, nil)
	require.NoError(t, err)
	require.Len(t, rels, 1)
	assert.Equal(t, graph.LabelUseInstead, rels[0].RelationType)
	assert.Equal(t, forbiddenID, rels[0].SourceID)
	assert.Equal(t, replacement.ID, rels[0].TargetID)

	// Events: two creations + one relation, each scoped to the workspace.
	assert.Equal(t,
		[]knowledge.EventType{
			knowledge.EventConceptCreated,
			knowledge.EventConceptCreated,
			knowledge.EventConceptRelationAdded,
		}, eventTypes(events))
	for _, e := range events {
		assert.Equal(t, wsID, e.WorkspaceID)
	}
	assert.Equal(t, forbiddenID, events[0].ConceptID)
	assert.Equal(t, replacement.ID, events[1].ConceptID)
	assert.Equal(t, forbiddenID, events[2].ConceptID)
}

func TestLinkRuleToConcept_Idempotent(t *testing.T) {
	srv := linkTestServer(t)
	ctx := context.Background()
	const wsSlug, wsID = "acme", "ws-acme"
	rule := corebrand.SuggestedRule{Term: "utilize", Replacement: "use"}

	firstID, _, err := srv.linkRuleToConcept(ctx, wsSlug, wsID, rule)
	require.NoError(t, err)

	secondID, events, err := srv.linkRuleToConcept(ctx, wsSlug, wsID, rule)
	require.NoError(t, err)
	assert.Equal(t, firstID, secondID, "re-promotion reuses the forbidden concept")
	assert.Empty(t, events, "a re-promotion that changes nothing emits no events")

	tb, err := srv.wsStores.getTB(wsSlug)
	require.NoError(t, err)
	count, err := tb.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, count, "no duplicate concepts")
	rels, err := tb.RelationsOf(ctx, firstID, nil)
	require.NoError(t, err)
	assert.Len(t, rels, 1, "no duplicate USE_INSTEAD edge")
}

func TestLinkRuleToConcept_NoReplacement(t *testing.T) {
	srv := linkTestServer(t)
	ctx := context.Background()
	const wsSlug, wsID = "acme", "ws-acme"

	rule := corebrand.SuggestedRule{Term: "synergy"}
	forbiddenID, events, err := srv.linkRuleToConcept(ctx, wsSlug, wsID, rule)
	require.NoError(t, err)
	require.NotEmpty(t, forbiddenID)

	tb, err := srv.wsStores.getTB(wsSlug)
	require.NoError(t, err)
	count, err := tb.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "a ban with no replacement creates only the forbidden concept")
	rels, err := tb.RelationsOf(ctx, forbiddenID, nil)
	require.NoError(t, err)
	assert.Empty(t, rels, "no replacement means no relation")
	assert.Equal(t, []knowledge.EventType{knowledge.EventConceptCreated}, eventTypes(events))
}

func TestLinkRuleToConcept_ReusesExistingBrandConceptCaseInsensitive(t *testing.T) {
	srv := linkTestServer(t)
	ctx := context.Background()
	const wsSlug, wsID = "acme", "ws-acme"

	tb, err := srv.wsStores.getTB(wsSlug)
	require.NoError(t, err)
	// Seed a pre-existing brand-vocab forbidden concept with a different casing.
	seeded := termbase.Concept{
		ID:     "seed-forbidden",
		Source: termbase.TermSourceBrandVocabulary,
		Terms:  []termbase.Term{{Text: "Utilize", Locale: "en", Status: model.TermForbidden}},
	}
	require.NoError(t, tb.AddConcept(ctx, seeded))

	rule := corebrand.SuggestedRule{Term: "utilize", Replacement: "use"}
	forbiddenID, events, err := srv.linkRuleToConcept(ctx, wsSlug, wsID, rule)
	require.NoError(t, err)
	assert.Equal(t, "seed-forbidden", forbiddenID, "case-insensitive reuse of the seeded concept")

	// Only the replacement concept and the relation are new.
	assert.Equal(t,
		[]knowledge.EventType{knowledge.EventConceptCreated, knowledge.EventConceptRelationAdded},
		eventTypes(events))
	count, err := tb.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 2, count, "the seeded forbidden concept is reused, not duplicated")
}

func TestLinkRuleToConcept_EmptyTermIsNoop(t *testing.T) {
	srv := linkTestServer(t)
	id, events, err := srv.linkRuleToConcept(context.Background(), "acme", "ws-acme",
		corebrand.SuggestedRule{Term: "   "})
	require.NoError(t, err)
	assert.Empty(t, id)
	assert.Empty(t, events)
}

func TestLinkRuleToConcept_UsesProjectSourceLocale(t *testing.T) {
	srv := linkTestServer(t)
	srv.ContentStore = &fakeProjectContentStore{projects: []*platstore.Project{
		{ID: "p1", WorkspaceID: "ws-acme", DefaultSourceLanguage: "de"},
	}}
	ctx := context.Background()

	forbiddenID, _, err := srv.linkRuleToConcept(ctx, "acme", "ws-acme", corebrand.SuggestedRule{Term: "nutzen"})
	require.NoError(t, err)

	tb, err := srv.wsStores.getTB("acme")
	require.NoError(t, err)
	c, ok, err := tb.GetConcept(ctx, forbiddenID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Len(t, c.Terms, 1)
	assert.Equal(t, model.LocaleID("de"), c.Terms[0].Locale, "the concept term takes the project's source locale")
}

func TestFirstWorkspaceSourceLocale(t *testing.T) {
	ctx := context.Background()
	ps := &fakeProjectContentStore{projects: []*platstore.Project{
		{ID: "other", WorkspaceID: "ws-other", DefaultSourceLanguage: "fr"},
		{ID: "blank", WorkspaceID: "ws-acme", DefaultSourceLanguage: ""},
		{ID: "match", WorkspaceID: "ws-acme", DefaultSourceLanguage: "de"},
	}}

	assert.Equal(t, model.LocaleID("de"), firstWorkspaceSourceLocale(ctx, ps, "ws-acme"),
		"first matching project with a declared source language wins")
	assert.Equal(t, model.LocaleID(""), firstWorkspaceSourceLocale(ctx, ps, "ws-none"),
		"no matching project yields the empty locale (caller falls back)")
	// wsID == "" matches any project (single-tenant / test setups).
	assert.Equal(t, model.LocaleID("fr"), firstWorkspaceSourceLocale(ctx, ps, ""))
}

// fakeProjectContentStore is a minimal store.ContentStore that serves a fixed
// project list; every other method is an unused no-op. It lets the locale
// resolver run without a real PostgreSQL content store.
type fakeProjectContentStore struct {
	platstore.ContentStore
	projects []*platstore.Project
}

func (f *fakeProjectContentStore) ListProjects(context.Context) ([]*platstore.Project, error) {
	return f.projects, nil
}
