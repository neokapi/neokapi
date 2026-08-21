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
// (and the cwd-relative voice store) resolve against the test project.
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
	srcPath := filepath.Join(root, project.RelStatePath("terms.json"))
	require.FileExists(t, srcPath)

	proj, err := project.Load(recipe)
	require.NoError(t, err)
	require.Equal(t, project.RelStatePath("terms.json"), proj.Defaults.TermsSource)

	data, err := os.ReadFile(srcPath)
	require.NoError(t, err)
	file, err := ktb.Unmarshal(data)
	require.NoError(t, err)
	require.Len(t, file.Concepts, 1)
	require.Len(t, file.Concepts[0].Terms, 1)
	assert.Equal(t, "sign in", file.Concepts[0].Terms[0].Text)
	assert.Equal(t, model.TermPreferred, file.Concepts[0].Terms[0].Status)

	// 2. The project store's vocabulary was compiled from the source. There is
	// no second file to point at any more, so the assertion is on the store the
	// App already holds — the same one every term-aware command reads.
	require.FileExists(t, project.LayoutAt(root).StorePath())
	db, err := a.ProjectDB(ctx, root)
	require.NoError(t, err)
	n, err := db.Terms().Count(ctx)
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

	srcPath := filepath.Join(root, project.RelStatePath(project.MemoryDirName, "memory.json"))
	require.FileExists(t, srcPath)

	proj, err := project.Load(recipe)
	require.NoError(t, err)
	require.Equal(t, project.RelStatePath(project.MemoryDirName, "memory.json"), proj.Defaults.MemorySource)

	// Compiled into the project store, which now holds the pair.
	require.FileExists(t, project.LayoutAt(root).StorePath())
	db, err := a.ProjectDB(ctx, root)
	require.NoError(t, err)
	got := lookupMemoryTarget(t, ctx, db.Memory(), "Welcome back", "en", "fr")
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

