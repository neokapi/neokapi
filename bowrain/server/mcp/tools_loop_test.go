package mcp

import (
	"context"
	"testing"

	corebrand "github.com/neokapi/neokapi/core/brand"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoopTools_SuggestedRulesAndPromote(t *testing.T) {
	ctx := context.Background()
	store := &memBrandStore{
		profiles: []*corebrand.VoiceProfile{
			{ID: "p1", WorkspaceID: "ws1", Name: "Voice"},
		},
		suggested: []*corebrand.SuggestedRule{
			{Term: "utilize", Replacement: "use", CorrectionCount: 4, Dimension: corebrand.DimensionVocabulary},
			{Term: "leverage", Replacement: "use", CorrectionCount: 3, Dimension: corebrand.DimensionVocabulary},
		},
	}
	ms, err := NewMCPServer(store, Config{})
	require.NoError(t, err)

	// get_suggested_rules → both candidates pending.
	_, out, err := ms.handleGetSuggestedRules(ctx, nil, getSuggestedRulesInput{WorkspaceID: "ws1", ProfileID: "p1", MinCount: 3})
	require.NoError(t, err)
	require.Len(t, out.Candidates, 2)
	for _, c := range out.Candidates {
		assert.Equal(t, corebrand.RuleDecisionPending, c.Status)
	}

	// promote_rule → utilize becomes an enforced forbidden term.
	_, prom, err := ms.handlePromoteRule(ctx, nil, promoteRuleInput{ProfileID: "p1", Term: "utilize", Replacement: "use"})
	require.NoError(t, err)
	assert.True(t, prom.Promoted)
	p, _ := store.GetProfile(ctx, "p1")
	require.Len(t, p.Vocabulary.ForbiddenTerms, 1)
	assert.Equal(t, "utilize", p.Vocabulary.ForbiddenTerms[0].Term)

	// get_suggested_rules again → utilize now filtered (promoted), leverage remains.
	_, out, err = ms.handleGetSuggestedRules(ctx, nil, getSuggestedRulesInput{WorkspaceID: "ws1", ProfileID: "p1", MinCount: 3})
	require.NoError(t, err)
	require.Len(t, out.Candidates, 1)
	assert.Equal(t, "leverage", out.Candidates[0].Term)

	// ...and visible as promoted in the full history.
	_, all, err := ms.handleGetSuggestedRules(ctx, nil, getSuggestedRulesInput{WorkspaceID: "ws1", ProfileID: "p1", MinCount: 3, All: true})
	require.NoError(t, err)
	require.Len(t, all.Candidates, 2)
}
