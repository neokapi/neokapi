package host

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextop"
	coreprofile "github.com/neokapi/neokapi/core/profile"
)

// rulesProject writes a project whose docs write one name three ways and use
// a word a rule avoids. With voice set, the recipe binds the technical-docs
// starter pack.
func rulesProject(t *testing.T, name string, voice bool) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		path := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	recipe := "version: v1\nid: " + projectIDFor(name) + "\nname: " + name + "\ndefaults:\n  source_language: en\n"
	if voice {
		recipe += "  voice: {pack: technical-docs}\n"
	}
	recipe += `profiles:
  acme:
    channels: [docs, help]
collections:
  - name: docs
    channel: acme/docs
    source_only: true
    content:
      - path: "docs/*.md"
  - name: help
    channel: acme/help
    source_only: true
    content:
      - path: "help/*.md"
`
	write("kapi.yaml", recipe)
	write("docs/a.md", "# Login\n\nSign in to continue.\n\nPlease Sign In first.\n\nSign In again.\n")
	write("docs/b.md", "# Help\n\nSign in with your account.\n\nWe utilise the widget.\n")
	write("help/c.md", "# More\n\nSign In from the menu.\n")
	return root
}

var rulesAgent = contextop.Actor{Kind: contextop.ActorAgent, Name: "codex", Session: "s-rules"}

// suggestName records, as an assistant, that the files write "Sign in" and
// avoid "Sign In".
func suggestName(t *testing.T, app *App, root string) ContextOperation {
	t.Helper()
	op, err := app.RecordContextObservation(t.Context(), ContextObserveRequest{
		Actor:   rulesAgent,
		Project: recipeOf(root),
		Term:    "Sign in", InsteadOf: []string{"Sign In"},
		Evidence: []contextop.Evidence{{Path: "docs/a.md", Quote: "Please Sign In first."}},
	})
	require.NoError(t, err)
	return op
}

func suggestWord(t *testing.T, app *App, root string) ContextOperation {
	t.Helper()
	op, err := app.RecordContextObservation(t.Context(), ContextObserveRequest{
		Actor:   rulesAgent,
		Project: recipeOf(root),
		Term:    "use", InsteadOf: []string{"utilise"},
		Evidence: []contextop.Evidence{{Path: "docs/b.md", Quote: "We utilise the widget."}},
	})
	require.NoError(t, err)
	return op
}

func decisionFor(t *testing.T, v *RulesDecideView, id string) RuleDecision {
	t.Helper()
	for _, d := range v.Decisions {
		if d.ID == id {
			return d
		}
	}
	t.Fatalf("no decision about %s", id)
	return RuleDecision{}
}

