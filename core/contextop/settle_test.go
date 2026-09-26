package contextop_test

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/workspace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var mergeTool = contextop.Actor{Kind: contextop.ActorTool, Name: "settle-merged"}

func observe(t *testing.T, l *contextop.Ledger, actor contextop.Actor, term, use string) contextop.Record {
	t.Helper()
	r, err := l.Append(t.Context(), contextop.Record{Project: "prj", Actor: actor, Kind: contextop.KindObserve, Subject: termRule(term, use, false)})
	require.NoError(t, err)
	return r
}

func correct(t *testing.T, l *contextop.Ledger, actor contextop.Actor, from, to string) contextop.Record {
	t.Helper()
	r, err := l.Append(t.Context(), contextop.Record{Project: "prj", Actor: actor, Kind: contextop.KindCorrect,
		Correction: &contextop.Correction{From: from, To: to}})
	require.NoError(t, err)
	return r
}

func signal(t *testing.T, l *contextop.Ledger, target string, s contextop.Signal) contextop.Record {
	t.Helper()
	r, err := l.Append(t.Context(), contextop.Record{Project: "prj", Actor: mergeTool, Kind: contextop.KindSignal, Target: target, Signal: &s})
	require.NoError(t, err)
	return r
}

func settleAll(t *testing.T, l *contextop.Ledger) []contextop.Record {
	t.Helper()
	out, err := l.Settle(t.Context(), contextop.Filter{})
	require.NoError(t, err)
	return out
}

func establishOps(t *testing.T, l *contextop.Ledger) []contextop.Record {
	t.Helper()
	out, err := l.Records(t.Context(), contextop.Filter{Kinds: []contextop.Kind{contextop.KindEstablish}})
	require.NoError(t, err)
	return out
}

func TestSettle_AMergeEstablishesASuggestion(t *testing.T) {
	l := contextop.NewLedger(openWorkspace(t), contextop.PersonDecides)
	s := observe(t, l, agent("claude", "s1"), "Quick cast", "Quickcast")
	assert.Empty(t, settleAll(t, l), "a suggestion with no person's signal settles nothing")

	m := signal(t, l, s.ID, contextop.Signal{Source: contextop.SignalMerge, Commit: "abc1234def", PR: 412, Preferred: 2})
	settled := settleAll(t, l)
	require.Len(t, settled, 1)
	assert.Equal(t, contextop.StatusEstablished, settled[0].Status)
	assert.True(t, settled[0].Established)
	assert.Equal(t, "merged in #412", settled[0].Standing.Describe())

	ops := establishOps(t, l)
	require.Len(t, ops, 1)
	assert.Equal(t, []string{m.ID}, ops[0].Because, "the establishment names the evidence it rested on")
	assert.Equal(t, contextop.SettleActor, ops[0].Actor)

	assert.Empty(t, settleAll(t, l), "settling again records nothing")
	again := signal(t, l, s.ID, contextop.Signal{Source: contextop.SignalMerge, Commit: "abc1234def", PR: 412, Preferred: 2})
	assert.Equal(t, m.ID, again.ID, "the same merge recorded twice is one operation")
}

func TestSettle_ACorrectionTowardTheRuleEstablishesIt(t *testing.T) {
	l := contextop.NewLedger(openWorkspace(t), contextop.PersonDecides)
	s := observe(t, l, agent("claude", "s1"), "utilise", "use")
	c := correct(t, l, agent("claude", "s2"), "utilise", "use")
	settled := settleAll(t, l)
	require.Len(t, settled, 1)
	assert.Equal(t, s.ID, settled[0].ID)
	assert.Equal(t, 1, settled[0].Standing.Corrections)
	assert.Equal(t, []string{c.ID}, establishOps(t, l)[0].Because)
}

