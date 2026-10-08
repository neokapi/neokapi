package host

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/workspace"
)

// The context of a team: two people on two machines of one project, sharing
// through a backend or a transfer file. What one machine decided holds the
// same way on the other.

// failingFiles names the files a check fails on the word under test.
func failingFiles(report check.Report) map[string]bool {
	out := map[string]bool{}
	for _, d := range vocabularyFindings(report) {
		if d.Fails {
			out[filepath.Base(d.Location.File)] = true
		}
	}
	return out
}

// TestAWidenedRuleReachesEveryMachine: a rule a person widened to the workspace
// travels with the project, by a backend or by a transfer file, and the second
// machine applies it in every project of its own workspace, the way the first
// machine does.
func TestAWidenedRuleReachesEveryMachine(t *testing.T) {
	tests := []struct {
		name      string
		transport func(t *testing.T, from *App, fromRecipe string, to *App, toRecipe string)
	}{
		{
			name: "pushed and pulled",
			transport: func(t *testing.T, from *App, fromRecipe string, to *App, toRecipe string) {
				shared := t.TempDir()
				_, err := from.SyncProjectContextWith(t.Context(), fromRecipe, workspace.NewFileRemote(shared), false, true)
				require.NoError(t, err)
				_, err = to.SyncProjectContextWith(t.Context(), toRecipe, workspace.NewFileRemote(shared), true, false)
				require.NoError(t, err)
			},
		},
		{
			name: "exported and imported",
			transport: func(t *testing.T, from *App, fromRecipe string, to *App, toRecipe string) {
				file := filepath.Join(t.TempDir(), "context.kpz")
				_, err := from.ExportProjectContext(t.Context(), fromRecipe, file)
				require.NoError(t, err)
				_, err = to.ImportContextFile(t.Context(), toRecipe, file)
				require.NoError(t, err)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			first := contextOpsProject(t, "ctxteam-widened")
			second := t.TempDir()
			require.NoError(t, os.CopyFS(second, os.DirFS(first)))
			appA, _ := contextOpsApp(t)
			appB, _ := contextOpsApp(t)
			elsewhere := contextOpsProject(t, "ctxteam-elsewhere")

			kept := proposeUtilise(t, appA, first, person)
			_, err := appA.KeepContextOperation(ctx, ContextKeepRequest{Actor: person, Project: recipeOf(first), ID: kept.ID})
			require.NoError(t, err)
			_, err = appA.WidenContextOperation(ctx, ContextWidenRequest{Actor: person, Project: recipeOf(first), ID: kept.ID, To: WidenToWorkspace})
			require.NoError(t, err)
			require.True(t, failingFiles(checkWith(t, appA, first))["app.yaml"])

			tt.transport(t, appA, recipeOf(first), appB, recipeOf(second))

			assert.True(t, failingFiles(checkWith(t, appB, second))["app.yaml"],
				"the second machine fails the check the first one fails")
			assert.True(t, failingFiles(checkWith(t, appB, elsewhere))["app.yaml"],
				"and applies the rule in every project of its own workspace")
		})
	}
}

// channelProject is a project whose one profile ships two channels: the app's
// strings and the documentation. Both use the word a rule will be about.
func channelProject(t *testing.T, name string) string {
	t.Helper()
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		path := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
		require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	}
	write("kapi.yaml", `version: v1
id: `+projectIDFor(name)+`
name: `+name+`
defaults:
  source_language: en
profiles:
  quickcast:
    channels: [app, docs]
collections:
  - name: app-strings
    channel: quickcast/app
    source_only: true
    content:
      - path: "app/*.md"
  - name: guide
    channel: quickcast/docs
    source_only: true
    content:
      - path: "docs/*.md"
`)
	write("app/strings.md", "We utilise the widget.\n")
	write("docs/guide.md", "We utilise the widget.\n")
	return root
}

