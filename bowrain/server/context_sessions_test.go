package server

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/termcandidate"
	"github.com/neokapi/neokapi/terms"
)

// sessionClock pins the clock sessions read for one test.
func sessionClock(t *testing.T, at time.Time) *time.Time {
	t.Helper()
	clock := at
	prev := sessionNow
	sessionNow = func() time.Time { return clock }
	t.Cleanup(func() { sessionNow = prev })
	return &clock
}

// fileCandidateAt files a candidate term as a scan running at at would.
func fileCandidateAt(t *testing.T, tb terms.Terminology, pid, term string, blocks int, at time.Time) string {
	t.Helper()
	res, err := termcandidate.File(t.Context(), tb, pid, "en", []termcandidate.Candidate{{
		Term: term, Definition: "what " + term + " means",
		Reach: termcandidate.Reach{Blocks: blocks, Collections: []termcandidate.CollectionCount{{Name: "cli-pages", Blocks: blocks}}},
	}}, nil, termcandidate.ScanIdentity, at)
	require.NoError(t, err)
	require.Len(t, res.Filed, 1)
	return res.Filed[0]
}

func startSession(t *testing.T, e *echo.Echo, pid string) contextSessionResponse {
	t.Helper()
	var out contextSessionResponse
	require.Equal(t, http.StatusOK, decideRoute(t, e, http.MethodPost, pid, "/context/sessions", nil, &out))
	return out
}

func endSession(t *testing.T, e *echo.Echo, pid, sid string) contextSessionEndResponse {
	t.Helper()
	var out contextSessionEndResponse
	require.Equal(t, http.StatusOK, decideRoute(t, e, http.MethodPost, pid, "/context/sessions/"+sid+"/end", nil, &out))
	return out
}

func decideSessionCard(t *testing.T, e *echo.Echo, pid string, card contextSessionCard, action string) int {
	t.Helper()
	var out contextDeckDecideResponse
	return decideRoute(t, e, http.MethodPost, pid, "/context/deck/decide",
		contextDeckDecideRequest{Item: card.Item, Revision: card.Revision, Action: action}, &out)
}

func sessionKeys(cards []contextSessionCard) map[string]bool {
	out := map[string]bool{}
	for _, c := range cards {
		out[c.Item.Key()] = true
	}
	return out
}

// Two people who start sessions at once get disjoint cards, widest reach
// first; a card one session holds leaves the other person's listing, and
// deciding it from there is refused as held; an expired session releases
// what it held.
func TestContextSessions_TwoReviewersGetDisjointCards(t *testing.T) {
	clock := sessionClock(t, time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC))
	caller := &decideCaller{userID: "member-a", perms: decideAll}
	e, cs, _, tb := deckSQLiteServerWithTerms(t, caller)
	pid := seedDecideParity(t, e, cs)
	filed := time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC)
	for i := 1; i <= 12; i++ {
		fileCandidateAt(t, tb, pid, fmt.Sprintf("Term%02d", i), i*10, filed)
	}

	ada := startSession(t, e, pid)
	require.Len(t, ada.Cards, contextSessionSize)
	assert.Equal(t, "Term12", ada.Cards[0].Terms[0].Text, "the widest reach leads")
	assert.Equal(t, 120, ada.Cards[0].Reach.Blocks)
	assert.Equal(t, "Term03", ada.Cards[9].Terms[0].Text)
	assert.Nil(t, ada.Since, "a project's first session has nothing to compare against")
	assert.Positive(t, ada.Waiting)

	caller.userID = "member-b"
	bea := startSession(t, e, pid)
	require.NotEmpty(t, bea.Cards)
	held := sessionKeys(ada.Cards)
	for key := range sessionKeys(bea.Cards) {
		assert.False(t, held[key], "%s is held by both sessions", key)
	}
	assert.Equal(t, contextSessionSize, bea.HeldByOthers)
	assert.Equal(t, "Term02", bea.Cards[0].Terms[0].Text, "the widest card no one holds")

	deck := readContextDeck(t, e, pid, "?limit=200")
	for _, c := range deck.Cards {
		assert.False(t, held[c.Item.Key()], "Bea's deck lists %s, which Ada's session holds", c.Item.Key())
	}
	assert.Equal(t, http.StatusConflict, decideSessionCard(t, e, pid, ada.Cards[0], "keep"))

	// Ada decides a card; it leaves every queue.
	caller.userID = "member-a"
	require.Equal(t, http.StatusOK, decideSessionCard(t, e, pid, ada.Cards[0], "keep"))
	caller.userID = "member-b"
	for _, c := range readContextDeck(t, e, pid, "?limit=200").Cards {
		assert.NotEqual(t, ada.Cards[0].Item.Key(), c.Item.Key())
	}

	// Ada leaves without ending her session. Once it expires, her cards are
	// back in Bea's listing.
	*clock = clock.Add(contextSessionIdle + time.Minute)
	listed := sessionKeys(nil)
	for _, c := range readContextDeck(t, e, pid, "?limit=200").Cards {
		listed[c.Item.Key()] = true
	}
	assert.True(t, listed[ada.Cards[1].Item.Key()], "an expired session releases its cards")
	again := startSession(t, e, pid)
	assert.Equal(t, ada.Cards[1].Item.Key(), again.Cards[0].Item.Key())
}

