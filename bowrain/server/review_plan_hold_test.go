package server

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/host/reviewdeck"
	"github.com/neokapi/neokapi/terms"
)

// heldDeckProject seeds a project whose French deck holds two fast, two
// closer and one desk translation.
func heldDeckProject(t *testing.T, s *Server, wsID string) string {
	t.Helper()
	long := strings.Repeat("A long paragraph of source text. ", 10)
	projID, ids := seedGovernedProject(t, s, wsID, []*model.Block{
		pendingFrBlock("b2", "Closer low", "Bonjour bas"),
		pendingFrBlock("b3", "Fast one", "Bonjour un"),
		pendingFrBlock("b4", "Closer unscored", "Bonjour sans"),
		pendingFrBlock("b5", long, "Un long paragraphe."),
		pendingFrBlock("b7", "Fast two", "Bonjour deux"),
	})
	recordScore(t, s, projID, ids["Fast one"], 92)
	recordScore(t, s, projID, ids["Fast two"], 80)
	recordScore(t, s, projID, ids["Closer low"], 50)
	recordScore(t, s, projID, ids[long], 95)
	return projID
}

// openKeyLanguage adds a concept with no French term, which leaves one
// key-language decision open.
func openKeyLanguage(t *testing.T, s *Server) {
	t.Helper()
	tb, err := s.wsStores.getTerms(t.Context(), "rc")
	require.NoError(t, err)
	require.NoError(t, tb.AddConcept(t.Context(), terms.Concept{ID: "c-workspace",
		Terms: []terms.Term{{Text: "workspace", Locale: "en", Status: model.TermPreferred}}}))
}

func TestReviewDeck_HoldsTranslationsWhileTheKeyLanguageIsOpen(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	projID := heldDeckProject(t, s, wsID)
	query := url.Values{"locale": {"fr"}}

	open := readDeck(t, s, wsID, projID, query)
	assert.Nil(t, open.Held, "nothing is held while the key language has no open decision")
	assert.Equal(t, 5, open.Counts.Waiting)

	openKeyLanguage(t, s)
	deck := readDeck(t, s, wsID, projID, query)
	require.NotNil(t, deck.Held)
	assert.Equal(t, deckHeld{Count: 4, Reason: heldKeyLanguage}, *deck.Held)
	assert.Equal(t, deckCounts{Waiting: 1, Desk: 1, Held: 4, Total: 1}, deck.Counts)
	require.Len(t, deck.Cards, 1, "the exceptions stay reviewable")
	assert.Equal(t, reviewdeck.DeckDesk, deck.Cards[0].Group)

	fast := readDeck(t, s, wsID, projID, url.Values{"locale": {"fr"}, "group": {"fast"}})
	assert.Empty(t, fast.Cards, "a held group deals no cards")

	rec := deckCall(t, s, s.HandleGetReviewDeckCollections, wsID, projID, "", "u-reviewer", testReviewer, nil, query)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	colls := decodeJSON[reviewDeckCollectionsResponse](t, rec)
	assert.Equal(t, 1, colls.All.Waiting)
	assert.Equal(t, 4, colls.All.Held)
	assert.Zero(t, colls.All.Decided, "a held translation is not decided")

	plan := readPlan(t, s, wsID, projID)
	assert.Equal(t, heldKeyLanguage, plan.Held)
	require.Len(t, plan.Languages, 1)
	assert.Equal(t, 4, plan.Languages[0].Held)
	assert.Equal(t, 2, plan.Languages[0].Settleable, "a first pass still settles the fast group")
}

func TestReviewDeck_HoldsNothingUnderEvery(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	projID := heldDeckProject(t, s, wsID)
	openKeyLanguage(t, s)
	require.Equal(t, 204,
		planWrite(t, s, s.HandlePutReviewInvolvement, wsID, projID, `{"involvement":"every"}`, platauth.PermAll).Code)

	deck := readDeck(t, s, wsID, projID, url.Values{"locale": {"fr"}})
	assert.Nil(t, deck.Held)
	assert.Equal(t, 5, deck.Counts.Waiting)
	assert.Len(t, deck.Cards, 5)
	assert.Empty(t, readPlan(t, s, wsID, projID).Held)
}

func TestReviewInbox_LeavesHeldTranslationsOutOfTheCounts(t *testing.T) {
	s, wsID, owner := newRecheckHarness(t)
	projID := heldDeckProject(t, s, wsID)
	openKeyLanguage(t, s)

	// Before any deck request classifies the language, nothing is known to
	// fail a check, so every translation is held and the language is not
	// listed; the project still is, with its plan.
	inbox := readInbox(t, s, owner)
	assert.Empty(t, inbox.Languages)
	require.Len(t, inbox.Projects, 1)
	require.NotNil(t, inbox.Projects[0].Plan)
	assert.Equal(t, []planHeldDTO{{Locale: "fr", Count: 5, Reason: heldKeyLanguage}}, inbox.Projects[0].Plan.Held)

	readDeck(t, s, wsID, projID, url.Values{"locale": {"fr"}})
	inbox = readInbox(t, s, owner)
	require.Len(t, inbox.Languages, 1)
	fr := inbox.Languages[0]
	assert.Equal(t, 1, fr.Waiting, "only the exception waits")
	assert.Equal(t, 4, fr.Held)
	assert.Zero(t, fr.Fast+fr.Closer)

	require.Len(t, inbox.Projects, 1)
	p := inbox.Projects[0]
	assert.Equal(t, projID, p.Project)
	require.NotNil(t, p.Plan)
	assert.Equal(t, []planHeldDTO{{Locale: "fr", Count: 4, Reason: heldKeyLanguage}}, p.Plan.Held)
}

func TestReviewPlan_KeyLanguageWithNoScanHasNotStarted(t *testing.T) {
	s, wsID, _ := newRecheckHarness(t)
	projID, _ := containerProject(t, s, wsID)
	require.Equal(t, http.StatusNoContent,
		planWrite(t, s, s.HandleConfirmReviewCoordinates, wsID, projID, "", platauth.PermAll).Code)

	plan := readPlan(t, s, wsID, projID)
	require.Len(t, plan.Steps, 5)
	assert.Equal(t, stepDone, plan.Steps[0].State)
	key := plan.Steps[1]
	assert.Equal(t, planKeyLanguage, key.Level)
	assert.Equal(t, stepNotStarted, key.State, "no scan and no concepts")
	assert.False(t, key.Done)
	assert.Equal(t, planKeyLanguage, plan.Next, "a key language that has not started comes after the coordinates")

	openKeyLanguage(t, s)
	plan = readPlan(t, s, wsID, projID)
	assert.Equal(t, stepOpen, plan.Steps[1].State)
}