// TestRuleScope_AKeptRuleHoldsAtItsChannel: a rule kept from evidence in the
// app's strings fails the app's strings and says nothing about the same
// product's documentation, until a person widens it past the point it was
// seen at.
func TestRuleScope_AKeptRuleHoldsAtItsChannel(t *testing.T) {
	tests := []struct {
		name     string
		widenTo  string
		wantDocs bool
	}{
		{name: "kept where it was seen"},
		{name: "widened past the channel", widenTo: "channel", wantDocs: true},
		// Past the product the rule still sits at the app channel.
		{name: "widened past the product", widenTo: "product"},
		{name: "widened to the project", widenTo: WidenToProject, wantDocs: true},
		{name: "widened to the workspace", widenTo: WidenToWorkspace, wantDocs: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, _ := contextOpsApp(t)
			root := channelProject(t, "ctxteam-channel")
			observed, err := app.RecordContextObservation(t.Context(), ContextObserveRequest{
				Actor: person, Project: recipeOf(root), Term: "use", InsteadOf: []string{"utilise"},
				Evidence: []contextop.Evidence{{Path: "app/strings.md"}},
			})
			require.NoError(t, err)
			assert.Equal(t, map[string]string{"product": "quickcast", "channel": "app"}, observed.Scope.Coordinates)
			_, err = app.KeepContextOperation(t.Context(), ContextKeepRequest{Actor: person, Project: recipeOf(root), ID: observed.ID})
			require.NoError(t, err)

			if tt.widenTo != "" {
				_, err = app.WidenContextOperation(t.Context(), ContextWidenRequest{
					Actor: person, Project: recipeOf(root), ID: observed.ID, To: tt.widenTo,
				})
				require.NoError(t, err)
			}

			failing := failingFiles(checkWith(t, app, root))
			assert.True(t, failing["strings.md"], "the rule holds where its evidence was seen")
			assert.Equal(t, tt.wantDocs, failing["guide.md"])
		})
	}
}

