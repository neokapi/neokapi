package server

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/termcandidate"
	sqlterms "github.com/neokapi/neokapi/bowrain/terms"
	"github.com/neokapi/neokapi/bowrain/terms/termstest"
	"github.com/neokapi/neokapi/bowrain/termscope"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/terms"
)

// Each test here puts one kind of concept writer in the race its kind must
// win or lose (sqlterms writers_guard_test.go lists them): a person's edit
// lands between the writer's read of the concept and its write.

const personsEdit = "a person's edit"

// interleaveTerms makes the workspace's terms store land a person's edit on
// each concept right after a writer first reads it, and returns the store
// beneath, to read the outcome from.
func interleaveTerms(t *testing.T, srv *Server, slug string) sqlterms.Store {
	t.Helper()
	base, err := srv.wsStores.getTerms(t.Context(), slug)
	require.NoError(t, err)
	w := srv.wsStores.getOrCreate(slug)
	w.mu.Lock()
	w.terms = termstest.Interleaved(base, termstest.SetDefinition(t, personsEdit))
	w.mu.Unlock()
	return base
}

func storedConcept(t *testing.T, tb terms.Terminology, id string) terms.Concept {
	t.Helper()
	c, ok, err := tb.GetConcept(t.Context(), id)
	require.NoError(t, err)
	require.True(t, ok, "concept %s is stored", id)
	return c
}

func memoryScopeBackend(t *testing.T) terms.Store { return termstest.Memory() }

// A person's update names the revision they read. One that names an older
// revision is refused with the concept as it stands, one that names none is
// told to update the app, and the current revision lands.
func TestConceptWriters_APersonsUpdateNamesTheRevisionItRead(t *testing.T) {
	h := newKGHarness(t)
	ctx := t.Context()
	require.NoError(t, h.tb(t).AddConcept(ctx, terms.Concept{ID: "c-save", Domain: "ui",
		Terms: []terms.Term{{Text: "Save", Locale: "en", Status: model.TermApproved}}}))
	read := conceptRevision(t, h.tb(t), "c-save")
	rewriteConcept(t, h.tb(t), terms.Concept{ID: "c-save", Domain: "ui", Definition: personsEdit,
		Terms: []terms.Term{{Text: "Save", Locale: "en", Status: model.TermApproved}}})

	body := `{"domain":"ui","definition":"mine","terms":[{"text":"Save","locale":"en","status":"approved"}]}`
	c, rec := h.req(http.MethodPut, "/", withBase(body, read), platauth.PermManageTerms, "cid", "c-save")
	require.NoError(t, h.srv.HandleUpdateConcept(c))
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	var changed ChangedResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &changed))
	assert.Equal(t, "concept_changed", changed.Code)
	current, _ := json.Marshal(changed.Current)
	assert.Contains(t, string(current), personsEdit, "the refusal carries the concept as it stands")
	assert.Equal(t, personsEdit, storedConcept(t, h.tb(t), "c-save").Definition, "and nothing was written")

	c, rec = h.req(http.MethodPut, "/", body, platauth.PermManageTerms, "cid", "c-save")
	require.NoError(t, h.srv.HandleUpdateConcept(c))
	assert.Equal(t, http.StatusPreconditionRequired, rec.Code, "an update that names no revision is refused")

	c, rec = h.req(http.MethodPut, "/", withBase(body, conceptRevision(t, h.tb(t), "c-save")), platauth.PermManageTerms, "cid", "c-save")
	require.NoError(t, h.srv.HandleUpdateConcept(c))
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	assert.Equal(t, "mine", storedConcept(t, h.tb(t), "c-save").Definition)
}

// Widening applies to the concept as it stands when written.
func TestConceptWriters_WideningKeepsAnEditMadeMeanwhile(t *testing.T) {
	f := newScopeFixture(t, memoryScopeBackend)
	cid := f.keepInProject(t, f.a, "Flowgraph")
	base := interleaveTerms(t, f.srv, "ws")

	require.Equal(t, http.StatusOK, decideRoute(t, f.e, http.MethodPost, "concepts/"+cid, "/widen", nil, nil))
	got := storedConcept(t, base, cid)
	assert.Equal(t, personsEdit, got.Definition, "the person's edit stays")
	assert.True(t, termscope.Workspace(got), "and the concept is widened")
}

