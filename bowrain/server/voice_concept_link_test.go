package server

import (
	"context"
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/knowledge"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// linkTestServer wires a server whose workspace terms is a fresh in-memory
// store, so promoteRuleToTerms runs with no PostgreSQL. ContentStore is left nil,
// so the source locale defaults to English (the resolution-from-project path is
// covered by TestFirstWorkspaceSourceLocale).
func linkTestServer(t *testing.T) *Server {
	t.Helper()
	srv := shutdownOnCleanup(t, NewServer(DefaultConfig()))
	srv.wsStores.termsFactory = func() terms.Store {
		return &testTermStore{terms.NewInMemoryStore()}
	}
	return srv
}

func eventTypes(events []knowledge.MergeEvent) []knowledge.EventType {
	out := make([]knowledge.EventType, len(events))
	for i, e := range events {
		out[i] = e.Type
	}
	return out
}

// A promoted term joins the concept of its replacement as a forbidden term, so
// the rule the terms store states names what to write instead.
func TestPromoteRuleToTerms_JoinsTheReplacementsConcept(t *testing.T) {
	srv := linkTestServer(t)
	ctx := context.Background()
	const wsSlug, wsID = "acme", "ws-acme"

	changed, conceptID, events, err := srv.promoteRuleToTerms(ctx, wsSlug, wsID,
		coreprofile.SuggestedRule{Term: "utilize", Replacement: "use", CorrectionCount: 4})
	require.NoError(t, err)
	require.True(t, changed)
	require.NotEmpty(t, conceptID)
	assert.Equal(t, []knowledge.EventType{knowledge.EventConceptCreated}, eventTypes(events))

	tb, err := srv.wsStores.getTerms(wsSlug)
	require.NoError(t, err)
	c, ok, err := tb.GetConcept(ctx, conceptID)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "4", c.Properties[terms.PropPromotedFrom])

	rules := workspaceWordRulesFor(t, srv, wsSlug)
	require.Len(t, rules, 1)
	assert.Equal(t, "utilize", rules[0].Term)
	assert.Equal(t, "use", rules[0].Replacement)
	assert.Equal(t, conceptID, rules[0].ConceptID)

	// Promoting it again changes nothing and announces nothing.
	changed, _, events, err = srv.promoteRuleToTerms(ctx, wsSlug, wsID,
		coreprofile.SuggestedRule{Term: "utilize", Replacement: "use", CorrectionCount: 4})
	require.NoError(t, err)
	assert.False(t, changed)
	assert.Empty(t, events)

	// A second term with the same replacement joins the same concept.
	changed, second, events, err := srv.promoteRuleToTerms(ctx, wsSlug, wsID,
		coreprofile.SuggestedRule{Term: "leverage", Replacement: "use"})
	require.NoError(t, err)
	require.True(t, changed)
	assert.Equal(t, conceptID, second)
	assert.Equal(t, []knowledge.EventType{knowledge.EventConceptUpdated}, eventTypes(events))

	// Demoting removes the term again.
	demoted, err := srv.demoteRuleFromTerms(ctx, wsSlug, wsID, "utilize")
	require.NoError(t, err)
	assert.True(t, demoted)
	rules = workspaceWordRulesFor(t, srv, wsSlug)
	require.Len(t, rules, 1)
	assert.Equal(t, "leverage", rules[0].Term)
}

func TestPromoteRuleToTerms_EmptyTermIsNoop(t *testing.T) {
	srv := linkTestServer(t)
	changed, id, events, err := srv.promoteRuleToTerms(context.Background(), "acme", "ws-acme",
		coreprofile.SuggestedRule{Term: "  "})
	require.NoError(t, err)
	assert.False(t, changed)
	assert.Empty(t, id)
	assert.Empty(t, events)
}

func TestPromoteRuleToTerms_UsesProjectSourceLocale(t *testing.T) {
	srv := linkTestServer(t)
	srv.ContentStore = &fakeProjectContentStore{projects: []*platstore.Project{
		{ID: "p1", WorkspaceID: "ws-acme", DefaultSourceLanguage: "de"},
	}}
	ctx := context.Background()

	_, conceptID, _, err := srv.promoteRuleToTerms(ctx, "acme", "ws-acme", coreprofile.SuggestedRule{Term: "nutzen"})
	require.NoError(t, err)

	tb, err := srv.wsStores.getTerms("acme")
	require.NoError(t, err)
	c, ok, err := tb.GetConcept(ctx, conceptID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Len(t, c.Terms, 1)
	assert.Equal(t, model.LocaleID("de"), c.Terms[0].Locale, "the term takes the project's source locale")
	assert.Equal(t, model.TermForbidden, c.Terms[0].Status)
}

// A starter pack's terms land in the terms store: a rule naming a term as a
// forbidden term (a competitor's name, or advisory, when the rule says so), a
// rule naming only a replacement as a preferred term.
func TestLandWordRules_WritesThePacksTerms(t *testing.T) {
	srv := linkTestServer(t)
	ctx := context.Background()

	n, err := srv.landWordRules(ctx, "acme", "ws-acme", []coreprofile.TermRule{
		{Term: "utilize", Replacement: "use"},
		{Term: "Globex", Competitor: true},
		{Term: "simply", Advisory: true},
		{Replacement: "workspace"},
	})
	require.NoError(t, err)
	assert.Equal(t, 4, n)

	byTerm := map[string]coreprofile.TermRule{}
	for _, r := range workspaceWordRulesFor(t, srv, "acme") {
		byTerm[r.Term] = r
	}
	assert.Equal(t, "use", byTerm["utilize"].Replacement)
	assert.True(t, byTerm["Globex"].Competitor)
	assert.True(t, byTerm["simply"].Advisory)

	tb, err := srv.wsStores.getTerms("acme")
	require.NoError(t, err)
	concepts, err := tb.Concepts(ctx)
	require.NoError(t, err)
	ci := terms.IndexOfTerm(concepts, "workspace", "en")
	require.GreaterOrEqual(t, ci, 0, "a preferred form lands as a preferred term")
	assert.Equal(t, model.TermPreferred, concepts[ci].Terms[terms.TermIndex(&concepts[ci], "workspace", "en")].Status)

	// Landing the same rules again changes nothing.
	n, err = srv.landWordRules(ctx, "acme", "ws-acme", []coreprofile.TermRule{{Term: "utilize", Replacement: "use"}})
	require.NoError(t, err)
	assert.Zero(t, n)
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

// Close overrides the nil embedded interface so Server.Shutdown (registered
// by shutdownOnCleanup) does not panic when this fake is installed.
func (f *fakeProjectContentStore) Close() error { return nil }