func TestDecideRulesAsksWhichFormTheFilesUse(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := rulesProject(t, "rulesdecideform", false)
	name := suggestName(t, app, root)

	view, err := app.DecideRules(t.Context(), RulesRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	d := decisionFor(t, view, name.ID)
	assert.Equal(t, RuleDecisionName, d.Kind)
	assert.Equal(t, "Which one do your files use?", d.Question)
	assert.Equal(t, 0, d.Chosen, "the form the suggestion names is chosen until the person picks another")
	require.GreaterOrEqual(t, len(d.Options), 2)

	// docs/ and help/ both sit inside the suggestion's scope only where the
	// evidence's point covers them: the suggestion was seen in docs.
	assert.Equal(t, "Sign in", d.Options[0].Form)
	assert.Equal(t, 2, d.Options[0].Blocks)
	assert.Equal(t, 2, d.Options[0].Files)
	assert.Equal(t, "Sign In", d.Options[1].Form)
	assert.Equal(t, 2, d.Options[1].Blocks, "only the blocks at the suggestion's point count")
	assert.Equal(t, 1, d.Options[1].Files)
	assert.Equal(t, "docs", d.Applies)
	others := len(d.Options) - 1
	assert.Equal(t,
		"If you keep \"Sign in\": the other "+itoa(others)+" forms become findings in 2 blocks. Applies to docs.",
		d.Options[0].Consequence)
	assert.Equal(t,
		"If you keep \"Sign In\": the other "+itoa(others)+" forms become findings in 2 blocks. Applies to docs.",
		d.Options[1].Consequence)
	assert.Contains(t, view.Consequence(d, "Sign-In"), "If you keep \"Sign-In\": the other")

	// Keeping the second form records the keep `context review --keep --use`
	// records, with the forms traded so the rule never avoids what it prefers.
	req := d.KeepRequest(1, "")
	assert.Equal(t, []string{name.ID}, req.Keep)
	assert.Equal(t, "Sign In", req.Replacement)
	req.Project, req.Actor = recipeOf(root), person
	_, err = app.DecideContextReview(t.Context(), req)
	require.NoError(t, err)
	kept, err := app.ContextSubject(t.Context(), recipeOf(root), name.ID)
	require.NoError(t, err)
	assert.Equal(t, contextop.StatusEstablished, kept.Status)
	rule, ok := kept.Rule()
	require.True(t, ok)
	assert.Equal(t, "Sign In", rule.Replacement)
	assert.Contains(t, append([]string{rule.Term}, rule.Forms...), "Sign in")
	assert.NotContains(t, append([]string{rule.Term}, rule.Forms...), "Sign In")
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func TestDecideRulesRecordsKeepNotARuleAndLater(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := rulesProject(t, "rulesdecideeach", false)
	name := suggestName(t, app, root)
	word := suggestWord(t, app, root)

	view, err := app.DecideRules(t.Context(), RulesRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	require.Len(t, view.Decisions, 2)
	w := decisionFor(t, view, word.ID)
	assert.Equal(t, RuleDecisionWord, w.Kind)
	assert.Equal(t, "Keep this as a rule?", w.Question)
	assert.Equal(t, "If you keep it: \"utilise\" becomes a finding in 1 block. Applies to docs.", w.Consequence)

	// Not a rule is a drop, as `context review --drop` records it.
	drop := w.DropRequest()
	drop.Project, drop.Actor = recipeOf(root), person
	_, err = app.DecideContextReview(t.Context(), drop)
	require.NoError(t, err)
	dropped, err := app.ContextSubject(t.Context(), recipeOf(root), word.ID)
	require.NoError(t, err)
	assert.Equal(t, contextop.StatusDropped, dropped.Status)

	// Later records nothing: the suggestion keeps waiting.
	later, err := app.LeaveRulesForLater(t.Context(), recipeOf(root), []string{name.ID})
	require.NoError(t, err)
	assert.Equal(t, []string{name.ID}, later.Later)
	waiting, err := app.ContextSubject(t.Context(), recipeOf(root), name.ID)
	require.NoError(t, err)
	assert.Equal(t, contextop.StatusSuggested, waiting.Status)
	_, err = app.LeaveRulesForLater(t.Context(), recipeOf(root), []string{word.ID})
	require.ErrorContains(t, err, "not a suggestion waiting", "a decided suggestion is not left for later")

	// Keep, with the form the suggestion names.
	n := decisionFor(t, view, name.ID)
	keep := n.KeepRequest(n.Chosen, "")
	assert.Empty(t, keep.Replacement, "keeping the form the suggestion names changes nothing about it")
	keep.Project, keep.Actor = recipeOf(root), person
	_, err = app.DecideContextReview(t.Context(), keep)
	require.NoError(t, err)

	after, err := app.DecideRules(t.Context(), RulesRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	assert.Empty(t, after.Decisions)
	assert.Equal(t, "kapi context suggest", after.Hint)
	var text bytes.Buffer
	require.NoError(t, after.FormatText(&text))
	assert.Equal(t, "No suggestions to decide. 1 rule · 0 suggested · 1 narrowed.\nFind some in the files with: kapi context suggest\n", text.String())
}

func TestListRulesGroupsAndCounts(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := rulesProject(t, "ruleslist", true)
	name := suggestName(t, app, root)
	word := suggestWord(t, app, root)
	note, err := app.RecordContextObservation(t.Context(), ContextObserveRequest{
		Actor: rulesAgent, Project: recipeOf(root), Text: "The docs address the reader as you.",
		Evidence: []contextop.Evidence{{Path: "docs/a.md"}},
	})
	require.NoError(t, err)
	_, err = app.DecideContextReview(t.Context(), ContextReviewRequest{
		Actor: person, Project: recipeOf(root), Keep: []string{word.ID, note.ID},
	})
	require.NoError(t, err)

	listing, err := app.ListRules(t.Context(), RulesRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	groups := map[string]RulesGroup{}
	var order []string
	for _, g := range listing.Groups {
		groups[g.Group] = g
		order = append(order, g.Group)
	}
	assert.Equal(t, []string{RulesGroupWords, RulesGroupTone, RulesGroupSuggested}, order)

	// The kept word, narrowed to the docs collection it was seen in, and the
	// starter pack's words.
	var kept *RuleLine
	for i, l := range groups[RulesGroupWords].Rules {
		if l.ID == word.ID {
			kept = &groups[RulesGroupWords].Rules[i]
		}
	}
	require.NotNil(t, kept, "the kept word is listed with its id: %+v", groups[RulesGroupWords].Rules)
	assert.Equal(t, "Write use, not utilise.", kept.Sentence)
	assert.True(t, hasPrefixSuffix(kept.From, "kept by ", ", "+keptDay(t, app, root, word.ID)), "%v", kept.From)
	assert.True(t, kept.Narrowed)
	assert.Equal(t, "docs", kept.Applies)
	assert.True(t, kept.Fails)
	assert.True(t, hasLine(groups[RulesGroupWords].Rules, func(l RuleLine) bool {
		return l.Sentence == "Avoid please." && !l.Fails && l.From[0] == "pack technical-docs"
	}), "an advisory word of the bound pack reports and fails nothing: %+v", groups[RulesGroupWords].Rules)

	// The voice's measures and patterns, and the kept note.
	tone := groups[RulesGroupTone].Rules
	assert.True(t, hasLine(tone, func(l RuleLine) bool {
		return l.Sentence == "Sentence length: short." && l.From[0] == "voice Technical Documentation"
	}))
	assert.True(t, hasLine(tone, func(l RuleLine) bool {
		return l.Sentence == "Avoid: Minimizing words that assume reader knowledge." && !l.Fails
	}), "an unmarked pattern reports")
	assert.True(t, hasLine(tone, func(l RuleLine) bool {
		return l.Sentence == "Avoid: Passive voice constructions." && !l.Fails
	}), "an advisory pattern fails nothing")
	assert.True(t, hasLine(tone, func(l RuleLine) bool { return l.ID == note.ID && !l.Fails }))

	// The suggestion still waiting.
	require.Len(t, groups[RulesGroupSuggested].Rules, 1)
	s := groups[RulesGroupSuggested].Rules[0]
	assert.Equal(t, name.ID, s.ID)
	assert.Equal(t, []string{"suggested by codex"}, s.From)
	assert.False(t, s.Fails)

	rules := len(groups[RulesGroupWords].Rules) + len(tone)
	assert.Equal(t, RulesCounts{Rules: rules, Suggested: 1, Narrowed: 2}, listing.Counts)
	var text bytes.Buffer
	require.NoError(t, listing.FormatText(&text))
	assert.Contains(t, text.String(), listing.Counts.Line()+"\n\nWords\n")
	assert.Contains(t, text.String(), "Write use, not utilise.  kept by ")
	assert.Contains(t, text.String(), "applies to docs  #"+contextop.ShortID(word.ID))

	// --json carries the same listing.
	data, err := json.Marshal(listing)
	require.NoError(t, err)
	var shape struct {
		Counts RulesCounts `json:"counts"`
		Groups []struct {
			Group string `json:"group"`
			Rules []struct {
				ID       string   `json:"id"`
				Sentence string   `json:"sentence"`
				From     []string `json:"from"`
				Applies  string   `json:"applies"`
				Fails    bool     `json:"fails"`
			} `json:"rules"`
		} `json:"groups"`
	}
	require.NoError(t, json.Unmarshal(data, &shape))
	assert.Equal(t, listing.Counts, shape.Counts)
	assert.Equal(t, RulesGroupWords, shape.Groups[0].Group)
}

func keptDay(t *testing.T, app *App, root, id string) string {
	t.Helper()
	ops, err := app.ContextOperations(t.Context(), ContextLogRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	for _, op := range ops.Operations {
		if op.Kind == contextop.KindKeep && op.Target == id {
			return op.At.UTC().Format("2006-01-02")
		}
	}
	t.Fatalf("no keep of %s", id)
	return ""
}

func hasPrefixSuffix(list []string, prefix, suffix string) bool {
	return slices.ContainsFunc(list, func(s string) bool {
		return strings.HasPrefix(s, prefix) && strings.HasSuffix(s, suffix)
	})
}

func hasLine(lines []RuleLine, match func(RuleLine) bool) bool {
	return slices.ContainsFunc(lines, match)
}

// TestVoiceLinesPatternOutcome: a voice pattern is listed as failing only
// when it is marked `fails: true` and not `advisory: true`.
func TestVoiceLinesPatternOutcome(t *testing.T) {
	v := &coreprofile.VoiceProfile{Name: "house", Style: coreprofile.StyleRules{
		ProhibitedPatterns: []coreprofile.Pattern{
			{Regex: "a", Description: "Unmarked"},
			{Regex: "b", Description: "Marked", Fails: true},
			{Regex: "c", Description: "Both", Fails: true, Advisory: true},
		},
		RequiredPatterns: []coreprofile.Pattern{{Regex: "d", Description: "Required", Fails: true}},
	}}
	fails := map[string]bool{}
	for _, l := range voiceLines(v) {
		fails[l.Sentence] = l.Fails
	}
	assert.Equal(t, map[string]bool{
		"Avoid: Unmarked.":   false,
		"Avoid: Marked.":     true,
		"Avoid: Both.":       false,
		"Require: Required.": true,
	}, fails)
}

func TestNarrowRuleToACollection(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := rulesProject(t, "rulesnarrow", false)
	word := suggestWord(t, app, root)
	_, err := app.DecideContextReview(t.Context(), ContextReviewRequest{
		Actor: person, Project: recipeOf(root), Keep: []string{word.ID}, WidenTo: WidenToProject,
	})
	require.NoError(t, err)
	before, err := app.ContextSubject(t.Context(), recipeOf(root), word.ID)
	require.NoError(t, err)
	require.True(t, before.Scope.AllProfiles, "widened to the project first")

	res, err := app.NarrowRule(t.Context(), RulesNarrowRequest{Actor: person, Project: recipeOf(root), ID: word.ID, To: "help"})
	require.NoError(t, err)
	assert.Equal(t, "help", res.Applies)
	assert.Equal(t, "help", res.Collection)
	after, err := app.ContextSubject(t.Context(), recipeOf(root), word.ID)
	require.NoError(t, err)
	assert.Equal(t, contextop.StatusEstablished, after.Status)
	assert.False(t, after.Scope.AllProfiles)
	assert.Equal(t, "help", after.Scope.Coordinates["channel"])

	// A folder reads as the point its files sit at.
	_, err = app.NarrowRule(t.Context(), RulesNarrowRequest{Actor: person, Project: recipeOf(root), ID: word.ID, To: "docs/"})
	require.ErrorContains(t, err, "does not hold at docs")

	// A suggestion is kept before it is narrowed.
	name := suggestName(t, app, root)
	_, err = app.NarrowRule(t.Context(), RulesNarrowRequest{Actor: person, Project: recipeOf(root), ID: name.ID, To: "docs"})
	require.ErrorContains(t, err, "keep it before narrowing it")
}

func TestNarrowRuleRefusesAFolderAtTheProjectsOwnPoint(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := contextOpsProject(t, "rulesnarrowgap")
	op, err := app.RecordContextObservation(t.Context(), ContextObserveRequest{
		Actor: rulesAgent, Project: recipeOf(root), Term: "use", InsteadOf: []string{"utilise"},
		Evidence: []contextop.Evidence{{Path: "config/app.yaml"}},
	})
	require.NoError(t, err)
	_, err = app.DecideContextReview(t.Context(), ContextReviewRequest{Actor: person, Project: recipeOf(root), Keep: []string{op.ID}})
	require.NoError(t, err)

	_, err = app.NarrowRule(t.Context(), RulesNarrowRequest{Actor: person, Project: recipeOf(root), ID: op.ID, To: "config"})
	require.ErrorContains(t, err, "sits at the project's own point")
	_, err = app.NarrowRule(t.Context(), RulesNarrowRequest{Actor: person, Project: recipeOf(root), ID: op.ID, To: "nowhere"})
	require.ErrorContains(t, err, "neither a collection of this project nor a folder in it")
}

func TestDecideRulesAsksWhichSideOfADisagreementHolds(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := rulesProject(t, "rulesdisagree", false)
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
	require.Len(t, view.Decisions, 1, "the two sides are one question")
	d := view.Decisions[0]
	assert.Equal(t, RuleDecisionDisagreement, d.Kind)
	require.Len(t, d.Options, 2)
	ids := []string{d.Options[0].ID, d.Options[1].ID}
	assert.ElementsMatch(t, []string{use.ID, employ.ID}, ids)
	for _, o := range d.Options {
		rule, _ := o.subject.Rule()
		assert.Equal(t, "If you keep \""+rule.Replacement+"\": \"utilise\" becomes a finding in 1 block. Applies to docs.", o.Consequence)
	}

	var text bytes.Buffer
	require.NoError(t, view.FormatText(&text))
	assert.Contains(t, text.String(), "--choose <id>", "a side is kept by choosing it")
	assert.NotContains(t, text.String(), "\n  \n", "a disagreement states no sentence of its own")

	// Keeping the marked side is a choice, which sets the other side aside.
	req := d.KeepRequest(d.Chosen, "")
	assert.Equal(t, d.Options[d.Chosen].ID, req.Choose)
	assert.Empty(t, req.Keep)
}

func TestFindRulesNamesRulesSuggestionsAndDecisions(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := rulesProject(t, "rulesfind", false)
	name := suggestName(t, app, root)
	word := suggestWord(t, app, root)
	_, err := app.DecideContextReview(t.Context(), ContextReviewRequest{Actor: person, Project: recipeOf(root), Drop: []string{word.ID}})
	require.NoError(t, err)

	found, err := app.FindRules(t.Context(), RulesFindRequest{Project: recipeOf(root), Word: "SIGN"})
	require.NoError(t, err)
	require.Len(t, found.Suggested, 1)
	assert.Equal(t, name.ID, found.Suggested[0].ID)
	assert.Equal(t, "docs", found.Suggested[0].Applies)
	assert.Empty(t, found.Decided)

	found, err = app.FindRules(t.Context(), RulesFindRequest{Project: recipeOf(root), Word: "utilise"})
	require.NoError(t, err)
	require.Len(t, found.Decided, 1)
	assert.Equal(t, word.ID, found.Decided[0].ID)
	assert.Equal(t, "not a rule", found.Decided[0].Status)
	assert.True(t, hasPrefixSuffix(found.Decided[0].From, "decided by ", ""), "%v", found.Decided[0].From)

	var text bytes.Buffer
	require.NoError(t, found.FormatText(&text))
	assert.Contains(t, text.String(), "Decided against\n  Write use, not utilise.")

	none, err := app.FindRules(t.Context(), RulesFindRequest{Project: recipeOf(root), Word: "zebra"})
	require.NoError(t, err)
	text.Reset()
	require.NoError(t, none.FormatText(&text))
	assert.Equal(t, "No rule, suggestion or decision mentions \"zebra\".\n", text.String())
}
