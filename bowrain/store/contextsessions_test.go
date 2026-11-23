package store

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/contextop"
)

// Two sessions started on one project hold disjoint cards; a decision
// releases its card; an expired session releases every card it held and
// reads as ended at its expiry.
func TestContextSessions_HoldsAreDisjointAndExpire(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)
	at := time.Date(2026, 11, 19, 9, 0, 0, 0, time.UTC)
	concept := func(id string) contextop.ItemRef { return contextop.ItemRef{Kind: contextop.ItemConcept, ID: id} }
	items := []contextop.ItemRef{concept("c1"), concept("c2"), concept("c3"), concept("c4")}

	_, ok, err := s.LastEndedContextSession(ctx, p.ID, at)
	require.NoError(t, err)
	assert.False(t, ok, "a project with no session has no last one")

	a := platstore.ContextSession{
		ID: "s-a", ProjectID: p.ID, UserID: "ada", StartedAt: at, ExpiresAt: at.Add(30 * time.Minute),
		Reach: map[string]int{"c1": 12},
	}
	heldA, err := s.StartContextSession(ctx, a, items, 2)
	require.NoError(t, err)
	assert.Equal(t, items[:2], heldA)

	b := platstore.ContextSession{ID: "s-b", ProjectID: p.ID, UserID: "bea", StartedAt: at.Add(time.Minute), ExpiresAt: at.Add(31 * time.Minute)}
	heldB, err := s.StartContextSession(ctx, b, items, 10)
	require.NoError(t, err)
	assert.Equal(t, items[2:], heldB, "a card another live session holds is skipped")

	card := platstore.ContextSessionCard{Item: concept("c1"), Kind: "concept", Term: "workspace", Concept: "c1", Blocks: 12}
	require.NoError(t, s.SetContextSessionCards(ctx, "s-a", []platstore.ContextSessionCard{card}))
	holds, err := s.ContextSessionHolds(ctx, p.ID, at.Add(2*time.Minute))
	require.NoError(t, err)
	assert.Len(t, holds, 4)

	decided := platstore.ContextSessionDecision{Item: concept("c1"), Outcome: platstore.SessionKept, At: at.Add(3 * time.Minute)}
	require.NoError(t, s.RecordContextSessionDecision(ctx, "s-a", decided, at.Add(33*time.Minute)))
	got, ok, err := s.GetContextSession(ctx, "s-a")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, []platstore.ContextSessionCard{card}, got.Cards)
	assert.Equal(t, map[string]int{"c1": 12}, got.Reach)
	assert.Equal(t, []platstore.ContextSessionDecision{decided}, got.Decisions)
	assert.True(t, got.ExpiresAt.Equal(at.Add(33*time.Minute)), "a decision moves the expiry on")
	assert.True(t, got.Live(at.Add(32*time.Minute)))

	holds, err = s.ContextSessionHolds(ctx, p.ID, at.Add(4*time.Minute))
	require.NoError(t, err)
	assert.Len(t, holds, 3, "the decided card is released")

	since, err := s.ContextSessionsSince(ctx, p.ID, at)
	require.NoError(t, err)
	require.Len(t, since, 1)
	assert.Equal(t, "s-a", since[0].ID)

	// Both sessions expire; a third session takes every card.
	late := at.Add(40 * time.Minute)
	holds, err = s.ContextSessionHolds(ctx, p.ID, late)
	require.NoError(t, err)
	assert.Empty(t, holds, "no live session holds anything after both expired")
	c := platstore.ContextSession{ID: "s-c", ProjectID: p.ID, UserID: "cai", StartedAt: late, ExpiresAt: late.Add(30 * time.Minute)}
	heldC, err := s.StartContextSession(ctx, c, items, 10)
	require.NoError(t, err)
	assert.Equal(t, items, heldC, "expired sessions release their cards")

	last, ok, err := s.LastEndedContextSession(ctx, p.ID, late)
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "s-a", last.ID, "the session that expired last")
	assert.True(t, last.EndedAt.Equal(at.Add(33*time.Minute)))

	require.NoError(t, s.EndContextSession(ctx, "s-c", late.Add(time.Minute)))
	require.NoError(t, s.EndContextSession(ctx, "s-c", late.Add(time.Hour)), "ending twice keeps the first end")
	got, _, err = s.GetContextSession(ctx, "s-c")
	require.NoError(t, err)
	assert.True(t, got.EndedAt.Equal(late.Add(time.Minute)))
	holds, err = s.ContextSessionHolds(ctx, p.ID, late.Add(2*time.Minute))
	require.NoError(t, err)
	assert.Empty(t, holds)
	last, _, err = s.LastEndedContextSession(ctx, p.ID, late.Add(2*time.Minute))
	require.NoError(t, err)
	assert.Equal(t, "s-c", last.ID)
}

