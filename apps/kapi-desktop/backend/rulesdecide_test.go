package backend

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/screens"
	"github.com/neokapi/neokapi/kapi-desktop/backend/sample"
)

// openKapiMart scaffolds the bundled sample, opens it in a tab and returns the
// recipe and the tab. The sample reads its context into the workspace the
// data directory holds, so the app and the scaffold share one of their own.
func openKapiMart(t *testing.T) (*App, string, string) {
	t.Helper()
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("KAPI_DATA_DIR", t.TempDir())
	app := NewApp()
	t.Cleanup(func() { app.shutdownEngine() })
	dir := t.TempDir()
	require.NoError(t, sample.Scaffold("kapimart", dir))
	recipe := filepath.Join(dir, "kapi.yaml")
	tab, err := app.OpenProject(recipe)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
	return app, recipe, tab.ID
}

// decisionIDs are the ids a view's questions answer to: a suggestion's own,
// and each side's of a disagreement.
func decisionIDs(decisions []screens.RuleDecisionDTO) []string {
	var out []string
	for _, d := range decisions {
		if d.ID != "" {
			out = append(out, d.ID)
		}
		for _, o := range d.Options {
			if o.ID != "" {
				out = append(out, o.ID)
			}
		}
	}
	return out
}

// TestRulesDecideView_KapiMart: Decide asks about the suggestions `kapi
// rules` asks about, the ones the Map lists, with the forms of a name counted
// over the sample's blocks and what keeping each does stated.
func TestRulesDecideView_KapiMart(t *testing.T) {
	app, recipe, tabID := openKapiMart(t)
	engine := agentProcess(t, app)
	quickcast := agentSuggest(t, engine, recipe, "sess-1", "Quick cast", "Quickcast", "site/index.html", "Try Quick cast today.")
	business := agentSuggest(t, engine, recipe, "sess-1", "business", "studio", "site/about.html", "Our business plan.")

	view, err := app.RulesDecideView(tabID)
	require.NoError(t, err)
	assert.Equal(t, "KapiMart", view.ProjectName)
	assert.Positive(t, view.Rules, "the sample's context is imported, so it holds rules")
	assert.Equal(t, 2, view.Suggested)

	// The same questions the terminal asks, in the same order.
	terminal, err := engine.DecideRules(context.Background(), host.RulesRequest{Project: recipe})
	require.NoError(t, err)
	require.Len(t, view.Decisions, len(terminal.Decisions))
	for i, d := range terminal.Decisions {
		assert.Equal(t, d.ID, view.Decisions[i].ID)
		assert.Equal(t, d.Question, view.Decisions[i].Question)
		assert.Equal(t, d.Consequence, view.Decisions[i].Consequence)
	}
	assert.ElementsMatch(t, []string{quickcast.ID, business.ID}, decisionIDs(view.Decisions))

	// Every suggestion the Map lists opens a question here.
	summary, err := app.MapSummary(tabID)
	require.NoError(t, err)
	for _, row := range summary.Suggestions {
		assert.Contains(t, decisionIDs(view.Decisions), row.Item.ID)
	}

	byID := map[string]screens.RuleDecisionDTO{}
	for _, d := range view.Decisions {
		byID[d.ID] = d
	}
	name := byID[quickcast.ID]
	assert.Equal(t, host.RuleDecisionName, name.Kind)
	assert.Equal(t, "Which one do your files use?", name.Question)
	assert.Equal(t, "Quickcast", name.Label)
	assert.True(t, name.Other)
	assert.Equal(t, "agent", name.NoticedBy.Kind)
	require.NotNil(t, name.Seen)
	assert.Equal(t, "Try Quick cast today.", name.Seen.Quote)
	assert.Equal(t, "site/index.html", name.Seen.Path)
	require.NotEmpty(t, name.Options)
	for i := 1; i < len(name.Options); i++ {
		assert.GreaterOrEqual(t, name.Options[i-1].Blocks, name.Options[i].Blocks, "the most used form first")
	}
	assert.Equal(t, "Quickcast", name.Options[name.Chosen].Form, "the form the suggestion names is chosen")
	for _, o := range name.Options {
		assert.Contains(t, o.Consequence, "If you keep \""+o.Form+"\": ")
	}

	word := byID[business.ID]
	assert.Equal(t, host.RuleDecisionWord, word.Kind)
	assert.Equal(t, "Keep this as a rule?", word.Question)
	assert.Equal(t, "business", word.Label)
	assert.Equal(t, 1, word.Avoided)
	assert.False(t, word.Other)
	assert.Contains(t, word.Consequence, "If you keep it: ")

	// A form the person types is counted over the same blocks.
	typed, err := app.RulesDecideConsequence(tabID, quickcast.ID, "QuickCast")
	require.NoError(t, err)
	assert.Contains(t, typed, "If you keep \"QuickCast\": ")
	_, err = app.RulesDecideConsequence(tabID, business.ID, "trade")
	require.Error(t, err, "only a name takes a form of its own")
}

