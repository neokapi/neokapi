package host

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/project"
)

// Two checkouts of one project share one context store, and the store is the
// only thing either of them reads context from. These tests hold the three
// consequences that used to fail (#2917, #2918, #2919): a file in one checkout
// reaches nothing until someone reads it in, a read in either checkout is in
// force in both, and the gate and the retrieval surfaces answer alike.

// twoCheckoutProject builds two checkouts of one project on one workspace, the
// way two git worktrees sit on one machine. The recipe is identical in both,
// which is what makes the workspace key them to one context store.
type twoCheckoutProject struct {
	a           *App
	aRoot       string
	aRecipe     string
	b           *App
	bRoot       string
	bRecipe     string
	workspaceAt string
}

const twoCheckoutRecipe = `version: v1
name: two-checkouts
id: prj_twocheckoutsaaaaaaaaaaaaaaaaaaaa
defaults:
  source_language: en
  target_languages: [nb]
  terms_source: .kapi/terms.json
  voice:
    profile_file: .kapi/voice.yaml
collections:
  - name: docs
    path: "docs/*.md"
    target: "docs/{lang}/*.md"
`

func newTwoCheckoutProject(t *testing.T) *twoCheckoutProject {
	t.Helper()
	workspace := t.TempDir()

	build := func() (*App, string, string) {
		root := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(root, project.StateDirName), 0o755))
		recipe := filepath.Join(root, project.RecipeFileName)
		require.NoError(t, os.WriteFile(recipe, []byte(twoCheckoutRecipe), 0o644))
		app := &App{}
		app.InitRegistries()
		app.SetWorkspaceRoot(workspace)
		t.Cleanup(app.Shutdown)
		return app, root, recipe
	}

	c := &twoCheckoutProject{workspaceAt: workspace}
	c.a, c.aRoot, c.aRecipe = build()
	c.b, c.bRoot, c.bRecipe = build()
	return c
}

// conceptTexts is the source-language text of every concept the gate would
// enforce in a checkout — the resolution `kapi check` runs.
func conceptTexts(t *testing.T, app *App, recipe string) []string {
	t.Helper()
	cmd := newProjectCmd(t, recipe)
	concepts, err := app.projectConcepts(cmd, project.GovernancePoint{})
	require.NoError(t, err)
	var out []string
	for _, c := range concepts {
		for _, term := range c.Terms {
			if term.Locale == "en" {
				out = append(out, term.Text)
			}
		}
	}
	return out
}

// newProjectCmd is a command bound to one project, with the flags a terms
// resolution reads.
func newProjectCmd(t *testing.T, recipe string) Command {
	t.Helper()
	cmd := NewEnvCommand(context.Background(), "test")
	cmd.Flags().String("project", recipe, "")
	cmd.Flags().String("termstore", "", "")
	return cmd
}

// TestTwoCheckouts_ATermsFileEnforcesNothingUntilItIsRead is #2917: a checkout
// carrying a terms bundle used to compile it into the shared store on the next
// run, and the other checkout's gate then enforced terms its branch never
// declared. Nothing reads a bundle now, so neither checkout enforces anything.
func TestTwoCheckouts_ATermsFileEnforcesNothingUntilItIsRead(t *testing.T) {
	c := newTwoCheckoutProject(t)
	writeTermsSource(t, c.aRoot, map[string]string{"widget": "dings"})

	// Opening A's store, which is what any command there does first.
	_, err := c.a.ProjectDB(context.Background(), c.aRoot)
	require.NoError(t, err)

	assert.Empty(t, conceptTexts(t, c.a, c.aRecipe),
		"the checkout holding the bundle enforces nothing from it")
	assert.Empty(t, conceptTexts(t, c.b, c.bRecipe),
		"and the checkout without one enforces nothing either")

	// Reading it in is a decision about the PROJECT, so it is in force in both.
	_, err = c.a.ImportProjectContext(context.Background(), c.aRecipe, ContextImportRequest{})
	require.NoError(t, err)

	assert.Equal(t, []string{"widget"}, conceptTexts(t, c.a, c.aRecipe))
	assert.Equal(t, []string{"widget"}, conceptTexts(t, c.b, c.bRecipe),
		"one store, one answer, whichever checkout asks")
}

