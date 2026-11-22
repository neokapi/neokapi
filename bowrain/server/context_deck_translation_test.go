package server

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/bowrain/termcandidate"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
)

// wireTermProposalJobs gives a deck server a real job store and an
// in-memory queue, and returns the queue.
func wireTermProposalJobs(t *testing.T, srv *Server) *jobs.ChannelQueue {
	t.Helper()
	store, err := jobs.NewContextScanJobStore(pgtest.NewTestDB(t))
	require.NoError(t, err)
	q := jobs.NewChannelQueue(8)
	srv.ContextScanStore, srv.ContextScanQueue = store, q
	return q
}

// nextTermProposalJob reads the job the queue holds and its request.
func nextTermProposalJob(t *testing.T, srv *Server, q *jobs.ChannelQueue) jobs.ContextScanRequest {
	t.Helper()
	jobID, ack, _, err := q.Dequeue(t.Context())
	require.NoError(t, err)
	ack()
	job, err := srv.ContextScanStore.GetContextScanJob(t.Context(), jobID)
	require.NoError(t, err)
	var req jobs.ContextScanRequest
	require.NoError(t, json.Unmarshal(job.Request, &req))
	return req
}

func translationCard(conceptID, locale string) func(contextDeckCard) bool {
	return func(c contextDeckCard) bool {
		return c.Item.Kind == itemTermProposal && c.Item.ID == conceptID+termProposalSep+locale
	}
}

// proposeTranslation files a target-term proposal the way the job does.
func proposeTranslation(t *testing.T, tb terms.Terminology, cid string, locale model.LocaleID, text string) {
	t.Helper()
	cp, ok, err := tb.GetConcept(t.Context(), cid)
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, termcandidate.SetTranslation(&cp, "en", locale, text))
	require.NoError(t, tb.AddConcept(t.Context(), cp))
}

// TestContextDeck_KeepTermThenItsTranslation: keeping a candidate starts a
// job proposing its target terms; the proposal is dealt first, with the
// source term, the proposed term, the language and the reach; keeping it
// with the reviewer's wording adds that term as preferred and attests the
// concept again, so neither card waits any longer.
func TestContextDeck_KeepTermThenItsTranslation(t *testing.T) {
	caller := &decideCaller{userID: "member-a", perms: decideAll}
	e, cs, ks, tb, srv := deckServer(t, caller)
	q := wireTermProposalJobs(t, srv)
	pid := seedDecideParity(t, e, cs)
	cid := fileCandidate(t, tb, pid, "Flowgraph", termcandidate.Reach{
		Blocks: 40, Collections: []termcandidate.CollectionCount{{Name: "docs", Blocks: 40}},
	}, nil)

	card := deckCardWhere(t, readContextDeck(t, e, pid, ""), candidateCard(cid))
	var out contextDeckDecideResponse
	require.Equal(t, http.StatusOK, decideRoute(t, e, http.MethodPost, pid, "/context/deck/decide",
		contextDeckDecideRequest{Item: card.Item, Revision: card.Revision, Action: "keep"}, &out))
	req := nextTermProposalJob(t, srv, q)
	assert.Equal(t, pid, req.Project)
	require.NotNil(t, req.TermProposals)
	assert.Equal(t, []string{cid}, req.TermProposals.Concepts)

	proposeTranslation(t, tb, cid, "fr", "graphe de flux")
	deck := readContextDeck(t, e, pid, "")
	require.NotEmpty(t, deck.Cards)
	tr := deck.Cards[0]
	assert.Equal(t, deckKindTermTranslation, tr.Kind, "the proposal leads: its concept is reviewed")
	require.NotNil(t, tr.Translation)
	assert.Equal(t, contextDeckTranslation{Term: "Flowgraph", Proposed: "graphe de flux", Locale: "fr"}, *tr.Translation)
	assert.Equal(t, "fr", tr.Language)
	require.NotNil(t, tr.Reach)
	assert.Equal(t, 40, tr.Reach.Blocks)
	assert.Equal(t, "term-proposal", tr.Author.Name)
	assert.Equal(t, contextop.BasisAgentAuthored, tr.Basis)
	assert.Equal(t, []string{"keep", "reject", "later"}, tr.Actions)
	assert.Equal(t, 1, deck.Counts.ByKind[deckKindTermTranslation])

	require.Equal(t, http.StatusOK, decideRoute(t, e, http.MethodPost, pid, "/context/deck/decide",
		contextDeckDecideRequest{Item: tr.Item, Revision: tr.Revision, Action: "keep", Text: "graphe de flot"}, &out))
	assert.Equal(t, deckOutcomeKept, out.Outcome)
	assert.Empty(t, ks.changesets, "keeping a proposal proposes no change set")

	cp, _, err := tb.GetConcept(t.Context(), cid)
	require.NoError(t, err)
	rule, ok := terms.RuleForConcept(cp, "en", "fr")
	require.True(t, ok)
	assert.Equal(t, "graphe de flot", rule.Replacement, "the reviewer's wording wins")
	assert.Empty(t, termcandidate.Translations(cp))

	for _, c := range readContextDeck(t, e, pid, "").Cards {
		assert.NotEqual(t, cid, c.Item.ID, "the concept is attested at its new revision")
		assert.NotEqual(t, deckKindTermTranslation, c.Kind)
	}
}

