package server

import (
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/contextop"
)

// A context session on PostgreSQL, the only database production runs,
// through the real handlers: it starts and takes its holds, a decision
// releases the one it names and is recorded on the session, and ending the
// session releases the rest.
func TestContextSessions_RunsOnPostgres(t *testing.T) {
	at := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	sessionClock(t, at)
	caller := &decideCaller{userID: "member-a", perms: decideAll}
	e, cs, tb, _ := deckPostgresServer(t, caller)
	pid := seedDecideParity(t, e, cs)
	for i := 1; i <= 3; i++ {
		fileCandidateAt(t, tb, pid, fmt.Sprintf("Term%02d", i), i*10, at.Add(-time.Hour))
	}

	ada := startSession(t, e, pid)
	require.NotEmpty(t, ada.Cards)
	holds, err := cs.ContextSessionHolds(t.Context(), pid, at)
	require.NoError(t, err)
	require.Len(t, holds, len(ada.Cards), "the session holds each card it dealt")
	for _, h := range holds {
		assert.Equal(t, ada.Session.ID, h.SessionID)
	}

	decided := ada.Cards[0]
	require.Equal(t, http.StatusOK, decideSessionCard(t, e, pid, decided, "keep"))
	holds, err = cs.ContextSessionHolds(t.Context(), pid, at)
	require.NoError(t, err)
	assert.Len(t, holds, len(ada.Cards)-1, "the decision releases the card it names")
	for _, h := range holds {
		assert.NotEqual(t, decided.Item, h.Item)
	}
	sess, ok, err := cs.GetContextSession(t.Context(), ada.Session.ID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Len(t, sess.Decisions, 1)
	assert.Equal(t, decided.Item, sess.Decisions[0].Item)
	assert.Equal(t, platstore.SessionKept, sess.Decisions[0].Outcome)
	assert.Len(t, sess.Cards, len(ada.Cards), "the cards were recorded with the session")

	end := endSession(t, e, pid, ada.Session.ID)
	assert.Equal(t, 1, end.Summary.Kept)
	holds, err = cs.ContextSessionHolds(t.Context(), pid, at)
	require.NoError(t, err)
	assert.Empty(t, holds, "an ended session holds nothing")
}

// Two people start sessions at the same moment. Both rank the deck the same
// way, so they race for the same cards, the widest first: each card is held
// by exactly one of the two sessions, and every card held is dealt to the
// session that holds it.
func TestContextSessions_TwoSessionsRaceForOneItemOnPostgres(t *testing.T) {
	at := time.Date(2026, 11, 23, 10, 0, 0, 0, time.UTC)
	sessionClock(t, at)
	ada := &decideCaller{userID: "member-a", perms: decideAll}
	e, cs, tb, srv := deckPostgresServer(t, ada)
	pid := seedDecideParity(t, e, cs)
	fileCandidateAt(t, tb, pid, "Widest", 500, at.Add(-time.Hour))

	bea := &decideCaller{userID: "member-b", perms: decideAll}
	callers := []*decideCaller{ada, bea}
	results := make([]contextSessionResponse, len(callers))
	for round := range 5 {
		var wg sync.WaitGroup
		for i, caller := range callers {
			wg.Go(func() {
				results[i] = startSession(t, deckEcho(srv, caller), pid)
			})
		}
		wg.Wait()

		dealt := map[contextop.ItemRef]string{}
		for _, r := range results {
			for _, c := range r.Cards {
				prev, twice := dealt[c.Item]
				assert.False(t, twice, "round %d: %v dealt to %s and %s", round, c.Item, prev, r.Session.ID)
				dealt[c.Item] = r.Session.ID
			}
		}
		holds, err := cs.ContextSessionHolds(t.Context(), pid, at)
		require.NoError(t, err)
		require.Len(t, holds, len(dealt), "round %d: every card held is dealt", round)
		for _, h := range holds {
			assert.Equal(t, dealt[h.Item], h.SessionID, "round %d: held by the session that dealt it", round)
		}
		widest := 0
		for _, r := range results {
			for _, c := range r.Cards {
				if len(c.Terms) > 0 && c.Terms[0].Text == "Widest" {
					widest++
				}
			}
		}
		assert.Equal(t, 1, widest, "round %d: the card both raced for goes to exactly one session", round)
		for i, r := range results {
			endSession(t, deckEcho(srv, callers[i]), pid, r.Session.ID)
		}
	}
}