// TestTwoCheckouts_RetrievalAnswersWhatTheGateEnforces is #2919: the gate read
// the committed bundle while `kapi terms lookup` and `kapi context search` read
// the store, so a person and an agent asking one project one question got two
// answers. Both read the store.
func TestTwoCheckouts_RetrievalAnswersWhatTheGateEnforces(t *testing.T) {
	c := newTwoCheckoutProject(t)
	ctx := context.Background()

	// A bundle sitting in B's tree that nobody has read in, and a concept the
	// project has: the two cases that used to disagree.
	writeTermsSource(t, c.bRoot, map[string]string{"sidebar": "sidefelt"})
	writeTermsSource(t, c.aRoot, map[string]string{"widget": "dings"})
	_, err := c.a.ImportProjectContext(ctx, c.aRecipe, ContextImportRequest{})
	require.NoError(t, err)

	for _, checkout := range []struct {
		name   string
		app    *App
		recipe string
	}{
		{"the checkout that read the bundle in", c.a, c.aRecipe},
		{"the checkout carrying an unread one", c.b, c.bRecipe},
	} {
		t.Run(checkout.name, func(t *testing.T) {
			gate := conceptTexts(t, checkout.app, checkout.recipe)
			assert.Equal(t, []string{"widget"}, gate, "the gate enforces what the store holds")

			src, release := checkout.app.ContextSearchSourcesFor(newProjectCmd(t, checkout.recipe), "", "")
			defer release()
			require.NotNil(t, src.Terms)
			res, err := SearchContext(ctx, src, ContextSearchRequest{Query: "widget"})
			require.NoError(t, err)
			require.NotEmpty(t, res.Terms, "retrieval answers the same store")
			assert.Equal(t, "widget", res.Terms[0].Term)

			sidebar, err := SearchContext(ctx, src, ContextSearchRequest{Query: "sidebar"})
			require.NoError(t, err)
			assert.Empty(t, sidebar.Terms, "an unread bundle answers nowhere")
		})
	}
}

// TestTwoCheckouts_BothSeeTheProfileTheStoreHolds is the read half of #2918:
// the profile a recipe binds by path used to be loaded from whichever copy the
// checkout's branch carried, so two checkouts governed by one profile could be
// held to two different voices. Both read the one the store holds.
func TestTwoCheckouts_BothSeeTheProfileTheStoreHolds(t *testing.T) {
	c := newTwoCheckoutProject(t)
	ctx := context.Background()

	writeVoiceProfile(t, c.aRoot, "name: Audit Voice\nversion: 1\ntone:\n  formality: neutral\n")
	writeVoiceProfile(t, c.bRoot, "name: Audit Voice\nversion: 1\ntone:\n  formality: casual\n")

	_, err := c.a.ImportProjectContext(ctx, c.aRecipe, ContextImportRequest{})
	require.NoError(t, err)

	for _, checkout := range []struct {
		name   string
		app    *App
		root   string
		recipe string
	}{
		{"the checkout the profile was read from", c.a, c.aRoot, c.aRecipe},
		{"the checkout carrying a different copy", c.b, c.bRoot, c.bRecipe},
	} {
		t.Run(checkout.name, func(t *testing.T) {
			proj, err := project.Load(checkout.recipe)
			require.NoError(t, err)
			store, release, err := checkout.app.ProjectVoiceStore(ctx, checkout.root)
			require.NoError(t, err)
			defer release()

			profile, _, found, err := checkout.app.ResolveVoiceProfile(ctx, proj, checkout.root,
				VoiceResolveOptions{Store: store})
			require.NoError(t, err)
			require.True(t, found, "the binding resolves against the store")
			assert.Equal(t, "neutral", profile.Tone.Formality,
				"the profile the store holds, not the file in this tree")
		})
	}
}

// TestContextFilesUnread_NamesTheFilesAndTheCommand: a project whose recipe
// binds a terms source and whose store is empty gets told what it is carrying
// and what reads it, rather than an answer or a failure.
func TestContextFilesUnread_NamesTheFilesAndTheCommand(t *testing.T) {
	c := newTwoCheckoutProject(t)
	ctx := context.Background()
	writeTermsSource(t, c.aRoot, map[string]string{"widget": "dings"})
	writeVoiceProfile(t, c.aRoot, "name: Audit Voice\nversion: 1\n")

	notice, unread := c.a.ContextFilesUnread(ctx, c.aRecipe)
	require.True(t, unread, "a store that has never held context is unread")
	assert.Equal(t, []string{".kapi/terms.json", ".kapi/voice.yaml"}, notice.Files)
	assert.Equal(t, ContextImportCommand, notice.Command)
	assert.Contains(t, notice.Message(), "kapi context import")

	// The recipe still loads: a binding nobody has read in is a notice, never a
	// broken checkout.
	_, err := project.Load(c.aRecipe)
	require.NoError(t, err)
	assert.Empty(t, conceptTexts(t, c.a, c.aRecipe), "and it answers with nothing")

	_, err = c.a.ImportProjectContext(ctx, c.aRecipe, ContextImportRequest{})
	require.NoError(t, err)
	_, unread = c.a.ContextFilesUnread(ctx, c.aRecipe)
	assert.False(t, unread, "a store that holds context is not unread")
}

// TestContextFilesUnread_IsSilentWithoutFiles: a project carrying no context
// file has nothing to be told about, however empty its store.
func TestContextFilesUnread_IsSilentWithoutFiles(t *testing.T) {
	c := newTwoCheckoutProject(t)
	_, unread := c.b.ContextFilesUnread(context.Background(), c.bRecipe)
	assert.False(t, unread)
}

// writeVoiceProfile writes the project's conventional voice profile.
func writeVoiceProfile(t *testing.T, root, yaml string) {
	t.Helper()
	path := filepath.Join(root, project.RelStatePath(VoiceConventionalName))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0o644))
}