// TestContextDeck_TranslationFollowsItsConcept: a proposal for a concept
// still waiting for review is dealt right after that concept's card.
func TestContextDeck_TranslationFollowsItsConcept(t *testing.T) {
	caller := &decideCaller{userID: "member-a", perms: decideAll}
	e, cs, _, tb := deckSQLiteServerWithTerms(t, caller)
	pid := seedDecideParity(t, e, cs)
	at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, tb.AddConcept(t.Context(), terms.Concept{
		ID: "c-workspace", Terms: []terms.Term{{Text: "workspace", Locale: "en", Status: model.TermPreferred}},
		Properties: map[string]string{termcandidate.PropProject: pid}, CreatedAt: at, UpdatedAt: at,
	}))
	fileCandidate(t, tb, pid, "Flowgraph", termcandidate.Reach{Blocks: 9}, nil)
	proposeTranslation(t, tb, "c-workspace", "fr", "espace de travail")

	cards := readContextDeck(t, e, pid, "").Cards
	concept := slices.IndexFunc(cards, func(c contextDeckCard) bool { return c.Item.ID == "c-workspace" })
	require.GreaterOrEqual(t, concept, 0)
	require.Less(t, concept+1, len(cards))
	assert.True(t, translationCard("c-workspace", "fr")(cards[concept+1]), "the proposal follows its source term")
	assert.True(t, cards[0].Proposed, "candidates still lead")
}

// TestContextDeck_RejectTranslationRemembersIt: reject clears the proposal
// and records its text, so a later run does not propose it again.
func TestContextDeck_RejectTranslationRemembersIt(t *testing.T) {
	caller := &decideCaller{userID: "member-a", perms: decideAll}
	e, cs, _, tb := deckSQLiteServerWithTerms(t, caller)
	pid := seedDecideParity(t, e, cs)
	proposeTranslation(t, tb, "c-memory", "fr", "Mémoire de traduction")

	card := deckCardWhere(t, readContextDeck(t, e, pid, ""), translationCard("c-memory", "fr"))
	var out contextDeckDecideResponse
	require.Equal(t, http.StatusOK, decideRoute(t, e, http.MethodPost, pid, "/context/deck/decide",
		contextDeckDecideRequest{Item: card.Item, Revision: card.Revision, Action: "reject"}, &out))
	assert.Equal(t, deckOutcomeDropped, out.Outcome)

	cp, _, err := tb.GetConcept(t.Context(), "c-memory")
	require.NoError(t, err)
	assert.Empty(t, termcandidate.TranslationFor(cp, "fr"))
	assert.Equal(t, []string{"mémoire de traduction"}, termcandidate.RejectedTranslations(cp, "fr"))
	assert.False(t, termcandidate.SetTranslation(&cp, "en", "fr", "mémoire de traduction"))
	for _, c := range readContextDeck(t, e, pid, "").Cards {
		assert.NotEqual(t, deckKindTermTranslation, c.Kind)
	}
}

// TestContextDeck_TranslationOnlyInTheReviewersLanguages: a reviewer scoped
// to other languages is not dealt the proposal.
func TestContextDeck_TranslationOnlyInTheReviewersLanguages(t *testing.T) {
	caller := &decideCaller{userID: "member-a", perms: decideAll, languages: []string{"de"}}
	e, cs, _, tb := deckSQLiteServerWithTerms(t, caller)
	pid := seedDecideParity(t, e, cs)
	proposeTranslation(t, tb, "c-memory", "fr", "mémoire de contenu")
	for _, c := range readContextDeck(t, e, pid, "").Cards {
		assert.NotEqual(t, deckKindTermTranslation, c.Kind)
	}
}

// TestProposeTermTranslations_StartsAJob: the endpoint counts the concept
// and language pairs with no term and starts a job for the project, and
// starts none when nothing waits.
func TestProposeTermTranslations_StartsAJob(t *testing.T) {
	caller := &decideCaller{userID: "member-a", perms: decideAll}
	e, cs, _, tb, srv := deckServer(t, caller)
	q := wireTermProposalJobs(t, srv)
	pid := seedDecideParity(t, e, cs)

	var out termProposalsResponse
	require.Equal(t, http.StatusAccepted, decideRoute(t, e, http.MethodPost, pid, "/context/term-proposals", nil, &out))
	assert.Equal(t, 1, out.Waiting, "content memory has no French term")
	assert.NotEmpty(t, out.JobID)
	req := nextTermProposalJob(t, srv, q)
	assert.Equal(t, pid, req.Project)
	require.NotNil(t, req.TermProposals)
	assert.Empty(t, req.TermProposals.Concepts)

	proposeTranslation(t, tb, "c-memory", "fr", "mémoire de contenu")
	out = termProposalsResponse{}
	require.Equal(t, http.StatusOK, decideRoute(t, e, http.MethodPost, pid, "/context/term-proposals", nil, &out))
	assert.Zero(t, out.Waiting)
	assert.Empty(t, out.JobID)
}
