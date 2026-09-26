package host

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/kpz"
)

// Making a project's context portable. `kapi context import <dir>` reads the
// context files a person wrote into operations, and changes nothing the second
// time; `kapi context export` writes the whole log to one transfer file, and
// `kapi context import <file>.kpz` merges it into another machine's log, which
// then holds the same stores.

const portableVoiceYAML = `name: Portable Voice
version: 1
tone:
  formality: neutral
terms:
  - term: utilize
    replacement: use
    advisory: true
`

const portableProfileVoiceYAML = `name: Portable Landing Voice
version: 1
tone:
  formality: casual
`

const portableTermsJSON = `{
  "schemaVersion": "1.0",
  "kind": "kapi-terms",
  "concepts": [
    {
      "id": "c-widget",
      "domain": "product",
      "definition": "The thing the product is about.",
      "terms": [
        { "text": "widget", "locale": "en", "status": "approved" },
        { "text": "dings", "locale": "nb", "status": "approved" }
      ]
    }
  ]
}
`

// writePortableProject builds a project whose whole context sits in `.kapi/`:
// a bound voice profile, a per-profile voice profile, a bound terms bundle and
// a source document with a translated twin the record absorb learns from.
func writePortableProject(t *testing.T) (a *App, root, recipe string) {
	t.Helper()
	root = t.TempDir()
	return newPortableApp(t, root), root, writePortableTree(t, root)
}

