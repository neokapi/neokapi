package backend

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/workspace"
	"github.com/neokapi/neokapi/host"
)

// The feed is tested against a real workspace, real operations recorded
// through the host API, and a second host handle on the same workspace root
// standing in for the agent's process. Nothing here is mocked: the point of
// the surface is that two processes meet in one store, and a test that faked
// one of them would assert nothing about that.

// scaffoldCoordinateProject writes a recipe with a point of its own, so a rule
// proposed against it carries coordinates worth widening past.
func scaffoldCoordinateProject(t *testing.T, dir, id, name string) string {
	t.Helper()
	require.NoError(t, project.Save(filepath.Join(dir, project.RecipeFileName), &project.KapiProject{
		Version: project.CurrentVersion,
		ID:      id,
		Name:    name,
		Defaults: project.Defaults{
			SourceLanguage: "en-US",
			Coordinates:    map[string]string{"brand": "kapimart", "product": "store"},
		},
	}))
	return filepath.Join(dir, project.RecipeFileName)
}

// agentProcess is a second host.App on the same workspace root: the process an
// agent records from while the desktop is open.
func agentProcess(t *testing.T, app *App) *host.App {
	t.Helper()
	other := &host.App{}
	other.InitRegistries()
	other.SetWorkspaceRoot(workspaceRootOf(t, app))
	return other
}

// openFeedProject scaffolds a project, opens it in the app (which registers
// it) and returns the recipe path and the project's workspace key.
func openFeedProject(t *testing.T, app *App, name string) (recipe, key string) {
	t.Helper()
	dir := t.TempDir()
	id := project.NewID()
	recipe = scaffoldCoordinateProject(t, dir, id, name)
	tab, err := app.OpenProject(recipe)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
	return recipe, id
}

// agentSuggest records a term rule the way an agent's MCP server does: an
// agent actor, a session id, and the evidence the rule came from.
func agentSuggest(t *testing.T, engine *host.App, recipe, session, term, replacement, path, quote string) host.ContextOperation {
	t.Helper()
	op, err := engine.RecordContextObservation(context.Background(), host.ContextObserveRequest{
		Actor:   contextop.Actor{Kind: contextop.ActorAgent, Name: "claude@studio", Session: session},
		Project: recipe,
		Term:    replacement, InsteadOf: []string{term},
		Evidence: []contextop.Evidence{{
			Path: path, Unit: "unit-1", Quote: quote,
		}},
	})
	require.NoError(t, err)
	return op
}

// TestFeedCarriesWhatAnotherProcessRecorded: an agent proposes in its own
// process, and the desktop's feed shows the proposal with its evidence, its
// actor and the session it belongs to.
func TestFeedCarriesWhatAnotherProcessRecorded(t *testing.T) {
	app := newWorkspaceApp(t)
	recipe, key := openFeedProject(t, app, "KapiMart")
	engine := agentProcess(t, app)

	agentSuggest(t, engine, recipe, "sess-1", "sign in", "log in", "docs/pricing.md", "Please sign in to continue.")

	feed, err := app.ContextFeed("", 0)
	require.NoError(t, err)
	require.Len(t, feed.Groups, 1)

	group := feed.Groups[0]
	assert.Equal(t, "session:sess-1", group.ID)
	assert.Equal(t, "sess-1", group.Session)
	assert.Equal(t, "agent", group.Actor.Kind)
	assert.Equal(t, "claude", group.Actor.Name, "an actor named <client>@<host> shows the client")
	assert.Equal(t, "studio", group.Actor.Host, "and the machine it ran on")
	assert.Equal(t, "KapiMart", group.ProjectName)
	assert.Equal(t, 1, group.Recorded)
	assert.Equal(t, 1, group.Awaiting)

	require.Len(t, group.Entries, 1)
	entry := group.Entries[0]
	assert.Equal(t, "observe", entry.Kind)
	assert.Equal(t, "suggested", entry.Status)
	assert.True(t, entry.Decidable)
	assert.Equal(t, "sign in", entry.Subject.Term)
	assert.Equal(t, "log in", entry.Subject.Replacement)
	require.Len(t, entry.Evidence, 1)
	assert.Equal(t, "docs/pricing.md", entry.Evidence[0].Path)
	assert.Equal(t, "unit-1", entry.Evidence[0].Unit)
	assert.Equal(t, "Please sign in to continue.", entry.Evidence[0].Quote)
	assert.NotEmpty(t, entry.Recipe, "a checkout is here, so the entry can be decided on")

	assert.Equal(t, key, group.ProjectKey)
}