// Keeping a candidate is a person's decision on the candidate the card
// showed. One edited between the keep's read and its write is refused with
// the card as it stands, and the edit stays with the candidate unkept.
func TestConceptWriters_KeepingRefusesAnEditMadeMeanwhile(t *testing.T) {
	f := newScopeFixture(t, memoryScopeBackend)
	cid := fileCandidate(t, f.tb, f.a, "Flowgraph", termcandidate.Reach{Blocks: 3}, nil)
	card := deckCardWhere(t, readContextDeck(t, f.e, f.a, ""), candidateCard(cid))
	base := interleaveTerms(t, f.srv, "ws")

	require.Equal(t, http.StatusConflict, decideRoute(t, f.e, http.MethodPost, f.a, "/context/deck/decide",
		contextDeckDecideRequest{Item: card.Item, Revision: card.Revision, BaseRevision: card.BaseRevision, Action: "keep"}, nil))
	got := storedConcept(t, base, cid)
	assert.Equal(t, personsEdit, got.Definition, "the person's edit stays")
	assert.True(t, termcandidate.IsCandidate(got, "en"), "and the candidate is not kept")
}

// Dropping a candidate is a person's decision about the candidate they saw.
// One that another reviewer edited between the drop's read and its delete is
// refused with the card as it stands, and the edit stays.
func TestConceptWriters_DroppingACandidateKeepsAnEditMadeMeanwhile(t *testing.T) {
	f := newScopeFixture(t, memoryScopeBackend)
	cid := fileCandidate(t, f.tb, f.a, "Flowgraph", termcandidate.Reach{Blocks: 3}, nil)
	card := deckCardWhere(t, readContextDeck(t, f.e, f.a, ""), candidateCard(cid))
	base := interleaveTerms(t, f.srv, "ws")

	status := decideRoute(t, f.e, http.MethodPost, f.a, "/context/deck/decide",
		contextDeckDecideRequest{Item: card.Item, Revision: card.Revision, BaseRevision: card.BaseRevision, Action: "reject"}, nil)
	assert.Equal(t, http.StatusConflict, status, "the drop is refused")
	got := storedConcept(t, base, cid)
	assert.Equal(t, personsEdit, got.Definition, "and the edit stays with its concept")
}

// postgresScopeBackend is the platform's PostgreSQL terms store.
func postgresScopeBackend(t *testing.T) terms.Store { return scopeBackends[1].open(t) }

// A reviewer dealt a candidate decides on the candidate as the card showed
// it. When another reviewer edited it since the deal, a drop or a keep sent
// with the dealt base revision is refused with the card as it stands, and
// the other reviewer's edit stays. A decision that names no base revision is
// refused and told to update the app.
func TestConceptWriters_ADeckDecisionActsAtTheDealtRevision(t *testing.T) {
	for _, action := range []string{"reject", "keep"} {
		t.Run(action, func(t *testing.T) {
			f := newScopeFixture(t, postgresScopeBackend)
			cid := fileCandidate(t, f.tb, f.a, "Flowgraph", termcandidate.Reach{Blocks: 3}, nil)
			card := deckCardWhere(t, readContextDeck(t, f.e, f.a, ""), candidateCard(cid))

			// Reviewer B edits the candidate after A was dealt it.
			edited := storedConcept(t, f.tb, cid)
			edited.Definition = personsEdit
			rewriteConcept(t, f.tb, edited)

			var refusal map[string]any
			status := decideRoute(t, f.e, http.MethodPost, f.a, "/context/deck/decide",
				contextDeckDecideRequest{Item: card.Item, Revision: card.Revision, BaseRevision: card.BaseRevision, Action: action}, &refusal)
			require.Equal(t, http.StatusConflict, status, "a decision on the dealt revision is refused")
			assert.Equal(t, deckCodeStale, refusal["code"])
			got := storedConcept(t, f.tb, cid)
			assert.Equal(t, personsEdit, got.Definition, "B's edit stays")
			assert.True(t, termcandidate.IsCandidate(got, "en"), "and the candidate is as B left it")

			fresh := deckCardWhere(t, readContextDeck(t, f.e, f.a, ""), candidateCard(cid))
			require.NotEqual(t, card.BaseRevision, fresh.BaseRevision, "the edit moved the card's base revision")

			// An app that sends no base revision is refused and told to
			// update, never applied to the candidate as it stands.
			status = decideRoute(t, f.e, http.MethodPost, f.a, "/context/deck/decide",
				contextDeckDecideRequest{Item: fresh.Item, Revision: fresh.Revision, Action: action}, &refusal)
			assert.Equal(t, http.StatusPreconditionRequired, status, "a decision with no base revision is refused")
			assert.Equal(t, deckCodeUpdateApp, refusal["code"])
			assert.True(t, termcandidate.IsCandidate(storedConcept(t, f.tb, cid), "en"), "and nothing was decided")

			assert.Equal(t, http.StatusOK, decideRoute(t, f.e, http.MethodPost, f.a, "/context/deck/decide",
				contextDeckDecideRequest{Item: fresh.Item, Revision: fresh.Revision, BaseRevision: fresh.BaseRevision, Action: action}, nil))
		})
	}
}

