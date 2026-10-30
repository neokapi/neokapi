package contextop_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/workspace"
)

func checkRun(project, session, turn string, failing int) contextop.Record {
	run := &contextop.CheckRun{Hook: "stop", Turn: turn, Verdict: "passed", Clean: failing == 0, Failing: failing}
	if failing > 0 {
		run.Verdict = "failed"
	}
	return contextop.Record{
		Project: workspace.ProjectKey("prj_" + project),
		Actor:   agent("claude-code", session),
		Kind:    contextop.KindCheck,
		Check:   run,
	}
}

// TestCheckRun_OneOperationPerSessionAndTurn: the policy in force lets an
// agent record a check run, and the hook running twice for one turn records
// one operation.
func TestCheckRun_OneOperationPerSessionAndTurn(t *testing.T) {
	ctx := t.Context()
	ledger := contextop.NewLedger(openWorkspace(t), nil)

	first, err := ledger.Append(ctx, checkRun("docs", "s1", "turn-1", 1))
	require.NoError(t, err, "any actor may record a check run")
	again, err := ledger.Append(ctx, checkRun("docs", "s1", "turn-1", 1))
	require.NoError(t, err)
	assert.Equal(t, first.ID, again.ID, "one session and turn is one operation")

	other, err := ledger.Append(ctx, checkRun("docs", "s2", "turn-1", 1))
	require.NoError(t, err)
	assert.NotEqual(t, first.ID, other.ID, "another session's turn is another run")
	next, err := ledger.Append(ctx, checkRun("docs", "s1", "turn-2", 0))
	require.NoError(t, err)
	assert.NotEqual(t, first.ID, next.ID)

	read, err := ledger.Get(ctx, first.ID)
	require.NoError(t, err)
	require.NotNil(t, read.Check)
	assert.Equal(t, 1, read.Check.Failing)
	assert.Equal(t, "turn-1", read.Check.Turn)

	runs, err := ledger.Records(ctx, contextop.Filter{Kinds: []contextop.Kind{contextop.KindCheck}})
	require.NoError(t, err)
	assert.Len(t, runs, 3)
}

// TestCheckRun_ActsOnNothing: a check run is neither a subject nor a decision.
// It stays out of what the project has learned, and its instant is not one a
// status history folds at.
func TestCheckRun_ActsOnNothing(t *testing.T) {
	ctx := t.Context()
	ledger := contextop.NewLedger(openWorkspace(t), nil)
	rule, err := ledger.Append(ctx, contextop.Record{
		Project: "prj_docs", Actor: agent("claude-code", "s1"), Kind: contextop.KindObserve,
		Subject: termRule("Log in", "Sign in", false),
	})
	require.NoError(t, err)
	_, err = ledger.Append(ctx, checkRun("docs", "s1", "turn-1", 1))
	require.NoError(t, err)

	subjects, err := ledger.Records(ctx, contextop.Filter{Subjects: true})
	require.NoError(t, err)
	require.Len(t, subjects, 1)
	assert.Equal(t, contextop.StatusSuggested, subjects[0].Status, "a check run changes no rule's status")

	histories, complete, err := ledger.Histories(ctx, contextop.Filter{Project: "prj_docs", Subjects: true}, 0)
	require.NoError(t, err)
	assert.True(t, complete)
	assert.Equal(t, []contextop.Status{contextop.StatusSuggested}, statusesOf(histories[rule.ID]))

	// One instant a history folds at: the observation's. The check run's is
	// not one, so a limit of one still reads every history whole.
	_, complete, err = ledger.Histories(ctx, contextop.Filter{Project: "prj_docs", Subjects: true}, 1)
	require.NoError(t, err)
	assert.True(t, complete, "a check run's instant is not folded at")
}

func statusesOf(h []contextop.StatusAt) []contextop.Status {
	out := make([]contextop.Status, len(h))
	for i, s := range h {
		out[i] = s.Status
	}
	return out
}