func TestSettle_StandingAloneSettlesNothing(t *testing.T) {
	l := contextop.NewLedger(openWorkspace(t), contextop.PersonDecides)
	a := observe(t, l, agent("claude", "s1"), "utilise", "use")
	observe(t, l, agent("claude", "s2"), "utilise", "use")
	signal(t, l, a.ID, contextop.Signal{Source: contextop.SignalUsage, Preferred: 14, Rejected: 1, Within: "docs/"})
	assert.Empty(t, settleAll(t, l))

	r, err := l.Get(t.Context(), a.ID)
	require.NoError(t, err)
	assert.Equal(t, contextop.StatusSuggested, r.Status)
	assert.Equal(t, "seen in 2 sessions · 14 of 15 uses in docs/", r.Standing.Describe())
}

func TestSettle_ASuggestionsOwnCorrectionIsNotEvidenceForIt(t *testing.T) {
	l := contextop.NewLedger(openWorkspace(t), contextop.PersonDecides)
	_, err := l.Append(t.Context(), contextop.Record{Project: "prj", Actor: person("asgeir"), Kind: contextop.KindCorrect,
		Correction: &contextop.Correction{From: "utilise", To: "use"}, Subject: termRule("utilise", "use", false)})
	require.NoError(t, err)
	assert.Empty(t, settleAll(t, l))
}

func TestSettle_EvidenceAgainstLeavesItContested(t *testing.T) {
	l := contextop.NewLedger(openWorkspace(t), contextop.PersonDecides)
	s := observe(t, l, agent("claude", "s1"), "utilise", "use")
	signal(t, l, s.ID, contextop.Signal{Source: contextop.SignalMerge, Commit: "c1", Preferred: 1})
	away := correct(t, l, person("asgeir"), "use", "utilise")
	assert.Empty(t, settleAll(t, l))
	st, by := status(t, l, s.ID)
	assert.Equal(t, contextop.StatusContested, st)
	assert.Equal(t, []string{away.ID}, by)
}

func TestSettle_ContentMovingToTheRejectedFormCountsAgainst(t *testing.T) {
	l := contextop.NewLedger(openWorkspace(t), contextop.PersonDecides)
	s := observe(t, l, agent("claude", "s1"), "utilise", "use")
	signal(t, l, s.ID, contextop.Signal{Source: contextop.SignalUsage, Preferred: 9, Rejected: 1})
	signal(t, l, s.ID, contextop.Signal{Source: contextop.SignalUsage, Preferred: 7, Rejected: 3})
	signal(t, l, s.ID, contextop.Signal{Source: contextop.SignalMerge, Commit: "c1", Preferred: 1})
	assert.Empty(t, settleAll(t, l))
	st, _ := status(t, l, s.ID)
	assert.Equal(t, contextop.StatusContested, st)
}

func TestSettle_ADropIsThePersonsAnswer(t *testing.T) {
	l := contextop.NewLedger(openWorkspace(t), contextop.PersonDecides)
	s := observe(t, l, agent("claude", "s1"), "utilise", "use")
	later := observe(t, l, agent("claude", "s2"), "utilise", "use")
	_, err := l.Append(t.Context(), contextop.Record{Project: "prj", Actor: person("asgeir"), Kind: contextop.KindDrop, Target: s.ID})
	require.NoError(t, err)
	correct(t, l, agent("claude", "s3"), "utilise", "use")
	assert.Empty(t, settleAll(t, l), "what was recorded before the drop does not settle")

	fresh := observe(t, l, agent("claude", "s4"), "utilise", "use")
	correct(t, l, agent("claude", "s5"), "utilise", "use")
	settled := settleAll(t, l)
	require.Len(t, settled, 1)
	assert.Equal(t, fresh.ID, settled[0].ID)
	st, _ := status(t, l, later.ID)
	assert.Equal(t, contextop.StatusSuggested, st)
}

