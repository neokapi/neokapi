package backend

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/screens"
)

// TestKeptRules_NamesWhoKeptEachRule: a suggestion an agent made and a person
// kept lists as kept by the person, with the day it came into force, and the
// rules the listing holds no keeper for are left out.
func TestKeptRules_NamesWhoKeptEachRule(t *testing.T) {
	app, recipe, tabID := openKapiMart(t)
	engine := agentProcess(t, app)
	word := agentSuggest(t, engine, recipe, "sess-1", "business", "studio", "site/about.html", "Our business plan.")
	agentSuggest(t, engine, recipe, "sess-1", "purchase", "buy", "site/about.html", "Purchase today.")

	before, err := app.KeptRules(tabID)
	require.NoError(t, err)

	_, err = app.DecideRule(tabID, word.ID, host.RuleAnswerKeep, "")
	require.NoError(t, err)

	kept, err := app.KeptRules(tabID)
	require.NoError(t, err)
	require.Len(t, kept, len(before)+1, "the kept suggestion lists; the waiting one does not")

	var found *screens.KeptRuleDTO
	for i, k := range kept {
		assert.NotContains(t, k.Rule, "purchase", "a suggestion still waiting is not kept")
		if strings.Contains(k.Rule, "studio") && strings.Contains(k.Rule, "business") {
			found = &kept[i]
		}
	}
	require.NotNil(t, found, "the kept word rule lists: %+v", kept)
	assert.Equal(t, "word", found.Kind)
	assert.NotEmpty(t, found.Applies, "where the rule holds")
	assert.Equal(t, screens.ContextActorDTO{Kind: "person"}, found.KeptBy, "kept by the person, not the agent that suggested it")
	at, err := time.Parse(time.RFC3339, found.KeptAt)
	require.NoError(t, err)
	assert.WithinDuration(t, time.Now(), at, time.Hour)
	assert.NotContains(t, found.From, "kept by", "From says where the rule is held, not who kept it")

	// Every kept rule is a line `kapi rules list` prints.
	listing, err := engine.ListRules(context.Background(), host.RulesRequest{Project: recipe})
	require.NoError(t, err)
	var sentences []string
	for _, g := range listing.Groups {
		for _, l := range g.Rules {
			sentences = append(sentences, l.Sentence)
		}
	}
	for _, k := range kept {
		assert.Contains(t, sentences, k.Rule)
	}
}

// TestKeptRules_UnknownTab: a tab that is not open reads nothing.
func TestKeptRules_UnknownTab(t *testing.T) {
	app := NewApp()
	t.Cleanup(func() { app.shutdownEngine() })
	_, err := app.KeptRules("no-such-tab")
	require.Error(t, err)
}

// TestKeptRuleKind: a line's kind follows its group, and a note is a note.
func TestKeptRuleKind(t *testing.T) {
	assert.Equal(t, "name", keptRuleKind(host.RuleLine{Group: host.RulesGroupNames}))
	assert.Equal(t, "tone", keptRuleKind(host.RuleLine{Group: host.RulesGroupTone}))
	assert.Equal(t, "word", keptRuleKind(host.RuleLine{Group: host.RulesGroupWords}))
	_, _, ok := host.RuleLine{}.Kept()
	assert.False(t, ok, "a line built outside the log names no keeper")
	assert.Equal(t, contextop.SubjectKind(""), host.RuleLine{}.SubjectKind())
}