// TestKeepingWithAnEditIsWhatTheAgentReadsNext: the person changes the
// replacement before accepting, and the agent's own process sees the edited
// rule established on its next read.
func TestKeepingWithAnEditIsWhatTheAgentReadsNext(t *testing.T) {
	app := newWorkspaceApp(t)
	recipe, key := openFeedProject(t, app, "KapiMart")
	engine := agentProcess(t, app)

	proposal := agentSuggest(t, engine, recipe, "sess-1", "sign in", "log in", "docs/pricing.md", "Please sign in.")

	_, err := app.KeepContextSuggestion(ContextDecisionRequest{
		Project:     key,
		ID:          proposal.ID,
		Replacement: "sign in",
		Advisory:    new(true),
		Note:        "the store says sign in",
	})
	require.NoError(t, err)

	read, err := engine.ContextOperations(context.Background(), host.ContextLogRequest{
		Project: recipe, Subjects: true,
	})
	require.NoError(t, err)
	require.Len(t, read.Operations, 1)
	assert.Equal(t, contextop.StatusEstablished, read.Operations[0].Status)
	rule, ok := read.Operations[0].Rule()
	require.True(t, ok)
	assert.Equal(t, "sign in", rule.Replacement, "the person's edit is the rule in force")
	assert.True(t, rule.Advisory)

	feed, err := app.ContextFeed(key, 0)
	require.NoError(t, err)
	assert.Zero(t, awaitingIn(feed), "a decided candidate awaits nobody")
	confirmed := findFeedEntry(t, feed, proposal.ID)
	assert.Equal(t, "established", confirmed.Status)
	assert.True(t, confirmed.Revertible)
	assert.Contains(t, confirmed.WidenTo, "workspace")
	assert.Contains(t, confirmed.WidenTo, "brand")
}

// TestDroppingStopsASuggestionAnswering: a dropped suggestion leaves the feed's
// awaiting count and stays on the record as dropped.
func TestDroppingStopsASuggestionAnswering(t *testing.T) {
	app := newWorkspaceApp(t)
	recipe, key := openFeedProject(t, app, "KapiMart")
	engine := agentProcess(t, app)

	proposal := agentSuggest(t, engine, recipe, "sess-1", "utilise", "use", "docs/guide.md", "Utilise the store.")

	_, err := app.DropContextSuggestion(ContextDecisionRequest{
		Project: key, ID: proposal.ID, Note: "house style keeps utilise",
	})
	require.NoError(t, err)

	feed, err := app.ContextFeed(key, 0)
	require.NoError(t, err)
	assert.Zero(t, awaitingIn(feed))
	assert.Equal(t, "dropped", findFeedEntry(t, feed, proposal.ID).Status)
}

// TestRevertingASessionNamesWhatItUndoes: the confirmation is read before
// anything moves, and the revert takes every rule the session got confirmed
// back out.
func TestRevertingASessionNamesWhatItUndoes(t *testing.T) {
	app := newWorkspaceApp(t)
	recipe, key := openFeedProject(t, app, "KapiMart")
	engine := agentProcess(t, app)

	first := agentSuggest(t, engine, recipe, "sess-1", "sign in", "log in", "docs/a.md", "Sign in here.")
	agentSuggest(t, engine, recipe, "sess-1", "utilise", "use", "docs/b.md", "Utilise this.")
	_, err := app.KeepContextSuggestion(ContextDecisionRequest{Project: key, ID: first.ID})
	require.NoError(t, err)

	scope, err := app.ContextRevertScope(ContextRevertRequest{Project: key, Session: "sess-1"})
	require.NoError(t, err)
	assert.Equal(t, 2, scope.Operations, "the confirmation names how many operations go")
	assert.Len(t, scope.Rules, 1, "one of them is in force")

	result, err := app.RevertContextOperations(ContextRevertRequest{Project: key, Session: "sess-1"})
	require.NoError(t, err)
	assert.Equal(t, "sess-1", result.Session)
	assert.Equal(t, 2, result.Operations)

	feed, err := app.ContextFeed(key, 0)
	require.NoError(t, err)
	assert.Zero(t, awaitingIn(feed))
	for _, group := range feed.Groups {
		for _, entry := range group.Entries {
			if entry.Kind == "observe" {
				assert.Equal(t, "reverted", entry.Status)
			}
		}
	}
}