// Two references that one joined string would have merged stay two items: a
// part holding the old separators, and an empty part beside a non-empty one.
// Each is held once, and a part holding the unit separator reads back as
// written.
func TestContextSessions_ItemsAreKeyedByTheirParts(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)
	at := time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC)
	items := []contextop.ItemRef{
		{Kind: contextop.ItemOp, Profile: "a\x1fb", ID: "c"},
		{Kind: contextop.ItemOp, Profile: "a", ID: "b\x1fc"},
		{Kind: contextop.ItemOp, Profile: "", ID: "x"},
		{Kind: contextop.ItemOp, Profile: "x", ID: ""},
	}
	sess := platstore.ContextSession{
		ID: "s-parts", ProjectID: p.ID, UserID: "ada", StartedAt: at, ExpiresAt: at.Add(30 * time.Minute),
		Cards: []platstore.ContextSessionCard{{Item: items[0], Kind: "term_rule"}},
	}
	held, err := s.StartContextSession(ctx, sess, items, 10)
	require.NoError(t, err)
	assert.Equal(t, items, held, "each reference is an item of its own")

	again := platstore.ContextSession{ID: "s-again", ProjectID: p.ID, UserID: "bea", StartedAt: at, ExpiresAt: at.Add(30 * time.Minute)}
	heldAgain, err := s.StartContextSession(ctx, again, items, 10)
	require.NoError(t, err)
	assert.Empty(t, heldAgain, "each item is held once")

	holds, err := s.ContextSessionHolds(ctx, p.ID, at.Add(time.Minute))
	require.NoError(t, err)
	got := make([]contextop.ItemRef, 0, len(holds))
	for _, h := range holds {
		got = append(got, h.Item)
	}
	assert.ElementsMatch(t, items, got)

	decided := platstore.ContextSessionDecision{Item: items[1], Outcome: platstore.SessionRejected, At: at.Add(2 * time.Minute)}
	require.NoError(t, s.RecordContextSessionDecision(ctx, "s-parts", decided, at.Add(32*time.Minute)))
	read, ok, err := s.GetContextSession(ctx, "s-parts")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, items[0], read.Cards[0].Item)
	assert.Equal(t, []platstore.ContextSessionDecision{decided}, read.Decisions)
	holds, err = s.ContextSessionHolds(ctx, p.ID, at.Add(3*time.Minute))
	require.NoError(t, err)
	assert.Len(t, holds, 3, "the decision releases the one item it names")
}

// A session whose holds cannot be written does not start: the store fails the
// call, and neither the session nor any of its holds is left behind.
// PostgreSQL refuses a NUL byte in text, which the route refuses before it
// gets here; the store holds the same line for any other writer.
func TestContextSessions_AHoldThatCannotBeWrittenStartsNoSession(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)
	at := time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC)
	items := []contextop.ItemRef{
		{Kind: contextop.ItemConcept, ID: "c1"},
		{Kind: contextop.ItemConcept, ID: "c\x002"},
	}
	sess := platstore.ContextSession{ID: "s-nul", ProjectID: p.ID, UserID: "ada", StartedAt: at, ExpiresAt: at.Add(30 * time.Minute)}
	_, err := s.StartContextSession(ctx, sess, items, 10)
	require.Error(t, err)

	_, ok, err := s.GetContextSession(ctx, "s-nul")
	require.NoError(t, err)
	assert.False(t, ok, "the session did not start")
	holds, err := s.ContextSessionHolds(ctx, p.ID, at.Add(time.Minute))
	require.NoError(t, err)
	assert.Empty(t, holds, "and holds nothing, the card it could hold included")
}