// A translation decision applies to the proposal the person saw, on the
// concept as it stands when written.
func TestConceptWriters_ATranslationDecisionKeepsAnEditMadeMeanwhile(t *testing.T) {
	caller := &decideCaller{userID: "member-a", perms: decideAll}
	e, cs, _, tb, srv := deckServer(t, caller)
	pid := seedDecideParity(t, e, cs)
	proposeTranslation(t, tb, "c-memory", "fr", "Mémoire de contenu")
	card := deckCardWhere(t, readContextDeck(t, e, pid, ""), translationCard("c-memory", "fr"))
	base := interleaveTerms(t, srv, "ws")

	require.Equal(t, http.StatusOK, decideRoute(t, e, http.MethodPost, pid, "/context/deck/decide",
		contextDeckDecideRequest{Item: card.Item, Revision: card.Revision, BaseRevision: card.BaseRevision, Action: "reject"}, nil))
	got := storedConcept(t, base, "c-memory")
	assert.Equal(t, personsEdit, got.Definition, "the person's edit stays")
	assert.Equal(t, []string{"mémoire de contenu"}, termcandidate.RejectedTranslations(got, "fr"), "and the decision lands")
}

// Promoting and demoting a rule fold into the concepts as they stand.
func TestConceptWriters_PromotingARuleKeepsAnEditMadeMeanwhile(t *testing.T) {
	srv := linkTestServer(t)
	ctx := context.Background()
	const wsSlug, wsID = "acme", "ws-acme"
	tb, err := srv.wsStores.getTerms(ctx, wsSlug)
	require.NoError(t, err)
	_, err = tb.CreateConcept(ctx, terms.Concept{ID: "c-use", Terms: []terms.Term{
		{Text: "use", Locale: "en", Status: model.TermPreferred},
	}}, "")
	require.NoError(t, err)
	base := interleaveTerms(t, srv, wsSlug)

	changed, cid, _, err := srv.promoteRuleToTerms(ctx, wsSlug, wsID, coreprofile.SuggestedRule{Term: "utilize", Replacement: "use"})
	require.NoError(t, err)
	require.True(t, changed)
	assert.Equal(t, "c-use", cid)
	got := storedConcept(t, base, "c-use")
	assert.Equal(t, personsEdit, got.Definition, "the person's edit stays")
	assert.Len(t, got.Terms, 2, "and the promoted term joins the concept")

	rewriteConcept(t, base, terms.Concept{ID: "c-use", Definition: "edited again", Terms: got.Terms})
	demoted, err := srv.demoteRuleFromTerms(ctx, wsSlug, wsID, "utilize")
	require.NoError(t, err)
	assert.True(t, demoted)
	got = storedConcept(t, base, "c-use")
	assert.Equal(t, "edited again", got.Definition)
	assert.Len(t, got.Terms, 1)
}

// A voice file's word rules fold into the concepts as they stand.
func TestConceptWriters_LandingWordRulesKeepsAnEditMadeMeanwhile(t *testing.T) {
	srv := linkTestServer(t)
	ctx := context.Background()
	const wsSlug, wsID = "acme", "ws-acme"
	tb, err := srv.wsStores.getTerms(ctx, wsSlug)
	require.NoError(t, err)
	_, err = tb.CreateConcept(ctx, terms.Concept{ID: "c-use", Terms: []terms.Term{
		{Text: "use", Locale: "en", Status: model.TermPreferred},
	}}, "")
	require.NoError(t, err)
	base := interleaveTerms(t, srv, wsSlug)

	n, err := srv.landWordRules(ctx, wsSlug, wsID, []coreprofile.TermRule{{Term: "utilize", Replacement: "use"}})
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	got := storedConcept(t, base, "c-use")
	assert.Equal(t, personsEdit, got.Definition, "the person's edit stays")
	assert.Len(t, got.Terms, 2, "and the word rule joins the concept")
}

// A scan refreshing its own candidate refreshes it as it stands, and leaves
// one a person has kept since.
func TestConceptWriters_RefreshingACandidateKeepsAnEditMadeMeanwhile(t *testing.T) {
	st := termstest.Memory()
	ctx := t.Context()
	res, err := termcandidate.File(ctx, st, "p1", "en", []termcandidate.Candidate{{Term: "Growing", Reach: termcandidate.Reach{Blocks: 3}}},
		nil, termcandidate.ScanIdentity, sessionNow())
	require.NoError(t, err)
	require.Len(t, res.Filed, 1)
	raced := termstest.Interleaved(st, termstest.SetDefinition(t, personsEdit))

	res, err = termcandidate.File(ctx, raced, "p1", "en", []termcandidate.Candidate{{Term: "Growing", Reach: termcandidate.Reach{Blocks: 30}}},
		nil, termcandidate.ScanIdentity, sessionNow())
	require.NoError(t, err)
	assert.Len(t, res.Refreshed, 1)
	got := storedConcept(t, st, res.Refreshed[0])
	assert.Equal(t, personsEdit, got.Definition, "the person's edit stays")
}
