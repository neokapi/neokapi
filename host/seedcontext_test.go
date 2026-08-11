package host

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/memory/kmb"
	"github.com/neokapi/neokapi/terms"
	"github.com/neokapi/neokapi/terms/ktb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newSeedProject writes a recipe binding a committed terms source and returns
// the app, project root and recipe path. Nothing is compiled yet: the store
// starts as empty as it is on a fresh clone.
func newSeedProject(t *testing.T, bindTermsSource bool) (a *App, root, recipe string) {
	t.Helper()
	root = t.TempDir()
	recipe = filepath.Join(root, project.RecipeFileName)
	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "SeedTest",
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"nb"},
		},
	}
	if bindTermsSource {
		proj.Defaults.TermsSource = project.RelStatePath(ktb.ConventionalName)
	}
	require.NoError(t, project.Save(recipe, proj))
	require.NoError(t, os.MkdirAll(filepath.Join(root, project.StateDirName), 0o755))

	a = &App{}
	a.InitRegistries()
	return a, root, recipe
}

// writeTermsSource writes a committed terms bundle holding one concept per
// (text, translation) pair.
func writeTermsSource(t *testing.T, root string, pairs map[string]string) {
	t.Helper()
	var concepts []terms.Concept
	stamp := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for en, nb := range pairs {
		concepts = append(concepts, terms.Concept{
			ID:     "term:" + en,
			Source: terms.TermSourceTerminology,
			Terms: []terms.Term{
				{Text: en, Locale: "en", Status: model.TermPreferred},
				{Text: nb, Locale: "nb", Status: model.TermPreferred},
			},
			CreatedAt: stamp,
			UpdatedAt: stamp,
		})
	}
	data, err := ktb.Marshal(ktb.FromConcepts(concepts))
	require.NoError(t, err)
	path := filepath.Join(root, project.RelStatePath(ktb.ConventionalName))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, data, 0o644))
}

// writeMemoryBundle writes one committed content-memory bundle under
// .kapi/memory/, named for the surface it backs.
func writeMemoryBundle(t *testing.T, root, name string, pairs map[string]string) {
	t.Helper()
	var entries []memory.Entry
	stamp := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for src, tgt := range pairs {
		entries = append(entries, memory.Entry{
			ID:          name + ":" + src,
			HintSrcLang: "en",
			Variants: map[model.LocaleID][]model.Run{
				"en": {{Text: &model.TextRun{Text: src}}},
				"nb": {{Text: &model.TextRun{Text: tgt}}},
			},
			CreatedAt: stamp,
			UpdatedAt: stamp,
		})
	}
	data, err := kmb.Marshal(kmb.FromModel(entries, nil))
	require.NoError(t, err)
	dir := project.LayoutAt(root).MemoryDir()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, name+".memory.json"), data, 0o644))
}

// storeCounts reports how many concepts and content-memory entries the project
// store holds.
func storeCounts(t *testing.T, a *App, root string) (concepts, entries int) {
	t.Helper()
	ctx := context.Background()
	db, err := a.ProjectDB(ctx, root)
	require.NoError(t, err)
	if tb := db.Terms(); tb != nil {
		concepts, err = tb.Count(ctx)
		require.NoError(t, err)
	}
	if tm := db.Memory(); tm != nil {
		entries, err = tm.Count(ctx)
		require.NoError(t, err)
	}
	return concepts, entries
}

