package contextop_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPersonDecides(t *testing.T) {
	agentRecord := contextop.Record{
		Actor:  agent("claude", "s1"),
		Kind:   contextop.KindObserve,
		Status: contextop.StatusSuggested,
	}
	establishedRecord := contextop.Record{
		Actor:       agent("claude", "s1"),
		Kind:        contextop.KindObserve,
		Status:      contextop.StatusEstablished,
		Established: true,
	}
	personRecord := contextop.Record{Actor: person("asgeir"), Kind: contextop.KindObserve, Status: contextop.StatusSuggested}

	tests := []struct {
		name       string
		transition contextop.Transition
		refused    bool
	}{
		{
			name:       "a person keeps",
			transition: contextop.Transition{Actor: person("asgeir"), Kind: contextop.KindKeep, Targeted: true, Target: agentRecord},
		},
		{
			name:       "a person drops",
			transition: contextop.Transition{Actor: person("asgeir"), Kind: contextop.KindDrop, Targeted: true, Target: agentRecord},
		},
		{
			name:       "a person widens",
			transition: contextop.Transition{Actor: person("asgeir"), Kind: contextop.KindWiden, Targeted: true, Target: establishedRecord},
		},
		{
			name:       "a person imports",
			transition: contextop.Transition{Actor: person("asgeir"), Kind: contextop.KindImport, Subject: contextop.SubjectNote},
		},
		{
			name:       "a person withdraws their own suggestion",
			transition: contextop.Transition{Actor: person("asgeir"), Kind: contextop.KindWithdraw, Targeted: true, Target: personRecord},
		},
		{
			name:       "an agent observes",
			transition: contextop.Transition{Actor: agent("claude", "s1"), Kind: contextop.KindObserve, Subject: contextop.SubjectNote},
		},
		{
			name:       "an agent observes a term",
			transition: contextop.Transition{Actor: agent("claude", "s1"), Kind: contextop.KindObserve, Subject: contextop.SubjectTerm},
		},
		{
			name:       "an agent records a correction",
			transition: contextop.Transition{Actor: agent("claude", "s1"), Kind: contextop.KindCorrect},
		},
		{
			name:       "a tool observes",
			transition: contextop.Transition{Actor: contextop.Actor{Kind: contextop.ActorTool, Name: "converge"}, Kind: contextop.KindObserve},
		},
		{
			name:       "an agent withdraws its own suggestion",
			transition: contextop.Transition{Actor: agent("claude", "s1"), Kind: contextop.KindWithdraw, Targeted: true, Target: agentRecord},
		},
		{
			name:       "an agent reverts its own suggestion",
			transition: contextop.Transition{Actor: agent("claude", "s1"), Kind: contextop.KindRevert, Targeted: true, Target: agentRecord},
		},
		{
			name:       "an agent drops its own suggestion",
			transition: contextop.Transition{Actor: agent("claude", "s1"), Kind: contextop.KindDrop, Targeted: true, Target: agentRecord},
			refused:    true,
		},
		{
			name:       "an agent keeps",
			transition: contextop.Transition{Actor: agent("claude", "s1"), Kind: contextop.KindKeep, Targeted: true, Target: agentRecord},
			refused:    true,
		},
		{
			name:       "a tool keeps",
			transition: contextop.Transition{Actor: contextop.Actor{Kind: contextop.ActorTool, Name: "converge"}, Kind: contextop.KindKeep, Targeted: true, Target: agentRecord},
			refused:    true,
		},
		{
			name:       "an agent imports",
			transition: contextop.Transition{Actor: agent("claude", "s1"), Kind: contextop.KindImport, Subject: contextop.SubjectNote},
			refused:    true,
		},
		{
			name:       "an agent writes a rule directly",
			transition: contextop.Transition{Actor: agent("claude", "s1"), Kind: contextop.KindEdit, Subject: contextop.SubjectTerm},
			refused:    true,
		},
		{
			name:       "an agent widens",
			transition: contextop.Transition{Actor: agent("claude", "s1"), Kind: contextop.KindWiden, Targeted: true, Target: establishedRecord},
			refused:    true,
		},
		{
			name:       "an agent observes straight to the workspace",
			transition: contextop.Transition{Actor: agent("claude", "s1"), Kind: contextop.KindObserve, Widening: true},
			refused:    true,
		},
		{
			name:       "an agent reverts an established rule",
			transition: contextop.Transition{Actor: agent("claude", "s1"), Kind: contextop.KindRevert, Targeted: true, Target: establishedRecord},
			refused:    true,
		},
		{
			name:       "an agent withdraws an established rule",
			transition: contextop.Transition{Actor: agent("claude", "s1"), Kind: contextop.KindWithdraw, Targeted: true, Target: establishedRecord},
			refused:    true,
		},
		{
			name:       "a person withdraws another actor's suggestion",
			transition: contextop.Transition{Actor: person("asgeir"), Kind: contextop.KindWithdraw, Targeted: true, Target: agentRecord},
			refused:    true,
		},
		{
			name: "an agent withdraws another session's suggestion",
			transition: contextop.Transition{
				Actor: agent("claude", "s2"), Kind: contextop.KindWithdraw, Targeted: true, Target: agentRecord,
			},
			refused: true,
		},
		{
			name:       "an agent withdraws a person's suggestion",
			transition: contextop.Transition{Actor: agent("claude", "s1"), Kind: contextop.KindWithdraw, Targeted: true, Target: personRecord},
			refused:    true,
		},
		{
			name:       "an agent reverts a whole session",
			transition: contextop.Transition{Actor: agent("claude", "s1"), Kind: contextop.KindRevert},
			refused:    true,
		},
		{
			name:       "an unknown kind from an agent",
			transition: contextop.Transition{Actor: agent("claude", "s1"), Kind: "ponder"},
			refused:    true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := contextop.PersonDecides(tt.transition)
			if tt.refused {
				require.Error(t, err)
				require.ErrorIs(t, err, contextop.ErrRefused)
				assert.Contains(t, err.Error(), string(tt.transition.Kind))
				return
			}
			assert.NoError(t, err)
		})
	}
}

