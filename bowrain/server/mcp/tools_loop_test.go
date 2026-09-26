package mcp

import (
	"context"
	"testing"

	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoopTools_SuggestedRulesAndPromote(t *testing.T) {
	ctx := context.Background()
	store := &memVoiceStore{
		profiles: []*coreprofile.VoiceProfile{
			{ID: "p1", Scope: "ws1", Name: "Voice"},
		},
		suggested: []*coreprofile.SuggestedRule{
			{Term: "utilize", Replacement: "use", CorrectionCount: 4, Dimension: coreprofile.DimensionVocabulary},
			{Term: "leverage", Replacement: "use", CorrectionCount: 3, Dimension: coreprofile.DimensionVocabulary},
		},
	}
	tb := newTestTermsStore(t)
	ms, err := NewMCPServerWithStore(store, nil, Config{}, WithTermsResolver(singleTermsResolver{tb: tb}))
	require.NoError(t, err)

	// get_suggested_rules → both candidates pending.
	_, out, err := ms.handleGetSuggestedRules(ctx, nil, getSuggestedRulesInput{WorkspaceID: "ws1", ProfileID: "p1", MinCount: 3})
	require.NoError(t, err)
	require.Len(t, out.Candidates, 2)
	for _, c := range out.Candidates {
		assert.Equal(t, coreprofile.RuleDecisionPending, c.Status)
	}

	// promote_rule names no language and no project: it cannot place the term.
	_, _, err = ms.handlePromoteRule(ctx, nil, promoteRuleInput{ProfileID: "p1", Term: "utilize", Replacement: "use"})
	require.ErrorContains(t, err, "locale or project_id is required")

	// promote_rule → utilize becomes a forbidden term in the workspace terms
	// store, joined to the concept of its replacement.
	_, prom, err := ms.handlePromoteRule(ctx, nil, promoteRuleInput{ProfileID: "p1", Term: "utilize", Replacement: "use", Locale: "en"})
	require.NoError(t, err)
	assert.True(t, prom.Promoted)
	concepts, err := tb.Concepts(ctx)
	require.NoError(t, err)
	rules := terms.SourceWordRules(concepts, "en")
	require.Len(t, rules, 1)
	assert.Equal(t, "utilize", rules[0].Term)
	assert.Equal(t, "use", rules[0].Replacement)

	// Promoting it again changes nothing.
	_, prom, err = ms.handlePromoteRule(ctx, nil, promoteRuleInput{ProfileID: "p1", Term: "utilize", Replacement: "use", Locale: "en"})
	require.NoError(t, err)
	assert.False(t, prom.Promoted)

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
