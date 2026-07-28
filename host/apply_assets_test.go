package host

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/terms"
	"github.com/neokapi/neokapi/terms/ktb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newApplyAssetProject writes a minimal .kapi recipe + state dir under dir and
// returns the recipe path. It chdirs into dir so apply's project auto-discovery
// (and the cwd-relative brand store) resolve against the test project.
func newApplyAssetProject(t *testing.T) (a *App, cmd *EnvCommand, root, recipe string) {
	t.Helper()
	dir := t.TempDir()
	recipe = filepath.Join(dir, project.RecipeFileName)
	proj := &project.KapiProject{
		Version: "v1",
		Name:    "ApplyAssetTest",
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"fr"},
		},
	}
	require.NoError(t, project.Save(recipe, proj))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, project.StateDirName), 0o755))
	t.Chdir(dir)

	a = &App{}
	a.InitRegistries()
	cmd = NewEnvCommand(context.Background(), "apply")
	return a, cmd, dir, recipe
}

func TestApplyTermEntry_writesSourceCompilesCacheIdempotent(t *testing.T) {
	a, cmd, root, recipe := newApplyAssetProject(t)
	ctx := context.Background()

	e := changeEntry{
		Kind:        kindTerm,
		Op:          "upsert",
		Term:        "sign in",
		Locale:      "en",
		Status:      "preferred",
		Replacement: "log in",
	}

	res := a.applyAssetEntry(ctx, cmd, e)
	require.Equal(t, "applied", res.Status, "detail: %s", res.Detail)

	// 1. The committed .terms.json source was written and bound in the recipe.
	srcPath := filepath.Join(root, "l10n", "terms.json")
	require.FileExists(t, srcPath)

	proj, err := project.Load(recipe)
	require.NoError(t, err)
	require.Equal(t, filepath.Join("l10n", "terms.json"), proj.Defaults.TermsSource)
	require.NotEmpty(t, proj.Defaults.Terms, "the compiled cache should be bound too")

	data, err := os.ReadFile(srcPath)
	require.NoError(t, err)
	file, err := ktb.Unmarshal(data)
	require.NoError(t, err)
	require.Len(t, file.Concepts, 1)
	require.Len(t, file.Concepts[0].Terms, 1)
	assert.Equal(t, "sign in", file.Concepts[0].Terms[0].Text)
	assert.Equal(t, model.TermPreferred, file.Concepts[0].Terms[0].Status)

	// 2. The cache (.kapi/terms.db) was compiled from the source.
	dbPath := filepath.Join(root, project.StateDirName, "terms.db")
	require.FileExists(t, dbPath)
	tb, err := terms.NewSQLiteStore(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { tb.Close() })
	n, err := tb.Count(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	// 3. Idempotent re-run → skipped, no rewrite.
	before, err := os.ReadFile(srcPath)
	require.NoError(t, err)
	res2 := a.applyAssetEntry(ctx, cmd, e)
	assert.Equal(t, "skipped", res2.Status)
	after, err := os.ReadFile(srcPath)
	require.NoError(t, err)
	assert.Equal(t, before, after, "skipped re-run must not rewrite the source")

	// 4. A second, different term upserts → applied, two concepts.
	res3 := a.applyAssetEntry(ctx, cmd, changeEntry{
		Kind: kindTerm, Op: "upsert", Term: "dashboard", Locale: "en", Status: "preferred",
	})
	require.Equal(t, "applied", res3.Status, "detail: %s", res3.Detail)
	data, err = os.ReadFile(srcPath)
	require.NoError(t, err)
	file, err = ktb.Unmarshal(data)
	require.NoError(t, err)
	assert.Len(t, file.Concepts, 2)
}

func TestApplyMemoryEntry_writesSourceCompilesCacheIdempotent(t *testing.T) {
	a, cmd, root, recipe := newApplyAssetProject(t)
	ctx := context.Background()

	e := changeEntry{
		Kind:         kindMemory,
		Op:           "add",
		Source:       "Welcome back",
		Target:       "Bon retour",
		SourceLocale: "en",
		TargetLocale: "fr",
	}

	res := a.applyAssetEntry(ctx, cmd, e)
	require.Equal(t, "applied", res.Status, "detail: %s", res.Detail)

	srcPath := filepath.Join(root, "l10n", "memory.json")
	require.FileExists(t, srcPath)

	proj, err := project.Load(recipe)
	require.NoError(t, err)
	require.Equal(t, filepath.Join("l10n", "memory.json"), proj.Defaults.MemorySource)

	// Cache compiled, contains the pair.
	dbPath := filepath.Join(root, project.StateDirName, "memory.db")
	require.FileExists(t, dbPath)
	tm, err := memory.NewSQLiteStore(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { tm.Close() })
	got := lookupMemoryTarget(t, ctx, tm, "Welcome back", "en", "fr")
	assert.Equal(t, "Bon retour", got)

	// Idempotent.
	res2 := a.applyAssetEntry(ctx, cmd, e)
	assert.Equal(t, "skipped", res2.Status)
}

func TestApplyMemoryEntry_reviewStatus(t *testing.T) {
	a, cmd, _, _ := newApplyAssetProject(t)
	ctx := context.Background()

	base := changeEntry{Kind: kindMemory, Source: "Save", Target: "Enregistrer", SourceLocale: "en", TargetLocale: "fr"}

	// A signed-off status is accepted and applied.
	so := base
	so.Status = "signed-off"
	res := a.applyAssetEntry(ctx, cmd, so)
	assert.Equal(t, "applied", res.Status, "detail: %s", res.Detail)

	// An unknown review status is rejected.
	bad := changeEntry{Kind: kindMemory, Source: "Cancel", Target: "Annuler", SourceLocale: "en", TargetLocale: "fr", Status: "bogus"}
	res2 := a.applyAssetEntry(ctx, cmd, bad)
	assert.Equal(t, "error", res2.Status)
	assert.Contains(t, res2.Detail, "status")
}

func TestApplyBrandEntry_writesProfileCompilesStore(t *testing.T) {
	a, cmd, root, recipe := newApplyAssetProject(t)
	ctx := context.Background()

	e := changeEntry{
		Kind:        kindBrand,
		Op:          "add-rule",
		List:        "forbidden",
		Term:        "utilize",
		Replacement: "use",
		Severity:    "minor",
	}

	res := a.applyAssetEntry(ctx, cmd, e)
	require.Equal(t, "applied", res.Status, "detail: %s", res.Detail)

	profilePath := filepath.Join(root, "l10n", "brand-voice.yaml")
	require.FileExists(t, profilePath)

	proj, err := project.Load(recipe)
	require.NoError(t, err)
	require.NotNil(t, proj.Defaults.BrandVoice)
	assert.Equal(t, filepath.Join("l10n", "brand-voice.yaml"), proj.Defaults.BrandVoice.ProfileFile)

	// Idempotent re-run.
	res2 := a.applyAssetEntry(ctx, cmd, e)
	assert.Equal(t, "skipped", res2.Status)
}

func TestApplyRecipeEntry_setTargetLanguages(t *testing.T) {
	a, cmd, _, recipe := newApplyAssetProject(t)
	ctx := context.Background()

	val, _ := json.Marshal([]string{"de", "ja"})
	res := a.applyAssetEntry(ctx, cmd, changeEntry{
		Kind:  kindRecipe,
		Op:    "set",
		Path:  "defaults.target_languages",
		Value: val,
	})
	require.Equal(t, "applied", res.Status, "detail: %s", res.Detail)

	proj, err := project.Load(recipe)
	require.NoError(t, err)
	assert.Equal(t, []model.LocaleID{"de", "ja"}, proj.Defaults.TargetLanguages)

	// Idempotent.
	res2 := a.applyAssetEntry(ctx, cmd, changeEntry{
		Kind: kindRecipe, Op: "set", Path: "defaults.target_languages", Value: val,
	})
	assert.Equal(t, "skipped", res2.Status)

	// Unknown path → error.
	res3 := a.applyAssetEntry(ctx, cmd, changeEntry{
		Kind: kindRecipe, Op: "set", Path: "defaults.bogus", Value: val,
	})
	assert.Equal(t, "error", res3.Status)
}

func TestApplyAssetEntry_noProjectIsError(t *testing.T) {
	// A directory with no .kapi recipe and discovery opted out.
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv(noProjectEnvVar, "1")

	a := &App{}
	a.InitRegistries()
	cmd := NewEnvCommand(context.Background(), "apply")

	res := a.applyAssetEntry(context.Background(), cmd, changeEntry{
		Kind: kindTerm, Op: "upsert", Term: "x",
	})
	assert.Equal(t, "error", res.Status)
	assert.Contains(t, res.Detail, "no kapi project")
}

func lookupMemoryTarget(t *testing.T, ctx context.Context, tm memory.Store, text, src, tgt string) string {
	t.Helper()
	matches, err := tm.LookupText(ctx, text, model.LocaleID(src), model.LocaleID(tgt), memory.LookupOptions{MinScore: 0.9, MaxResults: 5})
	require.NoError(t, err)
	require.NotEmpty(t, matches, "expected a content-memory match for %q", text)
	return matches[0].Entry.VariantText(model.LocaleID(tgt))
}