// TestLedger_PolicyRefusesAnAgentConfirmation drives the refusal through the
// ledger, because a policy nothing consults is a policy that is not in force.
func TestLedger_PolicyRefusesAnAgentConfirmation(t *testing.T) {
	ctx := t.Context()
	ledger := contextop.NewLedger(openWorkspace(t), nil)

	proposed, err := ledger.Append(ctx, contextop.Record{
		Project: "prj_docs", Actor: agent("claude", "s1"),
		Kind: contextop.KindObserve, Subject: termRule("utilise", "use", false),
	})
	require.NoError(t, err)

	_, err = ledger.Append(ctx, contextop.Record{
		Project: "prj_docs", Actor: agent("claude", "s1"),
		Kind: contextop.KindKeep, Target: proposed.ID,
	})
	require.ErrorIs(t, err, contextop.ErrRefused)

	still, err := ledger.Get(ctx, proposed.ID)
	require.NoError(t, err)
	assert.Equal(t, contextop.StatusSuggested, still.Status, "a refused confirmation records nothing")

	_, err = ledger.Append(ctx, contextop.Record{
		Project: "prj_docs", Actor: person("asgeir"),
		Kind: contextop.KindKeep, Target: proposed.ID,
	})
	require.NoError(t, err)
	confirmed, err := ledger.Get(ctx, proposed.ID)
	require.NoError(t, err)
	assert.Equal(t, contextop.StatusEstablished, confirmed.Status, "a person confirms the same rule")
}

// TestLedger_DefaultPolicyIsPersonDecides pins the default, because a nil
// policy that allowed everything would be an agent with a person's rights.
func TestLedger_DefaultPolicyIsPersonDecides(t *testing.T) {
	ctx := t.Context()
	ledger := contextop.NewLedger(openWorkspace(t), nil)
	observed, err := ledger.Append(ctx, contextop.Record{
		Project: "prj_docs", Actor: agent("claude", "s1"),
		Kind: contextop.KindObserve, Subject: termRule("utilise", "use", false),
	})
	require.NoError(t, err)
	_, err = ledger.Append(ctx, contextop.Record{
		Project: "prj_docs", Actor: agent("claude", "s1"),
		Kind: contextop.KindWiden, Target: observed.ID,
	})
	assert.ErrorIs(t, err, contextop.ErrRefused)
}