// A session's summary counts its decisions and the blocks the kept terms
// reach. The next session, by another person, says what changed since: new
// candidates, terms whose reach grew, and what others decided meanwhile.
func TestContextSessions_SummaryAndSince(t *testing.T) {
	clock := sessionClock(t, time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC))
	caller := &decideCaller{userID: "member-a", perms: decideAll}
	e, cs, _, tb := deckSQLiteServerWithTerms(t, caller)
	pid := seedDecideParity(t, e, cs)
	filed := time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC)
	require.NoError(t, cs.CreateCollection(t.Context(), &platstore.Collection{
		ProjectID: pid, Name: "cli-pages", Kind: platstore.CollectionUploaded, ItemLabel: "file",
		Stream: "main", Context: map[string]string{"channel": "cli"}, Owner: "recipe",
	}))
	fileCandidateAt(t, tb, pid, "Flowgraph", 40, filed)
	fileCandidateAt(t, tb, pid, "Dashpanel", 25, filed)
	fileCandidateAt(t, tb, pid, "Synergy", 9, filed)
	fileCandidateAt(t, tb, pid, "Growing", 3, filed)

	ada := startSession(t, e, pid)
	require.GreaterOrEqual(t, len(ada.Cards), 3)
	assert.Equal(t, []string{"cli"}, ada.Cards[0].Channels, "the collection the term is used in sits in the cli channel")
	*clock = clock.Add(time.Minute)
	require.Equal(t, http.StatusOK, decideSessionCard(t, e, pid, ada.Cards[0], "keep"))
	require.Equal(t, http.StatusOK, decideSessionCard(t, e, pid, ada.Cards[1], "keep"))
	require.Equal(t, http.StatusOK, decideSessionCard(t, e, pid, ada.Cards[2], "reject"))
	*clock = clock.Add(time.Minute)
	end := endSession(t, e, pid, ada.Session.ID)
	assert.Equal(t, 3, end.Summary.Decided)
	assert.Equal(t, 2, end.Summary.Kept)
	assert.Equal(t, 1, end.Summary.Rejected)
	assert.Zero(t, end.Summary.Changed)
	require.NotNil(t, end.Summary.Blocks)
	assert.Equal(t, 65, *end.Summary.Blocks, "Flowgraph and Dashpanel reach 40 and 25 blocks")
	assert.Nil(t, end.Summary.FindingsCleared, "no term rule was turned down")
	assert.Positive(t, end.Summary.Waiting)
	assert.Equal(t, "Growing", end.Summary.Next)
	require.NotNil(t, end.Session.EndedAt)

	// Ending again answers the same summary; another person cannot end it.
	assert.Equal(t, end.Summary, endSession(t, e, pid, ada.Session.ID).Summary)
	caller.userID = "member-b"
	assert.Equal(t, http.StatusNotFound, decideRoute(t, e, http.MethodPost, pid, "/context/sessions/"+ada.Session.ID+"/end", nil, nil))

	// Meanwhile a scan files a new candidate and finds Growing in more
	// blocks, and Bea decides a card in a session of her own.
	*clock = clock.Add(time.Hour)
	fileCandidateAt(t, tb, pid, "Newcomer", 7, *clock)
	_, err := termcandidate.File(t.Context(), tb, pid, "en", []termcandidate.Candidate{{
		Term: "Growing", Reach: termcandidate.Reach{Blocks: 30},
	}}, nil, termcandidate.ScanIdentity, *clock)
	require.NoError(t, err)
	bea := startSession(t, e, pid)
	require.NotNil(t, bea.Since)
	assert.Equal(t, "member-a", bea.Since.By)
	require.NotEmpty(t, bea.Cards)
	require.Equal(t, http.StatusOK, decideSessionCard(t, e, pid, bea.Cards[0], "keep"))

	caller.userID = "member-a"
	*clock = clock.Add(time.Minute)
	next := startSession(t, e, pid)
	require.NotNil(t, next.Since)
	assert.True(t, next.Since.At.Equal(end.Session.EndedAt.UTC()), "the baseline is the team's last ended session")
	assert.Empty(t, next.Since.By, "the caller ran the last session")
	assert.Equal(t, 1, next.Since.AddedTotal)
	require.Len(t, next.Since.Added, 1)
	assert.Equal(t, "Newcomer", next.Since.Added[0].Term)
	require.Len(t, next.Since.Grew, 1)
	assert.Equal(t, contextSinceTerm{Item: next.Since.Grew[0].Item, Term: "Growing", Blocks: 30, Before: 3}, next.Since.Grew[0])
	assert.Equal(t, 1, next.Since.DecidedByOthers, "Bea's decision")
}

