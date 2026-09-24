package contextop_test

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// status reads one operation's folded status and who contests it.
func status(t *testing.T, ledger *contextop.Ledger, id string) (contextop.Status, []string) {
	t.Helper()
	r, err := ledger.Get(context.Background(), id)
	require.NoError(t, err)
	return r.Status, r.ContestedBy
}

func TestContest_TwoSuggestionsDisagree(t *testing.T) {
	ctx := context.Background()
	ledger := contextop.NewLedger(openWorkspace(t), contextop.Allow)

	a, err := ledger.Append(ctx, contextop.Record{Project: "prj", Actor: agent("claude", "s1"), Kind: contextop.KindObserve, Subject: termRule("utilise", "use", false)})
	require.NoError(t, err)
	b, err := ledger.Append(ctx, contextop.Record{Project: "prj", Actor: agent("claude", "s2"), Kind: contextop.KindObserve, Subject: termRule("utilise", "employ", false)})
	require.NoError(t, err)
	other, err := ledger.Append(ctx, contextop.Record{Project: "elsewhere", Actor: agent("claude", "s3"), Kind: contextop.KindObserve, Subject: termRule("utilise", "apply", false)})
	require.NoError(t, err)

	st, by := status(t, ledger, a.ID)
	assert.Equal(t, contextop.StatusContested, st)
	assert.Equal(t, []string{b.ID}, by)
	st, by = status(t, ledger, b.ID)
	assert.Equal(t, contextop.StatusContested, st)
	assert.Equal(t, []string{a.ID}, by)
	st, _ = status(t, ledger, other.ID)
	assert.Equal(t, contextop.StatusSuggested, st, "a rule in another project meets neither")

	_, err = ledger.Append(ctx, contextop.Record{Project: "prj", Actor: person("asgeir"), Kind: contextop.KindDrop, Target: b.ID})
	require.NoError(t, err)
	st, by = status(t, ledger, a.ID)
	assert.Equal(t, contextop.StatusSuggested, st, "dropping one side settles the other")
	assert.Empty(t, by)
}

func TestContest_SuggestionAgainstAnEstablishedRule(t *testing.T) {
	ctx := context.Background()
	ledger := contextop.NewLedger(openWorkspace(t), contextop.Allow)

	rule, err := ledger.Append(ctx, contextop.Record{Project: "prj", Actor: person("asgeir"), Kind: contextop.KindEdit, Subject: termRule("sign in", "log in", false)})
	require.NoError(t, err)
	// The suggestion avoids the form the rule says to use.
	suggestion, err := ledger.Append(ctx, contextop.Record{Project: "prj", Actor: agent("claude", "s1"), Kind: contextop.KindObserve, Subject: termRule("log in", "sign in", false)})
	require.NoError(t, err)

	st, _ := status(t, ledger, rule.ID)
	assert.Equal(t, contextop.StatusEstablished, st, "the rule stays in force")
	st, by := status(t, ledger, suggestion.ID)
	assert.Equal(t, contextop.StatusContested, st)
	assert.Equal(t, []string{rule.ID}, by)
}

func TestContest_APersonsCorrectionReversesAnEstablishedRule(t *testing.T) {
	ctx := context.Background()
	ledger := contextop.NewLedger(openWorkspace(t), contextop.Allow)

	rule, err := ledger.Append(ctx, contextop.Record{Project: "prj", Actor: agent("claude", "s1"), Kind: contextop.KindObserve, Subject: termRule("sign in", "log in", false)})
	require.NoError(t, err)
	_, err = ledger.Append(ctx, contextop.Record{Project: "prj", Actor: person("asgeir"), Kind: contextop.KindKeep, Target: rule.ID})
	require.NoError(t, err)

	// An agent's correction is only a suggestion of its own.
	agentFix, err := ledger.Append(ctx, contextop.Record{Project: "prj", Actor: agent("claude", "s1"), Kind: contextop.KindCorrect,
		Correction: &contextop.Correction{From: "log in", To: "sign in"}})
	require.NoError(t, err)
	st, _ := status(t, ledger, rule.ID)
	assert.Equal(t, contextop.StatusEstablished, st)

	fix, err := ledger.Append(ctx, contextop.Record{Project: "prj", Actor: person("asgeir"), Kind: contextop.KindCorrect,
		Correction: &contextop.Correction{From: "log in", To: "sign in"}})
	require.NoError(t, err)
	st, by := status(t, ledger, rule.ID)
	assert.Equal(t, contextop.StatusContested, st, "a person's own edit contests the rule it reverses")
	assert.Equal(t, []string{fix.ID}, by)
	assert.NotContains(t, by, agentFix.ID)

	held, err := ledger.Get(ctx, rule.ID)
	require.NoError(t, err)
	assert.True(t, held.Established, "a contested rule was still established by a person")

	// Keeping the rule again answers the correction.
	_, err = ledger.Append(ctx, contextop.Record{Project: "prj", Actor: person("asgeir"), Kind: contextop.KindKeep, Target: rule.ID})
	require.NoError(t, err)
	st, _ = status(t, ledger, rule.ID)
	assert.Equal(t, contextop.StatusEstablished, st)
}

func TestResolve_ContestedRulesAdvise(t *testing.T) {
	records := []contextop.Record{
		{ID: "1", Seq: 1, Project: "prj", Kind: contextop.KindObserve, Subject: termRule("utilise", "use", false), Status: contextop.StatusContested},
		{ID: "2", Seq: 2, Project: "prj", Kind: contextop.KindObserve, Subject: termRule("leverage", "use", false), Status: contextop.StatusEstablished, Established: true},
	}
	res := contextop.Resolve(records, nil, contextop.ResolveRequest{Project: "prj"})
	require.Len(t, res.Advisory, 1)
	assert.Equal(t, "utilise", res.Advisory[0].Term)
	assert.Empty(t, res.Binding, "a project's established rules live in its own stores")
}
