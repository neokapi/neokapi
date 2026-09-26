package host

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/terms"
)

// scopedProject is a project with two profiles: acme governs docs/, relaunch
// governs landing/. Both trees use the word a rule will be about.
func scopedProject(t *testing.T, name string) string {
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
  acme:
    channels: [docs]
  relaunch:
    channels: [landing]
collections:
  - name: acme-docs
    channel: acme/docs
    source_only: true
    content:
      - path: "docs/*.md"
  - name: relaunch-landing
    channel: relaunch/landing
    source_only: true
    content:
      - path: "landing/*.md"
`)
	write("docs/a.md", "We utilise the widget.\n")
	write("landing/b.md", "We utilise the widget.\n")
	return root
}

// TestRuleScope_AKeptRuleHoldsAtItsProfile: a rule whose evidence was seen
// where one profile governs lands scoped to that profile, and a check fails on
// it there and nowhere else.
func TestRuleScope_AKeptRuleHoldsAtItsProfile(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := scopedProject(t, "ctxops-scope")

	observed, err := app.RecordContextObservation(t.Context(), ContextObserveRequest{
		Actor: person, Project: recipeOf(root), Term: "use", InsteadOf: []string{"utilise"},
		Evidence: []contextop.Evidence{{Path: "docs/a.md"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "acme", observed.Basis.Profile)
	_, err = app.KeepContextOperation(t.Context(), ContextKeepRequest{Actor: person, Project: recipeOf(root), ID: observed.ID})
	require.NoError(t, err)

	report := checkWith(t, app, root)
	failing := map[string]bool{}
	for _, d := range vocabularyFindings(report) {
		if d.Fails {
			failing[filepath.Base(d.Location.File)] = true
		}
	}
	assert.True(t, failing["a.md"], "the rule holds where its evidence was seen")
	assert.False(t, failing["b.md"], "and not where another profile governs")
}

// TestRuleScope_AProfileFileScopesWhatItCarries: the context files under a
// profile's directory are read as that profile's, and a concept written through
// them carries the profile.
func TestRuleScope_AProfileFileScopesWhatItCarries(t *testing.T) {
	root := scopedProject(t, "ctxops-scope-files")
	layout, err := project.LayoutFor(recipeOf(root))
	require.NoError(t, err)
	dir := layout.Export().ProfileDir("relaunch")
	require.NoError(t, os.MkdirAll(dir, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(dir, VoiceConventionalName), []byte("name: relaunch\n"), 0o600))

	sources, err := committedContextSources(layout)
	require.NoError(t, err)
	var found bool
	for _, src := range sources {
		if strings.HasPrefix(src.rel, ".kapi/profiles/relaunch/") {
			found = true
			assert.Equal(t, "relaunch", src.profile)
		}
	}
	require.True(t, found)

	mem := terms.NewInMemoryStore()
	require.NoError(t, scopedTerms(mem, "relaunch").AddConcept(t.Context(), terms.Concept{ID: "c1"}))
	held, err := mem.Concepts(t.Context())
	require.NoError(t, err)
	require.Len(t, held, 1)
	assert.Equal(t, "relaunch", held[0].Profile())
	assert.Empty(t, terms.AtProfile(held, "acme"))
	assert.Len(t, terms.AtProfile(held, "relaunch"), 1)
}

// TestRuleScope_WideningToTheProjectClearsTheProfile: a person widens a rule
// settled under one profile to the whole project, and a check then fails on it
// under every profile. Widening it again to the project is refused.
func TestRuleScope_WideningToTheProjectClearsTheProfile(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := scopedProject(t, "ctxops-scope-widen")

	observed, err := app.RecordContextObservation(t.Context(), ContextObserveRequest{
		Actor: person, Project: recipeOf(root), Term: "use", InsteadOf: []string{"utilise"},
		Evidence: []contextop.Evidence{{Path: "docs/a.md"}},
	})
	require.NoError(t, err)
	_, err = app.KeepContextOperation(t.Context(), ContextKeepRequest{Actor: person, Project: recipeOf(root), ID: observed.ID})
	require.NoError(t, err)

	_, err = app.WidenContextOperation(t.Context(), ContextWidenRequest{
		Actor: agentIn("s1"), Project: recipeOf(root), ID: observed.ID, To: WidenToProject,
	})
	require.ErrorIs(t, err, contextop.ErrRefused, "only a person widens a rule")

	widened, err := app.WidenContextOperation(t.Context(), ContextWidenRequest{
		Actor: person, Project: recipeOf(root), ID: observed.ID, To: WidenToProject,
	})
	require.NoError(t, err)
	assert.True(t, widened.Scope.AllProfiles)
	assert.NotEqual(t, contextop.LevelWorkspace, widened.Scope.Level)

	report := checkWith(t, app, root)
	failing := map[string]bool{}
	for _, d := range vocabularyFindings(report) {
		if d.Fails {
			failing[filepath.Base(d.Location.File)] = true
		}
	}
	assert.True(t, failing["a.md"])
	assert.True(t, failing["b.md"], "the rule now holds under the other profile too")

	_, err = app.WidenContextOperation(t.Context(), ContextWidenRequest{
		Actor: person, Project: recipeOf(root), ID: observed.ID, To: WidenToProject,
	})
	require.Error(t, err, "a rule that holds across the project is not widened to it again")
}
