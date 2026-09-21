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
		Kind:   contextop.KindPropose,
		Status: contextop.StatusCandidate,
	}
	confirmedRecord := contextop.Record{
		Actor:  agent("claude", "s1"),
		Kind:   contextop.KindPropose,
		Status: contextop.StatusConfirmed,
	}

	tests := []struct {
		name       string
		transition contextop.Transition
		refused    bool
	}{
		{
			name:       "a person confirms",
			transition: contextop.Transition{Actor: person("asgeir"), Kind: contextop.KindConfirm, Targeted: true, Target: agentRecord},
		},
		{
			name:       "a person widens",
			transition: contextop.Transition{Actor: person("asgeir"), Kind: contextop.KindWiden, Targeted: true, Target: confirmedRecord},
		},
		{
			name:       "an agent observes",
			transition: contextop.Transition{Actor: agent("claude", "s1"), Kind: contextop.KindObserve, Subject: contextop.SubjectNote},
		},
		{
			name:       "an agent proposes",
			transition: contextop.Transition{Actor: agent("claude", "s1"), Kind: contextop.KindPropose, Subject: contextop.SubjectTerm},
		},
		{
			name:       "an agent records a correction",
			transition: contextop.Transition{Actor: agent("claude", "s1"), Kind: contextop.KindCorrect},
		},
		{
			name:       "a tool proposes",
			transition: contextop.Transition{Actor: contextop.Actor{Kind: contextop.ActorTool, Name: "converge"}, Kind: contextop.KindPropose},
		},
		{
			name:       "an agent discards its own candidate",
			transition: contextop.Transition{Actor: agent("claude", "s1"), Kind: contextop.KindDiscard, Targeted: true, Target: agentRecord},
		},
		{
			name:       "an agent reverts its own candidate",
			transition: contextop.Transition{Actor: agent("claude", "s1"), Kind: contextop.KindRevert, Targeted: true, Target: agentRecord},
		},
		{
			name:       "an agent confirms",
			transition: contextop.Transition{Actor: agent("claude", "s1"), Kind: contextop.KindConfirm, Targeted: true, Target: agentRecord},
			refused:    true,
		},
		{
			name:       "a tool confirms",
			transition: contextop.Transition{Actor: contextop.Actor{Kind: contextop.ActorTool, Name: "converge"}, Kind: contextop.KindConfirm, Targeted: true, Target: agentRecord},
			refused:    true,
		},
		{
			name:       "an agent widens",
			transition: contextop.Transition{Actor: agent("claude", "s1"), Kind: contextop.KindWiden, Targeted: true, Target: confirmedRecord},
			refused:    true,
		},
		{
			name:       "an agent proposes straight to the workspace",
			transition: contextop.Transition{Actor: agent("claude", "s1"), Kind: contextop.KindPropose, Widening: true},
			refused:    true,
		},
		{
			name:       "an agent withdraws a confirmed rule",
			transition: contextop.Transition{Actor: agent("claude", "s1"), Kind: contextop.KindRevert, Targeted: true, Target: confirmedRecord},
			refused:    true,
		},
		{
			name: "an agent discards another agent's proposal",
			transition: contextop.Transition{
				Actor: agent("claude", "s2"), Kind: contextop.KindDiscard, Targeted: true, Target: agentRecord,
			},
			refused: true,
		},
		{
			name: "an agent discards a person's proposal",
			transition: contextop.Transition{
				Actor:    agent("claude", "s1"),
				Kind:     contextop.KindDiscard,
				Targeted: true,
				Target:   contextop.Record{Actor: person("asgeir"), Kind: contextop.KindPropose, Status: contextop.StatusCandidate},
			},
			refused: true,
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
		Kind: contextop.KindPropose, Subject: termRule("utilise", "use", ""),
	})
	require.NoError(t, err)

	_, err = ledger.Append(ctx, contextop.Record{
		Project: "prj_docs", Actor: agent("claude", "s1"),
		Kind: contextop.KindConfirm, Target: proposed.ID,
	})
	require.ErrorIs(t, err, contextop.ErrRefused)

	still, err := ledger.Get(ctx, proposed.ID)
	require.NoError(t, err)
	assert.Equal(t, contextop.StatusCandidate, still.Status, "a refused confirmation records nothing")

	_, err = ledger.Append(ctx, contextop.Record{
		Project: "prj_docs", Actor: person("asgeir"),
		Kind: contextop.KindConfirm, Target: proposed.ID,
	})
	require.NoError(t, err)
	confirmed, err := ledger.Get(ctx, proposed.ID)
	require.NoError(t, err)
	assert.Equal(t, contextop.StatusConfirmed, confirmed.Status, "a person confirms the same rule")
}

// TestLedger_DefaultPolicyIsPersonDecides pins the default, because a nil
// policy that allowed everything would be an agent with a person's rights.
func TestLedger_DefaultPolicyIsPersonDecides(t *testing.T) {
	ctx := t.Context()
	ledger := contextop.NewLedger(openWorkspace(t), nil)
	_, err := ledger.Append(ctx, contextop.Record{
		Project: "prj_docs", Actor: agent("claude", "s1"),
		Kind: contextop.KindPropose, Subject: termRule("utilise", "use", ""),
	})
	require.NoError(t, err)
	_, err = ledger.Append(ctx, contextop.Record{
		Project: "prj_docs", Actor: agent("claude", "s1"),
		Kind: contextop.KindWiden, Target: "1",
	})
	assert.ErrorIs(t, err, contextop.ErrRefused)
}
