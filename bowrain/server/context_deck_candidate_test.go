package server

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/termcandidate"
	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
)

// fileCandidate files a candidate term the way a context scan does.
func fileCandidate(t *testing.T, tb terms.Terminology, pid, term string, reach termcandidate.Reach, at map[string]string) string {
	t.Helper()
	res, err := termcandidate.File(t.Context(), tb, pid, "en", []termcandidate.Candidate{
		{Term: term, Definition: "what " + term + " means", Reach: reach, Coordinates: at},
	}, nil, termcandidate.ScanIdentity, time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Len(t, res.Filed, 1)
	return res.Filed[0]
}

func candidateCard(id string) func(contextDeckCard) bool {
	return func(c contextDeckCard) bool { return c.Item.Kind == contextop.ItemConcept && c.Item.ID == id }
}

// TestContextDeck_CandidatesDealFirstByReach: candidate terms come before
// every other card, the widest reach first, each with its reach, its
// coordinates and the scan as its author.
func TestContextDeck_CandidatesDealFirstByReach(t *testing.T) {
	caller := &decideCaller{userID: "member-a", perms: decideAll}
	e, cs, _, tb := deckSQLiteServerWithTerms(t, caller)
	pid := seedDecideParity(t, e, cs)

	narrow := fileCandidate(t, tb, pid, "Dashpanel", termcandidate.Reach{
		Blocks: 6, Collections: []termcandidate.CollectionCount{{Name: "bowrain-app", Blocks: 6}},
	}, map[string]string{"product": "bowrain"})
	wide := fileCandidate(t, tb, pid, "Flowgraph", termcandidate.Reach{
		Blocks: 40, Collections: []termcandidate.CollectionCount{{Name: "docs", Blocks: 30}, {Name: "cli", Blocks: 10}},
	}, nil)
	// A candidate of another project is not dealt here.
	fileCandidate(t, tb, "another-project", "Elsewhere", termcandidate.Reach{Blocks: 99}, nil)

	deck := readContextDeck(t, e, pid, "")
	require.GreaterOrEqual(t, len(deck.Cards), 3)
	assert.Equal(t, wide, deck.Cards[0].Item.ID, "the widest reach leads")
	assert.Equal(t, narrow, deck.Cards[1].Item.ID)
	assert.False(t, deck.Cards[2].Proposed, "then the rest of the deck")
	assert.Equal(t, 2, deck.Counts.Proposed)
	assert.Equal(t, 3, deck.Counts.ByKind["concept"])

	card := deck.Cards[0]
	assert.True(t, card.Proposed)
	assert.Equal(t, "concept", card.Kind)
	require.NotNil(t, card.Reach)
	assert.Equal(t, 40, card.Reach.Blocks)
	assert.Equal(t, []contextDeckCollectionReach{{Name: "docs", Blocks: 30}, {Name: "cli", Blocks: 10}}, card.Reach.Collections)
	assert.Equal(t, "project", card.Where.Level)
	assert.Empty(t, card.Where.Coordinates)
	assert.Equal(t, "tool", card.Author.Kind)
	assert.Equal(t, "context-scan", card.Author.Name)
	assert.Equal(t, contextop.BasisAgentAuthored, card.Basis)
	assert.Equal(t, []string{"keep", "reject", "later"}, card.Actions)
	require.Len(t, card.Terms, 1)
	assert.Equal(t, "proposed", card.Terms[0].Status)

	assert.Equal(t, map[string]string{"product": "bowrain"}, deck.Cards[1].Where.Coordinates)
}

// TestContextDeck_KeepCandidatePromotesIt: keep makes the proposed term
// preferred and attests the concept as it then reads, so it waits no longer.
func TestContextDeck_KeepCandidatePromotesIt(t *testing.T) {
	caller := &decideCaller{userID: "member-a", perms: decideAll}
	e, cs, ks, tb := deckSQLiteServerWithTerms(t, caller)
	pid := seedDecideParity(t, e, cs)
	cid := fileCandidate(t, tb, pid, "Flowgraph", termcandidate.Reach{Blocks: 4}, nil)

	card := deckCardWhere(t, readContextDeck(t, e, pid, ""), candidateCard(cid))
	var out contextDeckDecideResponse
	require.Equal(t, http.StatusOK, decideRoute(t, e, http.MethodPost, pid, "/context/deck/decide",
		contextDeckDecideRequest{Item: card.Item, Revision: card.Revision, Action: "keep"}, &out))
	assert.Equal(t, deckOutcomeKept, out.Outcome)
	assert.Empty(t, ks.changesets, "keeping a candidate proposes no change set")

	cp, ok, err := tb.GetConcept(t.Context(), cid)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, model.TermPreferred, cp.Terms[0].Status)

	after := readContextDeck(t, e, pid, "")
	assert.Zero(t, after.Counts.Proposed)
	for _, c := range after.Cards {
		assert.NotEqual(t, cid, c.Item.ID, "the kept term is attested at its new revision")
	}
}

// TestContextDeck_RejectCandidateForgetsIt: reject deletes the candidate and
// records it on the project, so the scan never files it again.
func TestContextDeck_RejectCandidateForgetsIt(t *testing.T) {
	caller := &decideCaller{userID: "member-a", perms: decideAll}
	e, cs, ks, tb := deckSQLiteServerWithTerms(t, caller)
	pid := seedDecideParity(t, e, cs)
	cid := fileCandidate(t, tb, pid, "Synergy", termcandidate.Reach{Blocks: 2}, nil)

	card := deckCardWhere(t, readContextDeck(t, e, pid, ""), candidateCard(cid))
	var out contextDeckDecideResponse
	require.Equal(t, http.StatusOK, decideRoute(t, e, http.MethodPost, pid, "/context/deck/decide",
		contextDeckDecideRequest{Item: card.Item, Revision: card.Revision, Action: "reject"}, &out))
	assert.Equal(t, deckOutcomeDropped, out.Outcome)
	assert.Empty(t, ks.changesets, "a candidate is deleted, not retired through a change set")

	_, ok, err := tb.GetConcept(t.Context(), cid)
	require.NoError(t, err)
	assert.False(t, ok)

	p, err := cs.GetProject(t.Context(), pid)
	require.NoError(t, err)
	rejected := termcandidate.Rejected(p.Properties)
	assert.True(t, rejected["synergy"])

	res, err := termcandidate.File(t.Context(), tb, pid, "en", []termcandidate.Candidate{{Term: "Synergy"}},
		rejected, termcandidate.ScanIdentity, time.Now())
	require.NoError(t, err)
	assert.Empty(t, res.Filed)
	assert.Equal(t, []string{"Synergy"}, res.Rejected)
}