func TestSeedProjectContext(t *testing.T) {
	tests := []struct {
		name string
		// setup writes the committed sources; the recipe binds terms_source
		// when bindTerms is set.
		bindTerms    bool
		setup        func(t *testing.T, root string)
		wantConcepts int
		wantEntries  int
		wantTerms    int // compiled terms bundles
		wantMemory   int // compiled memory bundles
	}{
		{
			name:      "no committed sources compiles nothing",
			bindTerms: false,
			setup:     func(*testing.T, string) {},
		},
		{
			name:      "bound terms source compiles into the store",
			bindTerms: true,
			setup: func(t *testing.T, root string) {
				writeTermsSource(t, root, map[string]string{"content memory": "innholdsminne"})
			},
			wantConcepts: 1,
			wantTerms:    1,
		},
		{
			name:      "every committed bundle under the memory directory compiles",
			bindTerms: false,
			setup: func(t *testing.T, root string) {
				writeMemoryBundle(t, root, "docs-nb", map[string]string{"Hello": "Hei"})
				writeMemoryBundle(t, root, "cli-nb", map[string]string{"Goodbye": "Ha det"})
			},
			wantEntries: 2,
			wantMemory:  2,
		},
		{
			name:      "terms and memory seed together",
			bindTerms: true,
			setup: func(t *testing.T, root string) {
				writeTermsSource(t, root, map[string]string{"terms": "termer"})
				writeMemoryBundle(t, root, "docs-nb", map[string]string{"Hello": "Hei"})
			},
			wantConcepts: 1,
			wantEntries:  1,
			wantTerms:    1,
			wantMemory:   1,
		},
		{
			name:      "a bound source that does not exist yet is not an error",
			bindTerms: true,
			setup:     func(*testing.T, string) {},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a, root, recipe := newSeedProject(t, tc.bindTerms)
			tc.setup(t, root)

			res, err := a.SeedProjectContext(context.Background(), recipe)
			require.NoError(t, err)
			assert.Equal(t, tc.wantTerms, res.TermsFiles)
			assert.Equal(t, tc.wantMemory, res.MemoryFiles)
			assert.Equal(t, tc.wantConcepts, res.Concepts)
			assert.Equal(t, tc.wantEntries, res.Entries)

			concepts, entries := storeCounts(t, a, root)
			assert.Equal(t, tc.wantConcepts, concepts, "concepts in the store")
			assert.Equal(t, tc.wantEntries, entries, "content-memory entries in the store")
		})
	}
}

// TestSeedProjectContext_DigestKeyed: an unchanged source recompiles nothing,
// and an edited one recompiles exactly itself. This is what keeps `kapi up`
// from re-importing the whole committed context on every pass.
func TestSeedProjectContext_DigestKeyed(t *testing.T) {
	a, root, recipe := newSeedProject(t, true)
	writeTermsSource(t, root, map[string]string{"content memory": "innholdsminne"})
	writeMemoryBundle(t, root, "docs-nb", map[string]string{"Hello": "Hei"})
	ctx := context.Background()

	first, err := a.SeedProjectContext(ctx, recipe)
	require.NoError(t, err)
	require.Equal(t, 1, first.TermsFiles)
	require.Equal(t, 1, first.MemoryFiles)
	require.Equal(t, 0, first.Skipped)

	second, err := a.SeedProjectContext(ctx, recipe)
	require.NoError(t, err)
	assert.False(t, second.Compiled(), "an unchanged source recompiles nothing")
	assert.Equal(t, 2, second.Skipped)

	// A pulled edit to the committed terms source reaches the store.
	writeTermsSource(t, root, map[string]string{
		"content memory": "innholdsminne",
		"voice profile":  "stemmeprofil",
	})
	third, err := a.SeedProjectContext(ctx, recipe)
	require.NoError(t, err)
	assert.Equal(t, 1, third.TermsFiles, "the edited terms source recompiled")
	assert.Equal(t, 1, third.Skipped, "the untouched memory bundle did not")

	concepts, _ := storeCounts(t, a, root)
	assert.Equal(t, 2, concepts)
}

// TestSeedProjectContext_ForgetsDeletedSources: a bundle removed from git stops
// being remembered, so the stamp map does not accumulate one entry per surface
// the project ever had and a re-added bundle compiles rather than reading as
// already-seeded.
func TestSeedProjectContext_ForgetsDeletedSources(t *testing.T) {
	a, root, recipe := newSeedProject(t, false)
	writeMemoryBundle(t, root, "docs-nb", map[string]string{"Hello": "Hei"})
	writeMemoryBundle(t, root, "cli-nb", map[string]string{"Goodbye": "Ha det"})
	ctx := context.Background()

	first, err := a.SeedProjectContext(ctx, recipe)
	require.NoError(t, err)
	require.Equal(t, 2, first.MemoryFiles)

	bundle := filepath.Join(project.LayoutAt(root).MemoryDir(), "docs-nb.memory.json")
	require.NoError(t, os.Remove(bundle))
	gone, err := a.SeedProjectContext(ctx, recipe)
	require.NoError(t, err)
	require.Equal(t, 1, gone.Skipped, "only the surviving bundle was considered")

	writeMemoryBundle(t, root, "docs-nb", map[string]string{"Hello": "Hei"})
	back, err := a.SeedProjectContext(ctx, recipe)
	require.NoError(t, err)
	assert.Equal(t, 1, back.MemoryFiles, "the re-added bundle compiled again")
	assert.Equal(t, 1, back.Skipped, "the untouched bundle did not")
}