// TestWideningToTheWorkspaceKeepsAnAxisNoProfileDerives: a brand the recipe
// declares stays on a rule widened to the workspace; the product and channel
// the profile derived go.
func TestWideningToTheWorkspaceKeepsAnAxisNoProfileDerives(t *testing.T) {
	target := contextop.Record{
		Basis: contextop.Basis{Profile: "quickcast"},
		Scope: contextop.Scope{Level: contextop.LevelProject, Coordinates: map[string]string{
			"brand": "acme", "product": "quickcast", "channel": "app",
		}},
	}
	tests := []struct {
		to   string
		want contextop.Scope
	}{
		{to: "channel", want: contextop.Scope{Level: contextop.LevelProject, Coordinates: map[string]string{"brand": "acme", "product": "quickcast"}}},
		{to: WidenToProject, want: contextop.Scope{Level: contextop.LevelProject, AllProfiles: true, Coordinates: map[string]string{"brand": "acme"}}},
		{to: WidenToWorkspace, want: contextop.Scope{Level: contextop.LevelWorkspace, Coordinates: map[string]string{"brand": "acme"}}},
	}
	for _, tt := range tests {
		t.Run(tt.to, func(t *testing.T) {
			got, err := widenScope(target, tt.to)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestKeepingAContestedCorrectionWaitsForAChoice: a person's correction that
// reverses an established rule and the rule contest each other. Keeping the
// correction is refused with the rule named, and choosing it reverts the rule.
func TestKeepingAContestedCorrectionWaitsForAChoice(t *testing.T) {
	ctx := t.Context()
	app, _ := contextOpsApp(t)
	root := contextOpsProject(t, "ctxteam-contested")
	rule := proposeUtilise(t, app, root, person)
	_, err := app.KeepContextOperation(ctx, ContextKeepRequest{Actor: person, Project: recipeOf(root), ID: rule.ID})
	require.NoError(t, err)

	teammate := contextop.Actor{Kind: contextop.ActorPerson, Name: "bob@example.com"}
	corrected, err := app.RecordContextCorrection(ctx, ContextCorrectRequest{
		Actor: teammate, Project: recipeOf(root), From: "use", To: "utilise",
		Evidence: []contextop.Evidence{{Path: "config/app.yaml", Unit: "greeting"}},
	})
	require.NoError(t, err)
	assert.Equal(t, contextop.StatusContested, corrected.Status)
	assert.Equal(t, []string{rule.ID}, corrected.ContestedBy, "the correction names the rule it disagrees with")

	_, err = app.KeepContextOperation(ctx, ContextKeepRequest{Actor: person, Project: recipeOf(root), ID: corrected.ID})
	require.Error(t, err)
	assert.Contains(t, err.Error(), contextop.ShortID(rule.ID), "the refusal names the other side")
	assert.Contains(t, err.Error(), "--choose")

	chosen, err := app.ChooseContextSide(ctx, ContextChooseRequest{Actor: person, Project: recipeOf(root), ID: corrected.ID})
	require.NoError(t, err)
	require.Len(t, chosen.SetAside, 1)
	assert.Equal(t, rule.ID, chosen.SetAside[0].ID)

	log, err := app.ContextOperations(ctx, ContextLogRequest{Project: recipeOf(root), Subjects: true})
	require.NoError(t, err)
	status := map[string]contextop.Status{}
	for _, op := range log.Operations {
		status[op.ID] = op.Status
	}
	assert.Equal(t, contextop.StatusReverted, status[rule.ID])
	assert.Equal(t, contextop.StatusEstablished, status[corrected.ID])
}

// TestTheDigestNamesTeammatesAndCorrections: what the reader did reads as
// theirs, a teammate's operation carries the teammate's name, and a correction
// with no rule of its own is shown as the change it records.
func TestTheDigestNamesTeammatesAndCorrections(t *testing.T) {
	alice := contextop.Actor{Kind: contextop.ActorPerson, Name: "alice@example.com"}
	bob := contextop.Actor{Kind: contextop.ActorPerson, Name: "bob@example.com"}
	records := []contextop.Record{
		{ID: "a", Actor: alice, Kind: contextop.KindObserve},
		{ID: "b", Actor: bob, Kind: contextop.KindCorrect},
		{ID: "c", Actor: contextop.Actor{Kind: contextop.ActorAgent, Name: "claude"}, Kind: contextop.KindObserve},
	}
	seen := seenBy(records, alice)
	assert.Equal(t, "you", actorPhrase(seen[0].Actor, false))
	assert.Equal(t, "bob@example.com", actorPhrase(seen[1].Actor, false))
	assert.Equal(t, "claude", actorPhrase(seen[2].Actor, false))
	assert.Equal(t, "alice@example.com", records[0].Actor.Name, "the log's records are left as they are")

	it := digestItem(contextop.Record{
		ID: "0pjbe70rjq7nex78wk6g1wqa", Kind: contextop.KindCorrect, Actor: bob,
		Correction: &contextop.Correction{From: "Sign in", To: "Log in"},
	}, nil)
	assert.Equal(t, `Changed "Sign in" to "Log in".`, it.Sentence)
}

// TestPersonName: a person is recorded under the email their version control
// is configured with, the name when no email is set, and unnamed otherwise.
func TestPersonName(t *testing.T) {
	tests := []struct {
		name   string
		config map[string]string
		want   string
	}{
		{name: "email and name", config: map[string]string{"user.email": "alice@example.com\n", "user.name": "Alice A\n"}, want: "alice@example.com"},
		{name: "name only", config: map[string]string{"user.name": "Alice A\n"}, want: "Alice A"},
		{name: "nothing configured", config: map[string]string{}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, PersonName(func(key string) string { return tt.config[key] }))
		})
	}
}

// TestContextAnswersRefuseAnUnreadableRecipe: a recipe that does not load is
// an error for the context answer and for the search, never an answer that
// says nothing is recorded.
func TestContextAnswersRefuseAnUnreadableRecipe(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := channelProject(t, "ctxteam-broken")
	recipe := recipeOf(root)
	body, err := os.ReadFile(recipe)
	require.NoError(t, err)
	broken := strings.Replace(string(body), "channel: quickcast/app", "channel: app", 1)
	require.NoError(t, os.WriteFile(recipe, []byte(broken), 0o600))

	src, done := app.ContextSourcesAt(bindingsCmd(t, recipe), ContextPointRequest{Path: "docs/guide.md"})
	defer done()
	_, err = ResolveContextAt(t.Context(), src, ContextPointRequest{Path: "docs/guide.md"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "recipe could not be read")

	searchSrc, cleanup := app.ContextSearchSourcesFor(bindingsCmd(t, recipe), "", "")
	defer cleanup()
	_, err = SearchContext(t.Context(), searchSrc, ContextSearchRequest{Query: "utilise"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "recipe could not be read")
}

// TestContextSearchFindsAWidenedRule: a rule widened to the workspace answers
// a search the way the project's own terms do, marked as holding across the
// workspace.
func TestContextSearchFindsAWidenedRule(t *testing.T) {
	ctx := t.Context()
	app, _ := contextOpsApp(t)
	root := contextOpsProject(t, "ctxteam-search")
	kept := proposeUtilise(t, app, root, person)
	_, err := app.KeepContextOperation(ctx, ContextKeepRequest{Actor: person, Project: recipeOf(root), ID: kept.ID})
	require.NoError(t, err)
	_, err = app.WidenContextOperation(ctx, ContextWidenRequest{Actor: person, Project: recipeOf(root), ID: kept.ID, To: WidenToWorkspace})
	require.NoError(t, err)

	src, cleanup := app.ContextSearchSourcesFor(bindingsCmd(t, recipeOf(root)), "", "")
	defer cleanup()
	res, err := SearchContext(ctx, src, ContextSearchRequest{Query: "utilise"})
	require.NoError(t, err)
	var hit *ContextTermHit
	for i := range res.Terms {
		if res.Terms[i].Term == "utilise" {
			hit = &res.Terms[i]
		}
	}
	require.NotNil(t, hit, "the widened rule answers the search")
	assert.True(t, hit.Discouraged)
	assert.Equal(t, "use", hit.Replacement)
	assert.Equal(t, string(contextop.LevelWorkspace), hit.Scope)
	assert.Equal(t, kept.ID, hit.Operation)
	assert.NotEqual(t, CoverageEmpty, res.Coverage)
}
