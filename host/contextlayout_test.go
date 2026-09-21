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
// the app, project root and recipe path. Nothing is read yet: the store starts
// as empty as it is on a fresh clone.
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

// writeTermsSource writes a terms bundle holding one concept per
// (text, translation) pair, at the conventional path.
func writeTermsSource(t *testing.T, root string, pairs map[string]string) {
	t.Helper()
	writeTermsBundleAt(t, filepath.Join(root, project.RelStatePath(ktb.ConventionalName)), pairs)
}

// writeTermsBundleAt writes a terms bundle at path.
func writeTermsBundleAt(t *testing.T, path string, pairs map[string]string) {
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
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, data, 0o644))
}

// writeMemoryBundle writes one content-memory bundle under .kapi/memory/, named
// for the surface it backs.
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
	dir := project.LayoutAt(root).Export().MemoryDir()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, name+".memory.json"), data, 0o644))
}

// seedResult is what one pass over a project's own files puts into its store.
type seedResult struct {
	Concepts      int
	Entries       int
	VoiceProfiles int
	// Record is what the committed translations taught the content memory.
	Record RecordAbsorbResult
}

// Compiled reports whether the pass has anything to say.
func (r seedResult) Compiled() bool {
	return r.Concepts > 0 || r.Entries > 0 || r.VoiceProfiles > 0 ||
		r.Record.Absorbed() || r.Record.Superseded > 0
}

// seedContext reads a project's context layout into its store and absorbs its
// committed translations: the two halves a converge run runs on the way in,
// which many of these tests need together.
func (a *App) seedContext(ctx context.Context, recipe string) (seedResult, error) {
	read, err := a.ImportProjectContext(ctx, recipe, ContextImportRequest{})
	if err != nil {
		return seedResult{}, err
	}
	record, err := a.AbsorbProjectRecord(ctx, recipe)
	if err != nil {
		return seedResult{}, err
	}
	return seedResult{
		Concepts:      read.Concepts,
		Entries:       read.Entries,
		VoiceProfiles: read.VoiceProfiles,
		Record:        record,
	}, nil
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

// TestImportProjectContext_ReadsTheLayout: what `kapi context import` puts into
// the store for each shape of `.kapi/` layout.
func TestImportProjectContext_ReadsTheLayout(t *testing.T) {
	tests := []struct {
		name string
		// setup writes the context files; the recipe binds terms_source when
		// bindTerms is set.
		bindTerms    bool
		setup        func(t *testing.T, root string)
		wantConcepts int
		wantEntries  int
	}{
		{
			name:      "no context files reads nothing",
			bindTerms: false,
			setup:     func(*testing.T, string) {},
		},
		{
			name:      "a bound terms source reads into the store",
			bindTerms: true,
			setup: func(t *testing.T, root string) {
				writeTermsSource(t, root, map[string]string{"content memory": "innholdsminne"})
			},
			wantConcepts: 1,
		},
		{
			name:      "every bundle under the memory directory is read",
			bindTerms: false,
			setup: func(t *testing.T, root string) {
				writeMemoryBundle(t, root, "docs-nb", map[string]string{"Hello": "Hei"})
				writeMemoryBundle(t, root, "cli-nb", map[string]string{"Goodbye": "Ha det"})
			},
			wantEntries: 2,
		},
		{
			name:      "terms and memory are read together",
			bindTerms: true,
			setup: func(t *testing.T, root string) {
				writeTermsSource(t, root, map[string]string{"terms": "termer"})
				writeMemoryBundle(t, root, "docs-nb", map[string]string{"Hello": "Hei"})
			},
			wantConcepts: 1,
			wantEntries:  1,
		},
		{
			name:      "a bound source that does not exist is not an error",
			bindTerms: true,
			setup:     func(*testing.T, string) {},
		},
		{
			// A profile keeps its own vocabulary under its directory, and a
			// read that skipped it would leave those terms in the checkout
			// with nothing to bring them in.
			name:      "a profile's own terms are read",
			bindTerms: false,
			setup: func(t *testing.T, root string) {
				writeTermsBundleAt(t,
					filepath.Join(project.LayoutAt(root).Export().ProfileDir("landing"), ktb.ConventionalName),
					map[string]string{"sign in": "logg inn"})
			},
			wantConcepts: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a, root, recipe := newSeedProject(t, tc.bindTerms)
			tc.setup(t, root)

			res, err := a.ImportProjectContext(context.Background(), recipe, ContextImportRequest{})
			require.NoError(t, err)
			assert.Equal(t, tc.wantConcepts, res.Concepts)
			assert.Equal(t, tc.wantEntries, res.Entries)

			concepts, entries := storeCounts(t, a, root)
			assert.Equal(t, tc.wantConcepts, concepts, "concepts in the store")
			assert.Equal(t, tc.wantEntries, entries, "content-memory entries in the store")
		})
	}
}

// TestImportProjectContext_KeepsServerVocabulary: an import is an upsert of what
// the files carry, never a replace — terminology a venue pull wrote into the
// store survives a later read of a bundle.
func TestImportProjectContext_KeepsServerVocabulary(t *testing.T) {
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

	_, err = a.ImportProjectContext(ctx, recipe, ContextImportRequest{})
	require.NoError(t, err)

	_, found, err := db.Terms().GetConcept(ctx, "concept:from-the-server")
	require.NoError(t, err)
	assert.True(t, found, "the pulled concept survived the read")
	concepts, _ := storeCounts(t, a, root)
	assert.Equal(t, 2, concepts, "the store is the union of the bundle and the pull")
}

// TestImportProjectContext_RelationsRoundTrip: the bundle is the terms store's
// lossless form, so the edges it carries reach the store too.
func TestImportProjectContext_RelationsRoundTrip(t *testing.T) {
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

	_, err = a.ImportProjectContext(ctx, recipe, ContextImportRequest{})
	require.NoError(t, err)

	db, err := a.ProjectDB(ctx, root)
	require.NoError(t, err)
	rels, err := db.Terms().ListRelations(ctx, nil)
	require.NoError(t, err)
	require.Len(t, rels, 1)
	assert.Equal(t, "rel:old-new", rels[0].ID)
}

// TestImportProjectContext_NothingIsReadWithoutTheCommand: a project whose
// checkout carries every context file answers with none of it until the import
// runs. This is the whole of the store-only contract: two checkouts of one
// project cannot decide each other's terms by what their branches happen to
// hold.
func TestImportProjectContext_NothingIsReadWithoutTheCommand(t *testing.T) {
	a, root, recipe := newSeedProject(t, true)
	writeTermsSource(t, root, map[string]string{"widget": "dings"})
	writeMemoryBundle(t, root, "docs-nb", map[string]string{"Hello": "Hei"})
	ctx := context.Background()

	concepts, entries := storeCounts(t, a, root)
	assert.Zero(t, concepts, "opening the store reads no terms bundle")
	assert.Zero(t, entries, "opening the store reads no content-memory bundle")

	_, err := a.ImportProjectContext(ctx, recipe, ContextImportRequest{})
	require.NoError(t, err)

	concepts, entries = storeCounts(t, a, root)
	assert.Equal(t, 1, concepts)
	assert.Equal(t, 1, entries)
}