// TestWidenReachListsTheProjectsAndSaysWhatItDoesNotCompute: widening to the
// workspace names every registered project, and reports that content impact is
// not part of the answer.
func TestWidenReachListsTheProjectsAndSaysWhatItDoesNotCompute(t *testing.T) {
	app := newWorkspaceApp(t)
	recipe, key := openFeedProject(t, app, "KapiMart")
	_, second := openFeedProject(t, app, "BowMart")
	engine := agentProcess(t, app)

	proposal := agentSuggest(t, engine, recipe, "sess-1", "sign in", "log in", "docs/a.md", "Sign in here.")
	_, err := app.KeepContextSuggestion(ContextDecisionRequest{Project: key, ID: proposal.ID})
	require.NoError(t, err)

	preview, err := app.ContextWidenReach(key, proposal.ID, host.WidenToWorkspace)
	require.NoError(t, err)
	assert.Equal(t, "workspace", preview.Scope.Level)
	assert.Equal(t, "project", preview.From.Level)
	assert.Equal(t, "sign in", preview.Rule.Term)
	assert.False(t, preview.ContentImpact, "reach is listed; impact is not computed")

	keys := map[string]bool{}
	for _, p := range preview.Projects {
		keys[p.ProjectKey] = p.Current
	}
	assert.Len(t, keys, 2)
	assert.True(t, keys[key], "the rule already answers in its own project")
	require.Contains(t, keys, second)
	assert.False(t, keys[second], "and would newly answer in the other")
}

// TestWidenPastAnAxisNamesThePointsItWouldNewlyCover: dropping an axis widens
// the rule inside its own project, and the preview lists the declared points
// the widened rule reaches.
func TestWidenPastAnAxisNamesThePointsItWouldNewlyCover(t *testing.T) {
	app := newWorkspaceApp(t)
	dir := t.TempDir()
	id := project.NewID()
	recipe := filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, project.Save(recipe, &project.KapiProject{
		Version: project.CurrentVersion,
		ID:      id,
		Name:    "KapiMart",
		Defaults: project.Defaults{
			SourceLanguage: "en-US",
			Coordinates:    map[string]string{"product": "store"},
		},
		Profiles: map[string]project.Profile{
			"marketing": {Channels: []project.Channel{{ID: "web"}}},
		},
	}))
	tab, err := app.OpenProject(recipe)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })

	engine := agentProcess(t, app)
	proposal := agentSuggest(t, engine, recipe, "sess-1", "sign in", "log in", "docs/a.md", "Sign in here.")
	_, err = app.KeepContextSuggestion(ContextDecisionRequest{Project: id, ID: proposal.ID})
	require.NoError(t, err)

	preview, err := app.ContextWidenReach(id, proposal.ID, "product")
	require.NoError(t, err)
	assert.Equal(t, "project", preview.Scope.Level, "dropping an axis keeps the rule in its project")
	assert.NotContains(t, preview.Scope.Coordinates, "product")
	require.NotEmpty(t, preview.Points, "the recipe declares a point the widened rule newly covers")
	assert.Equal(t, "marketing/web", preview.Points[0].Ref)
}

// TestAPersonsWorkGroupsByDay: a person at a terminal records no session, so
// their operations group by actor and day rather than by session.
func TestAPersonsWorkGroupsByDay(t *testing.T) {
	app := newWorkspaceApp(t)
	recipe, _ := openFeedProject(t, app, "KapiMart")
	engine := agentProcess(t, app)

	_, err := engine.RecordContextObservation(context.Background(), host.ContextObserveRequest{
		Actor:    contextop.Actor{Kind: contextop.ActorPerson, Name: "asgeir"},
		Project:  recipe,
		Text:     "the store writes prices without a space before the currency",
		Evidence: []contextop.Evidence{{Path: "docs/pricing.md"}},
	})
	require.NoError(t, err)

	feed, err := app.ContextFeed("", 0)
	require.NoError(t, err)
	require.Len(t, feed.Groups, 1)
	group := feed.Groups[0]
	assert.Empty(t, group.Session)
	assert.Equal(t, "person", group.Actor.Kind)
	assert.Equal(t, time.Now().Local().Format(time.DateOnly), group.Day)
	assert.Equal(t, "actor:person/asgeir:"+group.Day, group.ID)
	assert.Equal(t, 1, group.Recorded)
	assert.Zero(t, group.Awaiting, "an observation states no rule, so there is nothing to decide")
	assert.False(t, group.Entries[0].Decidable)
}