// writePortableTree lays the project out under root and returns its recipe.
func writePortableTree(t *testing.T, root string) string {
	t.Helper()
	layout := project.LayoutAt(root)
	require.NoError(t, os.MkdirAll(layout.StateDir, 0o755))
	require.NoError(t, os.MkdirAll(layout.Export().ProfileDir("landing"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "locales", "en"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(root, "locales", "nb"), 0o755))

	recipe := `version: v1
name: portable
defaults:
  source_language: en
  target_languages: [nb]
profiles:
  landing:
    channels: [web]
collections:
  - name: app
    path: "locales/en/*.json"
    target: "locales/{lang}/*.json"
`
	write := func(rel, content string) {
		path := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
	write(project.RecipeFileName, recipe)
	write(".kapi/voice.yaml", portableVoiceYAML)
	write(".kapi/profiles/landing/voice.yaml", portableProfileVoiceYAML)
	write(".kapi/terms.json", portableTermsJSON)
	write("locales/en/app.json", "{\n  \"greeting\": \"Hello there\",\n  \"farewell\": \"Goodbye\"\n}\n")
	write("locales/nb/app.json", "{\n  \"greeting\": \"Hei der\",\n  \"farewell\": \"Ha det\"\n}\n")
	return filepath.Join(root, project.RecipeFileName)
}

// newPortableApp returns an App whose project stores are closed when the test
// ends, so a second App over the same tree opens the file rather than a handle
// the first one still holds.
func newPortableApp(t *testing.T, root string) *App {
	t.Helper()
	a := &App{SourceLang: "en"}
	a.InitRegistries()
	t.Cleanup(a.Shutdown)
	_ = root
	return a
}

// seedPortable compiles the project's committed context and records one
// decision, so the store holds something of every kind a transfer carries.
//
// The decision is recorded before the seeding pass, the order a real project
// meets them in: a review approves wording, and the next run compiles the
// committed context and absorbs the translations that approval blessed. The
// pass carries the decision's governing context onto the pair it learns, so the
// store an export is taken from is the one a run leaves behind.
func seedPortable(t *testing.T, a *App, root, recipe string) {
	t.Helper()
	ctx := context.Background()

	st, err := a.OpenProjectState(ctx, root)
	require.NoError(t, err)
	scope := a.DocumentScope(ctx, root, filepath.Join(root, "locales", "en", "app.json"))
	require.NoError(t, st.Put(ctx, state.UnitState{
		Unit: "greeting", Variant: model.Variant("nb"), Scope: scope,
		Status:               model.TargetStatusEstablished,
		Decision:             state.Decision{ReviewState: "approved", By: "reviewer"},
		TargetHash:           state.TargetHash("Hei der"),
		ContentHash:          state.SourceHash("Hello there"),
		GoverningFingerprint: "fp-portable",
	}))

	_, err = a.seedContext(ctx, recipe)
	require.NoError(t, err)
}

func TestImportProjectContext_IsIdempotent(t *testing.T) {
	a, root, recipe := writePortableProject(t)
	seedPortable(t, a, root, recipe)
	ctx := context.Background()
	db, err := a.ProjectDB(ctx, root)
	require.NoError(t, err)

	first, err := a.ImportProjectContext(ctx, recipe, ContextImportRequest{Force: true})
	require.NoError(t, err)
	require.True(t, first.Read())
	afterFirst := projectionRows(t, a, db)

	second, err := a.ImportProjectContext(ctx, recipe, ContextImportRequest{Force: true})
	require.NoError(t, err)
	assert.Equal(t, first.Concepts, second.Concepts)
	assert.Equal(t, first.VoiceProfiles, second.VoiceProfiles)
	assert.Equal(t, afterFirst, projectionRows(t, a, db), "a second read leaves the store saying the same thing")

	// A third read without --force reads nothing at all: this checkout has
	// already read these sources at these bytes, and the store says so.
	third, err := a.ImportProjectContext(ctx, recipe, ContextImportRequest{})
	require.NoError(t, err)
	assert.False(t, third.Read(), "a source at bytes this checkout has read is skipped")
	assert.Positive(t, third.Unchanged)
	assert.Equal(t, afterFirst, projectionRows(t, a, db), "and the store still says the same thing")
}

func TestImportProjectContext_ReadsAnotherCheckoutsLayout(t *testing.T) {
	_, donor, _ := writePortableProject(t)

	clone := t.TempDir()
	cloneRecipe := writePortableTree(t, clone)
	require.NoError(t, os.RemoveAll(project.LayoutAt(clone).StateDir))
	require.NoError(t, os.MkdirAll(project.LayoutAt(clone).StateDir, 0o755))
	b := newPortableApp(t, clone)

	res, err := b.ImportProjectContext(context.Background(), cloneRecipe,
		ContextImportRequest{Dir: project.LayoutAt(donor).StateDir})
	require.NoError(t, err)
	assert.Positive(t, res.Concepts, "the donor's terms bundle comes across")
	assert.Equal(t, 2, res.VoiceProfiles)
}

// TestContextFile_CarriesTheStoresToAnotherMachine exports a seeded project,
// imports the file into a second checkout with its own workspace, and finds
// the same stores there: terms, voice profiles, approved wording and the
// decision ledger. Reading the file again merges nothing.
func TestContextFile_CarriesTheStoresToAnotherMachine(t *testing.T) {
	a, root, recipe := writePortableProject(t)
	seedPortable(t, a, root, recipe)
	ctx := context.Background()
	_, err := a.ImportProjectContext(ctx, recipe, ContextImportRequest{Force: true})
	require.NoError(t, err)

	out := filepath.Join(t.TempDir(), "context.kpz")
	exported, err := a.ExportProjectContext(ctx, recipe, out)
	require.NoError(t, err)
	assert.Positive(t, exported.Operations)
	assert.Positive(t, exported.Segments)
	assert.NotEmpty(t, exported.Checkpoint, "a transfer file carries a checkpoint for a fast first read")
	pkg, closer, err := kpz.OpenFile(out)
	require.NoError(t, err)
	require.NoError(t, closer.Close())
	assert.Equal(t, kpz.KindContext, pkg.Kind)

	clone := t.TempDir()
	cloneRecipe := writePortableTree(t, clone)
	b := newPortableApp(t, clone)
	imported, err := b.ImportContextFile(ctx, cloneRecipe, out)
	require.NoError(t, err)
	assert.Equal(t, exported.Operations, imported.Merged)
	assert.Equal(t, exported.Checkpoint, imported.Checkpoint, "a first read starts from the checkpoint")

	dbA, err := a.ProjectDB(ctx, root)
	require.NoError(t, err)
	dbB, err := b.ProjectDB(ctx, clone)
	require.NoError(t, err)
	rowsA := projectionRows(t, a, dbA)
	require.NotEmpty(t, rowsA["unit_decision"], "the decision ledger travels")
	require.NotEmpty(t, rowsA["tb_concepts"])
	rowsB := projectionRows(t, b, dbB)
	// Which decision each checkout pairs with its own files is that checkout's
	// view, and a checkout that has read no content pairs none.
	delete(rowsA, "unit_view")
	delete(rowsB, "unit_view")
	assert.Equal(t, rowsA, rowsB, "the second machine holds the same stores")

	again, err := b.ImportContextFile(ctx, cloneRecipe, out)
	require.NoError(t, err)
	assert.Zero(t, again.Merged, "reading the same file twice merges nothing")
}

// TestContextFile_RefusesAnotherProjectsFile names both projects, so a person
// who picked the wrong file or the wrong checkout knows which.
func TestContextFile_RefusesAnotherProjectsFile(t *testing.T) {
	a, root, recipe := writePortableProject(t)
	seedPortable(t, a, root, recipe)
	out := filepath.Join(t.TempDir(), "context.kpz")
	_, err := a.ExportProjectContext(context.Background(), recipe, out)
	require.NoError(t, err)

	other := contextOpsProject(t, "ctxfile-other")
	b := newPortableApp(t, other)
	_, err = b.ImportContextFile(context.Background(), recipeOf(other), out)
	require.ErrorContains(t, err, "holds the context of project portable")
}

// TestContextFile_RefusesAPackageOfAnotherKind: a project package is not a
// context file, and one from before the log carries no operations.
func TestContextFile_RefusesAPackageOfAnotherKind(t *testing.T) {
	_, root, recipe := writePortableProject(t)
	b := newPortableApp(t, root)
	dir := t.TempDir()

	project := filepath.Join(dir, "project.kpz")
	data, err := (&kpz.Package{Kind: kpz.KindProject, Media: nil}).Marshal()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(project, data, 0o644))
	_, err = b.ImportContextFile(context.Background(), recipe, project)
	require.ErrorContains(t, err, "is a kapi-project package")

	old := filepath.Join(dir, "old.kpz")
	data, err = (&kpz.Package{Kind: kpz.KindContext}).Marshal()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(old, data, 0o644))
	_, err = b.ImportContextFile(context.Background(), recipe, old)
	require.ErrorContains(t, err, "export it again with this version")
}