func TestApplyVoiceEntry_writesProfileCompilesStore(t *testing.T) {
	a, cmd, root, recipe := newApplyAssetProject(t)
	ctx := context.Background()

	e := changeEntry{
		Kind:        kindVoice,
		Op:          "add-rule",
		List:        "forbidden",
		Term:        "utilize",
		Replacement: "use",
		Severity:    "minor",
	}

	res := a.applyAssetEntry(ctx, cmd, e)
	require.Equal(t, "applied", res.Status, "detail: %s", res.Detail)

	profilePath := filepath.Join(root, project.RelStatePath("voice.yaml"))
	require.FileExists(t, profilePath)

	proj, err := project.Load(recipe)
	require.NoError(t, err)
	require.NotNil(t, proj.Defaults.Voice)
	assert.Equal(t, project.RelStatePath("voice.yaml"), proj.Defaults.Voice.ProfileFile)

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

// TestUpsertTerm_JoinsTheConceptItAnswers pins where a term decision lands. The
// answer to "what should this say instead" is the preferred term of the concept
// the retired word sits in, so a decision filed on its own island can never be
// answered however clearly it was written — which is what made every replacement
// handed to `kapi apply` invisible to the gate.
func TestUpsertTerm_JoinsTheConceptItAnswers(t *testing.T) {
	berth := func() []terms.Concept {
		return []terms.Concept{{
			ID: "c-berth",
			Terms: []terms.Term{
				{Text: "berth", Locale: "en-GB", Status: model.TermPreferred},
				{Text: "mooring", Locale: "en-GB", Status: model.TermDeprecated, Note: terms.ReplacementNote("berth")},
			},
		}}
	}

	tests := []struct {
		name        string
		concepts    []terms.Concept
		decision    termDecision
		wantChanged bool
		wantConcept string   // the concept the term landed in
		wantTerms   []string // its terms, as "text/status"
	}{
		{
			name:     "a replacement the graph declares joins its concept",
			concepts: berth(),
			decision: termDecision{
				Text: "dock", Locale: "en-GB", Status: model.TermForbidden, Replacement: "berth",
			},
			wantChanged: true,
			wantConcept: "c-berth",
			wantTerms:   []string{"berth/preferred", "mooring/deprecated", "dock/forbidden"},
		},
		{
			name:     "replaces names the concept to join, by id",
			concepts: berth(),
			decision: termDecision{
				Text: "quay", Locale: "en-GB", Status: model.TermDeprecated, Replaces: "c-berth",
			},
			wantChanged: true,
			wantConcept: "c-berth",
			wantTerms:   []string{"berth/preferred", "mooring/deprecated", "quay/deprecated"},
		},
		{
			name:     "replaces names the concept to join, by one of its terms",
			concepts: berth(),
			decision: termDecision{
				Text: "quay", Locale: "en-GB", Status: model.TermDeprecated, Replaces: "mooring",
			},
			wantChanged: true,
			wantConcept: "c-berth",
			wantTerms:   []string{"berth/preferred", "mooring/deprecated", "quay/deprecated"},
		},
		{
			name:     "a replacement the graph has never heard of becomes the preferred sibling",
			concepts: nil,
			decision: termDecision{
				Text: "dock", Locale: "en-GB", Status: model.TermForbidden, Replacement: "berth",
			},
			wantChanged: true,
			wantConcept: "term:en-GB:dock",
			wantTerms:   []string{"dock/forbidden", "berth/preferred"},
		},
		{
			name:     "a term the entry declares preferred is retired in favour of nothing",
			concepts: nil,
			decision: termDecision{
				Text: "sign in", Locale: "en", Status: model.TermPreferred, Replacement: "log in",
			},
			wantChanged: true,
			wantConcept: "term:en:sign-in",
			wantTerms:   []string{"sign in/preferred"},
		},
		{
			name:     "a decision already recorded changes nothing",
			concepts: berth(),
			decision: termDecision{
				Text: "mooring", Locale: "en-GB", Status: model.TermDeprecated, Replacement: "berth",
			},
			wantChanged: false,
			wantConcept: "c-berth",
			wantTerms:   []string{"berth/preferred", "mooring/deprecated"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := upsertTerm(tt.concepts, tt.decision)
			assert.Equal(t, tt.wantChanged, changed)

			idx := -1
			for i := range got {
				if got[i].ID == tt.wantConcept {
					idx = i
				}
			}
			require.GreaterOrEqual(t, idx, 0, "the term must land in %s (got %+v)", tt.wantConcept, got)

			var have []string
			for _, term := range got[idx].Terms {
				have = append(have, term.Text+"/"+string(term.Status))
			}
			assert.Equal(t, tt.wantTerms, have)

			// Re-applying the same decision is a no-op, whichever way it landed.
			_, again := upsertTerm(got, tt.decision)
			assert.False(t, again, "apply must be idempotent")
		})
	}
}

// TestUpsertTerm_ReplacementIsNeverDeclaredTwice: a word one concept already
// declares is not copied into a second, which would leave the graph with two
// concepts for one concept.
func TestUpsertTerm_ReplacementIsNeverDeclaredTwice(t *testing.T) {
	concepts := []terms.Concept{
		{ID: "c-berth", Terms: []terms.Term{{Text: "berth", Locale: "en-GB", Status: model.TermPreferred}}},
		{ID: "c-dock", Terms: []terms.Term{{Text: "dock", Locale: "en-GB", Status: model.TermProposed}}},
	}

	got, changed := upsertTerm(concepts, termDecision{
		Text: "dock", Locale: "en-GB", Status: model.TermForbidden, Replacement: "berth",
	})
	require.True(t, changed)

	seen := 0
	for _, c := range got {
		for _, term := range c.Terms {
			if term.Text == "berth" {
				seen++
			}
		}
	}
	assert.Equal(t, 1, seen, "the replacement stays in the one concept that declares it")
	assert.Equal(t, terms.ReplacementNote("berth"), got[1].Terms[0].Note,
		"and the decision is still recorded on the term it was made about")
}

func TestApplyRecipeEntry_setDefaultCoordinate(t *testing.T) {
	a, cmd, _, recipe := newApplyAssetProject(t)
	ctx := context.Background()

	set := func(axis string, value any) assetResult {
		val, err := json.Marshal(value)
		require.NoError(t, err)
		return a.applyAssetEntry(ctx, cmd, changeEntry{
			Kind: kindRecipe, Op: "set", Path: "defaults.coordinates." + axis, Value: val,
		})
	}

	res := set(project.BrandAxis, "acme")
	require.Equal(t, "applied", res.Status, "detail: %s", res.Detail)

	proj, err := project.Load(recipe)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{project.BrandAxis: "acme"}, proj.Defaults.Coordinates)

	// A second axis joins the point rather than replacing it.
	require.Equal(t, "applied", set("market", "emea").Status)
	proj, err = project.Load(recipe)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{project.BrandAxis: "acme", "market": "emea"}, proj.Defaults.Coordinates)

	// Idempotent.
	assert.Equal(t, "skipped", set(project.BrandAxis, "acme").Status)

	// An empty value withdraws the axis, so a change-set can retract a
	// coordinate as well as declare one.
	require.Equal(t, "applied", set("market", "").Status)
	proj, err = project.Load(recipe)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{project.BrandAxis: "acme"}, proj.Defaults.Coordinates)
	assert.Equal(t, "skipped", set("market", "").Status, "withdrawing an absent axis changes nothing")
}

// product and channel are computed from a collection's `channel:`. Declaring
// one as a default would state a point the recipe also derives, and the two
// would be free to disagree — so apply refuses them rather than letting a
// change-set author a contradiction.
func TestApplyRecipeEntry_refusesDerivedAxes(t *testing.T) {
	a, cmd, _, _ := newApplyAssetProject(t)
	ctx := context.Background()

	for _, axis := range []string{project.ProductAxis, project.ChannelAxis} {
		val, err := json.Marshal("anything")
		require.NoError(t, err)
		res := a.applyAssetEntry(ctx, cmd, changeEntry{
			Kind: kindRecipe, Op: "set", Path: "defaults.coordinates." + axis, Value: val,
		})
		assert.Equal(t, "error", res.Status, "axis %q must be refused", axis)
		assert.Contains(t, res.Detail, "derived")
	}

	// The whole map is not settable at once: one axis per entry, so a
	// change-set says which axis it is changing.
	val, err := json.Marshal(map[string]string{project.BrandAxis: "acme"})
	require.NoError(t, err)
	res := a.applyAssetEntry(ctx, cmd, changeEntry{
		Kind: kindRecipe, Op: "set", Path: "defaults.coordinates", Value: val,
	})
	assert.Equal(t, "error", res.Status)
	assert.Contains(t, res.Detail, "one axis at a time")

	// An empty axis is a precise error rather than a silent no-op.
	res = a.applyAssetEntry(ctx, cmd, changeEntry{
		Kind: kindRecipe, Op: "set", Path: "defaults.coordinates.", Value: val,
	})
	assert.Equal(t, "error", res.Status)
	assert.Contains(t, res.Detail, "empty axis")
}