// awaitingIn sums the suggestions awaiting a decision over a feed's groups.
func awaitingIn(feed *ContextFeed) int {
	n := 0
	for _, g := range feed.Groups {
		n += g.Awaiting
	}
	return n
}

// TestFeedNarrowsToOneProjectAndTheHomeReadsNews: a project's own feed shows
// its operations only, and the home screen reads what each project's digest
// holds that the person has not seen, rather than a count of work waiting.
func TestFeedNarrowsToOneProjectAndTheHomeReadsNews(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	app := newWorkspaceApp(t)
	first, firstKey := openFeedProject(t, app, "KapiMart")
	second, secondKey := openFeedProject(t, app, "BowMart")
	engine := agentProcess(t, app)

	agentSuggest(t, engine, first, "sess-1", "sign in", "log in", "docs/a.md", "Sign in.")
	agentSuggest(t, engine, second, "sess-2", "utilise", "use", "docs/b.md", "Utilise.")

	feed, err := app.ContextFeed(firstKey, 0)
	require.NoError(t, err)
	require.Len(t, feed.Groups, 1)
	assert.Equal(t, firstKey, feed.Groups[0].ProjectKey)

	news, err := app.ContextNews()
	require.NoError(t, err)
	byKey := map[string]int{}
	for _, n := range news {
		byKey[n.Project] = n.New
		assert.True(t, n.Since.IsZero(), "nobody has looked, so there is no marker")
	}
	assert.Equal(t, 1, byKey[firstKey])
	assert.Equal(t, 1, byKey[secondKey])

	// Once the person has looked at a project, it has nothing new to show.
	_, err = host.MarkContextDigestSeen(workspace.ProjectKey(firstKey), time.Now().UTC())
	require.NoError(t, err)
	news, err = app.ContextNews()
	require.NoError(t, err)
	require.Len(t, news, 1)
	assert.Equal(t, secondKey, news[0].Project)
}

// TestDecidingNeedsACheckoutOnThisMachine: a project registered from a machine
// that no longer holds it can be read here and not decided on, and the refusal
// says why.
func TestDecidingNeedsACheckoutOnThisMachine(t *testing.T) {
	app := newWorkspaceApp(t)
	_, err := app.KeepContextSuggestion(ContextDecisionRequest{Project: "absent", ID: "1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "absent")
}

// TestAQuietSessionIsSummarised: a session nothing has been added to for a
// while carries the counts a one-line summary reads out.
func TestAQuietSessionIsSummarised(t *testing.T) {
	app := newWorkspaceApp(t)
	recipe, key := openFeedProject(t, app, "KapiMart")
	engine := agentProcess(t, app)

	first := agentSuggest(t, engine, recipe, "sess-1", "sign in", "log in", "docs/a.md", "Sign in.")
	agentSuggest(t, engine, recipe, "sess-1", "utilise", "use", "docs/b.md", "Utilise.")
	_, err := app.KeepContextSuggestion(ContextDecisionRequest{Project: key, ID: first.ID})
	require.NoError(t, err)

	feed, err := app.ContextFeed(key, 0)
	require.NoError(t, err)
	var session ContextFeedGroup
	for _, group := range feed.Groups {
		if group.Session == "sess-1" {
			session = group
		}
	}
	require.Equal(t, "sess-1", session.Session)
	assert.Equal(t, 2, session.Recorded)
	assert.Equal(t, 1, session.Awaiting)
	assert.False(t, session.Quiet, "a session recorded a moment ago is still running")
	assert.NotEmpty(t, session.First)
	assert.NotEmpty(t, session.Last)
}

// findFeedEntry reads one entry out of a feed by id.
func findFeedEntry(t *testing.T, feed *ContextFeed, id string) ContextFeedEntry {
	t.Helper()
	for _, group := range feed.Groups {
		for _, entry := range group.Entries {
			if entry.ID == id {
				return entry
			}
		}
	}
	t.Fatalf("the feed holds no operation %s", id)
	return ContextFeedEntry{}
}