// A keep in the reviewer's own wording is a change; a reject is a reject.
func TestContextSessions_Outcome(t *testing.T) {
	assert.Equal(t, platstore.SessionKept, sessionOutcome(contextDeckDecideRequest{Action: "keep"}))
	assert.Equal(t, platstore.SessionChanged, sessionOutcome(contextDeckDecideRequest{Action: "keep", Text: "arbeidsrom"}))
	assert.Equal(t, platstore.SessionRejected, sessionOutcome(contextDeckDecideRequest{Action: "reject"}))
}

// Turning down a term rule clears the source findings the last check counted
// for it, and the summary says how many.
func TestContextSessions_FindingsCleared(t *testing.T) {
	caller := &decideCaller{userID: "member-a", perms: decideAll}
	_, cs, _, _, srv := deckServer(t, caller)
	pid := "p"
	require.NoError(t, cs.CreateProject(t.Context(), &platstore.Project{ID: pid, Name: "p", DefaultSourceLanguage: "en"}))
	require.NoError(t, cs.PutItemCheckTally(t.Context(), platstore.ItemCheckTally{
		ProjectID: pid, ItemID: "i1", Findings: 5,
		Rules: []platstore.RuleTally{
			{Rule: "terms.vocabulary", Subject: "utilise", Count: 4},
			{Rule: "terms.vocabulary", Subject: "leverage", Count: 1},
		},
	}))
	sess := platstore.ContextSession{
		ProjectID: pid,
		Cards:     []platstore.ContextSessionCard{{Item: "op::r1", Kind: deckKindTermRule, Term: "Utilise"}},
		Decisions: []platstore.ContextSessionDecision{{Item: "op::r1", Outcome: platstore.SessionRejected}},
	}
	summary := summarizeSession(sess)
	srv.countFindingsCleared(t.Context(), sess, &summary)
	require.NotNil(t, summary.FindingsCleared)
	assert.Equal(t, 4, *summary.FindingsCleared)
	assert.Nil(t, summary.Blocks)
}