func TestSettle_ARivalRuleIsOpen(t *testing.T) {
	l := contextop.NewLedger(openWorkspace(t), contextop.PersonDecides)
	a := observe(t, l, agent("claude", "s1"), "utilise", "use")
	b := observe(t, l, agent("claude", "s2"), "utilise", "employ")
	signal(t, l, a.ID, contextop.Signal{Source: contextop.SignalMerge, Commit: "c1", Preferred: 1})
	assert.Empty(t, settleAll(t, l))
	st, by := status(t, l, a.ID)
	assert.Equal(t, contextop.StatusContested, st)
	assert.Equal(t, []string{b.ID}, by)
}

func TestSettle_OnlyAToolRecordsEvidence(t *testing.T) {
	l := contextop.NewLedger(openWorkspace(t), contextop.PersonDecides)
	s := observe(t, l, agent("claude", "s1"), "utilise", "use")
	_, err := l.Append(t.Context(), contextop.Record{Project: "prj", Actor: agent("claude", "s1"), Kind: contextop.KindSignal,
		Target: s.ID, Signal: &contextop.Signal{Source: contextop.SignalMerge, Commit: "c1", Preferred: 1}})
	require.ErrorIs(t, err, contextop.ErrRefused)
	_, err = l.Append(t.Context(), contextop.Record{Project: "prj", Actor: agent("claude", "s1"), Kind: contextop.KindEstablish, Target: s.ID})
	require.ErrorIs(t, err, contextop.ErrRefused)
}

// TestSettle_TheAnswerIsTheSameWhateverTheMergeOrder settles two machines'
// logs, merges them in both directions and settles again: both end on the same
// statuses and the same rules established.
func TestSettle_TheAnswerIsTheSameWhateverTheMergeOrder(t *testing.T) {
	ctx := t.Context()
	origin := openWorkspace(t)
	lo := contextop.NewLedger(origin, contextop.PersonDecides)
	contested := observe(t, lo, agent("claude", "s1"), "utilise", "use")
	settles := observe(t, lo, agent("claude", "s1"), "Quick cast", "Quickcast")

	a, b := openWorkspace(t), openWorkspace(t)
	copyLog(t, origin, a)
	copyLog(t, origin, b)
	la, lb := contextop.NewLedger(a, contextop.PersonDecides), contextop.NewLedger(b, contextop.PersonDecides)

	// Machine a sees both rules merged and settles them.
	signal(t, la, contested.ID, contextop.Signal{Source: contextop.SignalMerge, Commit: "c1", Preferred: 1})
	signal(t, la, settles.ID, contextop.Signal{Source: contextop.SignalMerge, Commit: "c1", Preferred: 1})
	require.Len(t, settleAll(t, la), 2)
	// Machine b sees a person correct one of them away, and a correction
	// toward the other.
	correct(t, lb, person("asgeir"), "use", "utilise")
	correct(t, lb, agent("claude", "s2"), "Quick cast", "Quickcast")
	require.Len(t, settleAll(t, lb), 1)

	copyLog(t, a, b)
	copyLog(t, b, a)
	settleAll(t, la)
	settleAll(t, lb)

	for _, l := range []*contextop.Ledger{la, lb} {
		st, _ := status(t, l, contested.ID)
		assert.Equal(t, contextop.StatusContested, st)
		st, _ = status(t, l, settles.ID)
		assert.Equal(t, contextop.StatusEstablished, st)
	}
	ea, err := a.Select(ctx, workspace.OpQuery{KindPrefix: contextop.OpKindPrefix})
	require.NoError(t, err)
	eb, err := b.Select(ctx, workspace.OpQuery{KindPrefix: contextop.OpKindPrefix})
	require.NoError(t, err)
	assert.ElementsMatch(t, ids(ea), ids(eb), "both logs hold the same operations")
}

func copyLog(t *testing.T, from, to *workspace.Workspace) {
	t.Helper()
	ops, err := from.Ops(context.Background(), 0, 0)
	require.NoError(t, err)
	_, err = to.Record(context.Background(), ops...)
	require.NoError(t, err)
}

func ids(ops []workspace.Op) []string {
	out := make([]string, len(ops))
	for i, op := range ops {
		out[i] = op.ID
	}
	return out
}