// TestRulesDecideView_CountsAgainWhenAFileChanges: the questions count over
// the blocks the last read took while no file changes, and a file written
// since is read again, so the counts and what keeping a typed form does follow
// the files.
func TestRulesDecideView_CountsAgainWhenAFileChanges(t *testing.T) {
	app, recipe, tabID := openKapiMart(t)
	engine := agentProcess(t, app)
	name := agentSuggest(t, engine, recipe, "sess-1", "Kapi Mart", "KapiMart", "web/en/getting-started.md", "Welcome to the KapiMart Developer Platform!")

	blocksOf := func(view *screens.RulesDecideViewDTO, form string) int {
		t.Helper()
		for _, d := range view.Decisions {
			if d.ID != name.ID {
				continue
			}
			for _, o := range d.Options {
				if o.Form == form {
					return o.Blocks
				}
			}
		}
		t.Fatalf("no option %q on %s", form, name.ID)
		return 0
	}
	first, err := app.RulesDecideView(tabID)
	require.NoError(t, err)
	again, err := app.RulesDecideView(tabID)
	require.NoError(t, err)
	assert.Equal(t, blocksOf(first, "Kapi Mart"), blocksOf(again, "Kapi Mart"))
	typedBefore, err := app.RulesDecideConsequence(tabID, name.ID, "KapiMart")
	require.NoError(t, err)

	path := filepath.Join(filepath.Dir(recipe), "web", "en", "faq.md")
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, append(body, []byte("\n\nVisit Kapi Mart today.\n")...), 0o600))
	later := time.Now().Add(2 * time.Second)
	require.NoError(t, os.Chtimes(path, later, later))

	changed, err := app.RulesDecideView(tabID)
	require.NoError(t, err)
	assert.Equal(t, blocksOf(again, "Kapi Mart")+1, blocksOf(changed, "Kapi Mart"), "the written file is read again")
	typedAfter, err := app.RulesDecideConsequence(tabID, name.ID, "KapiMart")
	require.NoError(t, err)
	assert.NotEqual(t, typedBefore, typedAfter, "a typed form is counted over the new read")
}

