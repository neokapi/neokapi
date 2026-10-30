package host

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextop"
)

// TestDecideRuleRecordsOneAnswerAtATime: the call a surface makes for one
// answer finds the question by a suggestion's id, and records the keep, the
// not a rule and the later `kapi rules` records for k, n and l.
func TestDecideRuleRecordsOneAnswerAtATime(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := rulesProject(t, "rulesdecideone", false)
	name := suggestName(t, app, root)
	word := suggestWord(t, app, root)
	recipe := recipeOf(root)

	view, err := app.DecideRules(t.Context(), RulesRequest{Project: recipe, WithCounts: true})
	require.NoError(t, err)
	require.Len(t, view.Decisions, 2)
	assert.Equal(t, 2, view.Counts.Suggested, "the counts are read while suggestions wait")
	assert.Empty(t, view.Hint)

	n, option, ok := view.Find(name.ID)
	require.True(t, ok)
	assert.Equal(t, -1, option)
	assert.Equal(t, "Sign in", n.Label())
	assert.Equal(t, rulesAgent, n.NoticedBy())
	w := decisionFor(t, view, word.ID)
	assert.Equal(t, "utilise", w.Label())
	assert.Equal(t, 1, w.Avoided())
	_, _, ok = view.Find("")
	assert.False(t, ok)

	later, err := app.DecideRule(t.Context(), RuleDecideRequest{Actor: person, Project: recipe, ID: word.ID, Answer: RuleAnswerLater})
	require.NoError(t, err)
	assert.Equal(t, []string{word.ID}, later.Later)
	assert.Nil(t, later.Review, "later records nothing")

	kept, err := app.DecideRule(t.Context(), RuleDecideRequest{Actor: person, Project: recipe, ID: name.ID, Answer: RuleAnswerKeep, Form: "Sign In"})
	require.NoError(t, err)
	require.NotNil(t, kept.Review)
	require.Len(t, kept.Review.Kept, 1)
	subject, err := app.ContextSubject(t.Context(), recipe, name.ID)
	require.NoError(t, err)
	assert.Equal(t, contextop.StatusEstablished, subject.Status)
	rule, _ := subject.Rule()
	assert.Equal(t, "Sign In", rule.Replacement, "the form picked is the one the rule writes")

	dropped, err := app.DecideRule(t.Context(), RuleDecideRequest{Actor: person, Project: recipe, ID: word.ID, Answer: RuleAnswerDrop})
	require.NoError(t, err)
	require.NotNil(t, dropped.Review)
	require.Len(t, dropped.Review.Dropped, 1)

	_, err = app.DecideRule(t.Context(), RuleDecideRequest{Actor: person, Project: recipe, ID: word.ID, Answer: RuleAnswerKeep})
	require.ErrorContains(t, err, "waits for a decision", "a decided suggestion is not answered twice")
}

// TestDecideRuleRefusesAnUnknownAnswer: an answer other than keep, drop and
// later records nothing.
func TestDecideRuleRefusesAnUnknownAnswer(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := rulesProject(t, "rulesdecideunknown", false)
	name := suggestName(t, app, root)

	_, err := app.DecideRule(t.Context(), RuleDecideRequest{Actor: person, Project: recipeOf(root), ID: name.ID, Answer: "maybe"})
	require.ErrorContains(t, err, "none of keep, drop and later")
	subject, err := app.ContextSubject(t.Context(), recipeOf(root), name.ID)
	require.NoError(t, err)
	assert.Equal(t, contextop.StatusSuggested, subject.Status)
}

// TestDecideRuleChoosesASide: a keep naming one side of a disagreement
// chooses it, which sets the other side aside.
func TestDecideRuleChoosesASide(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := rulesProject(t, "rulesdecideside", false)
	use := suggestWord(t, app, root)
	employ, err := app.RecordContextObservation(t.Context(), ContextObserveRequest{
		Actor:   rulesAgent,
		Project: recipeOf(root),
		Term:    "employ", InsteadOf: []string{"utilise"},
		Evidence: []contextop.Evidence{{Path: "docs/b.md", Quote: "We utilise the widget."}},
	})
	require.NoError(t, err)

	view, err := app.DecideRules(t.Context(), RulesRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	require.Len(t, view.Decisions, 1)
	d, option, ok := view.Find(employ.ID)
	require.True(t, ok)
	assert.Equal(t, employ.ID, d.Options[option].ID)
	assert.Equal(t, rulesAgent, d.Options[option].NoticedBy())
	assert.Equal(t, "utilise", d.Label(), "both sides avoid one wording")

	res, err := app.DecideRule(t.Context(), RuleDecideRequest{Actor: person, Project: recipeOf(root), ID: employ.ID, Answer: RuleAnswerKeep})
	require.NoError(t, err)
	require.NotNil(t, res.Review)
	require.NotNil(t, res.Review.Chosen)
	chosen, err := app.ContextSubject(t.Context(), recipeOf(root), employ.ID)
	require.NoError(t, err)
	assert.Equal(t, contextop.StatusEstablished, chosen.Status)
	other, err := app.ContextSubject(t.Context(), recipeOf(root), use.ID)
	require.NoError(t, err)
	assert.Equal(t, contextop.StatusDropped, other.Status)
}