// TestSeedProjectContext_KeepsServerVocabulary: seeding is an upsert of what git
// carries, never a replace — terminology a venue pull wrote into the store
// survives a later compile of the committed source.
func TestSeedProjectContext_KeepsServerVocabulary(t *testing.T) {
	a, root, recipe := newSeedProject(t, true)
	writeTermsSource(t, root, map[string]string{"content memory": "innholdsminne"})
	ctx := context.Background()

	db, err := a.ProjectDB(ctx, root)
	require.NoError(t, err)
	require.NoError(t, db.Terms().AddConcept(ctx, terms.Concept{
		ID:     "concept:from-the-server",
		Source: terms.TermSourceTerminology,
		Terms:  []terms.Term{{Text: "workspace", Locale: "en", Status: model.TermPreferred}},
	}))

	_, err = a.SeedProjectContext(ctx, recipe)
	require.NoError(t, err)

	_, found, err := db.Terms().GetConcept(ctx, "concept:from-the-server")
	require.NoError(t, err)
	assert.True(t, found, "the pulled concept survived the compile")
	concepts, _ := storeCounts(t, a, root)
	assert.Equal(t, 2, concepts, "the store is the union of the source and the pull")
}

// TestSeedProjectContext_ApplyStampsWhatItCompiled: a write path that edits a
// committed source and imports it itself records the digest, so the next
// seeding pass does not re-import a source the store already holds.
func TestSeedProjectContext_ApplyStampsWhatItCompiled(t *testing.T) {
	a, cmd, root, recipe := newApplyAssetProject(t)
	ctx := context.Background()

	res := a.applyAssetEntry(ctx, cmd, changeEntry{
		Kind: kindTerm, Op: "upsert", Term: "sign in", Locale: "en", Status: "preferred",
	})
	require.Equal(t, "applied", res.Status, "detail: %s", res.Detail)

	seeded, err := a.SeedProjectContext(ctx, recipe)
	require.NoError(t, err)
	assert.False(t, seeded.Compiled(), "apply's own compile was recorded")
	assert.Equal(t, 1, seeded.Skipped)

	concepts, _ := storeCounts(t, a, root)
	assert.Equal(t, 1, concepts)
}

// TestSeedProjectContext_RelationsRoundTrip: the committed bundle is the terms
// store's lossless form, so the edges it carries reach the store too.
func TestSeedProjectContext_RelationsRoundTrip(t *testing.T) {
	a, root, recipe := newSeedProject(t, true)
	ctx := context.Background()

	stamp := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	file := ktb.FromConcepts([]terms.Concept{
		{ID: "term:old", Terms: []terms.Term{{Text: "termbase", Locale: "en", Status: model.TermForbidden}}, CreatedAt: stamp, UpdatedAt: stamp},
		{ID: "term:new", Terms: []terms.Term{{Text: "terms", Locale: "en", Status: model.TermPreferred}}, CreatedAt: stamp, UpdatedAt: stamp},
	})
	file.Relations = []terms.ConceptRelation{{
		ID:           "rel:old-new",
		SourceID:     "term:old",
		TargetID:     "term:new",
		RelationType: "REPLACED_BY",
		CreatedAt:    stamp,
	}}
	data, err := ktb.Marshal(file)
	require.NoError(t, err)
	path := filepath.Join(root, project.RelStatePath(ktb.ConventionalName))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, data, 0o644))

	_, err = a.SeedProjectContext(ctx, recipe)
	require.NoError(t, err)

	db, err := a.ProjectDB(ctx, root)
	require.NoError(t, err)
	rels, err := db.Terms().ListRelations(ctx, nil)
	require.NoError(t, err)
	require.Len(t, rels, 1)
	assert.Equal(t, "rel:old-new", rels[0].ID)
}