// TestDecideRule_KeepRecordsWhatRulesKeepRecords: keeping a suggestion in
// Decide appends the operation `kapi rules --keep` appends for the same kind
// of suggestion, and not a rule and later record what n and l record.
func TestDecideRule_KeepRecordsWhatRulesKeepRecords(t *testing.T) {
	app, recipe, tabID := openKapiMart(t)
	engine := agentProcess(t, app)
	inDecide := agentSuggest(t, engine, recipe, "sess-1", "business", "studio", "site/about.html", "Our business plan.")
	inTerminal := agentSuggest(t, engine, recipe, "sess-1", "purchase", "buy", "site/about.html", "Purchase today.")
	notARule := agentSuggest(t, engine, recipe, "sess-1", "utilise", "use", "site/about.html", "We utilise it.")
	later := agentSuggest(t, engine, recipe, "sess-1", "commence", "start", "site/about.html", "Commence now.")

	res, err := app.DecideRule(tabID, inDecide.ID, host.RuleAnswerKeep, "")
	require.NoError(t, err)
	assert.Equal(t, &screens.RuleDecideResultDTO{Answer: "keep", Kept: 1}, res)
	// What `kapi rules --keep <id>` calls, from a process of its own.
	_, err = engine.DecideContextReview(context.Background(), host.ContextReviewRequest{
		Actor: contextop.Actor{Kind: contextop.ActorPerson}, Project: recipe, Keep: []string{inTerminal.ID},
	})
	require.NoError(t, err)

	log, err := engine.ContextOperations(context.Background(), host.ContextLogRequest{Project: recipe})
	require.NoError(t, err)
	decisionOn := func(target string) contextop.Record {
		t.Helper()
		for _, op := range log.Operations {
			if op.Target == target {
				return op.Record
			}
		}
		t.Fatalf("no decision on %s", target)
		return contextop.Record{}
	}
	desk, term := decisionOn(inDecide.ID), decisionOn(inTerminal.ID)
	assert.Equal(t, term.Kind, desk.Kind)
	assert.Equal(t, contextop.KindKeep, desk.Kind)
	assert.Equal(t, term.Actor.Kind, desk.Actor.Kind)
	assert.Equal(t, term.Project, desk.Project)
	assert.Equal(t, term.Scope, desk.Scope)
	assert.Equal(t, term.Subject.Kind, desk.Subject.Kind)
	for _, id := range []string{inDecide.ID, inTerminal.ID} {
		subject, serr := engine.ContextSubject(context.Background(), recipe, id)
		require.NoError(t, serr)
		assert.Equal(t, contextop.StatusEstablished, subject.Status)
	}

	dropped, err := app.DecideRule(tabID, notARule.ID, host.RuleAnswerDrop, "")
	require.NoError(t, err)
	assert.Equal(t, 1, dropped.Dropped)
	subject, err := engine.ContextSubject(context.Background(), recipe, notARule.ID)
	require.NoError(t, err)
	assert.Equal(t, contextop.StatusDropped, subject.Status)

	left, err := app.DecideRule(tabID, later.ID, host.RuleAnswerLater, "")
	require.NoError(t, err)
	assert.Equal(t, 1, left.Later)
	subject, err = engine.ContextSubject(context.Background(), recipe, later.ID)
	require.NoError(t, err)
	assert.Equal(t, contextop.StatusSuggested, subject.Status, "later records nothing")

	view, err := app.RulesDecideView(tabID)
	require.NoError(t, err)
	assert.Equal(t, []string{later.ID}, decisionIDs(view.Decisions))
}

// TestDecideRule_ChoosesASide: two assistants disagree about one word, and
// keeping one side in Decide chooses it and sets the other aside.
func TestDecideRule_ChoosesASide(t *testing.T) {
	app, recipe, tabID := openKapiMart(t)
	engine := agentProcess(t, app)
	studio := agentSuggest(t, engine, recipe, "sess-1", "business", "studio", "site/about.html", "Our business plan.")
	shop := agentSuggest(t, engine, recipe, "sess-2", "business", "shop", "site/about.html", "Our business plan.")

	view, err := app.RulesDecideView(tabID)
	require.NoError(t, err)
	require.Len(t, view.Decisions, 1)
	d := view.Decisions[0]
	assert.Equal(t, host.RuleDecisionDisagreement, d.Kind)
	assert.Empty(t, d.ID)
	require.Len(t, d.Options, 2)
	for _, o := range d.Options {
		require.NotNil(t, o.NoticedBy)
		assert.Equal(t, "agent", o.NoticedBy.Kind)
		assert.NotEmpty(t, o.Consequence)
	}

	res, err := app.DecideRule(tabID, shop.ID, host.RuleAnswerKeep, "")
	require.NoError(t, err)
	assert.Equal(t, 1, res.Kept)
	assert.Equal(t, 1, res.Dropped)
	kept, err := engine.ContextSubject(context.Background(), recipe, shop.ID)
	require.NoError(t, err)
	assert.Equal(t, contextop.StatusEstablished, kept.Status)
	other, err := engine.ContextSubject(context.Background(), recipe, studio.ID)
	require.NoError(t, err)
	assert.Equal(t, contextop.StatusDropped, other.Status)
}

// TestRulesDecideView_FolderWithoutRecipe: a folder opened without a kapi.yaml
// has nothing recorded, and Decide asks nothing and writes no recipe.
func TestRulesDecideView_FolderWithoutRecipe(t *testing.T) {
	app := newWorkspaceApp(t)
	dir := t.TempDir()
	tab, err := app.OpenProject(dir)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })

	view, err := app.RulesDecideView(tab.ID)
	require.NoError(t, err)
	assert.Empty(t, view.Decisions)
	assert.Zero(t, view.Rules)
	assert.NoFileExists(t, filepath.Join(dir, "kapi.yaml"))

	_, err = app.RulesDecideView("no-such-tab")
	require.Error(t, err)
}
