package server

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/jobs"
	bstore "github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/terms"
)

// editorFakeVoiceStore resolves profiles by ID from a fixed set. Embedding the
// interface means only GetProfile is implemented — the resolution path uses
// nothing else.
type editorFakeVoiceStore struct {
	coreprofile.Store
	profiles map[string]*coreprofile.VoiceProfile
}

func (f *editorFakeVoiceStore) GetProfile(_ context.Context, id string) (*coreprofile.VoiceProfile, error) {
	if p, ok := f.profiles[id]; ok {
		return p, nil
	}
	return nil, errors.New("profile not found: " + id)
}

// editorFakeWorkspaceDefault returns a fixed workspace-level default profile ID.
type editorFakeWorkspaceDefault struct{ id string }

func (f editorFakeWorkspaceDefault) WorkspaceVoiceProfileID(context.Context, string) (string, error) {
	return f.id, nil
}

// editorVoiceContextFixture builds a content store with one project + item +
// block, plus a brand context whose workspace default binds a voice profile and
// whose terms mandates software → logiciel for en→fr.
func editorVoiceContextFixture(t *testing.T) (platstore.ContentStore, editorVoiceContext, *platstore.Project) {
	t.Helper()
	ctx := t.Context()

	cs, err := bstore.NewSQLiteStore(":memory:")
	require.NoError(t, err)

	proj := &platstore.Project{
		ID:                    "p1",
		Name:                  "Proj",
		WorkspaceID:           "ws-1",
		DefaultSourceLanguage: "en",
		TargetLanguages:       []model.LocaleID{"fr"},
	}
	require.NoError(t, cs.CreateProject(ctx, proj))
	require.NoError(t, cs.StoreItem(ctx, "p1", "main", &platstore.Item{Name: "hello.txt", Format: "txt", ItemType: "file"}))

	b := &model.Block{ID: "b1", Translatable: true}
	b.SetSourceText("the software is ready")
	require.NoError(t, cs.StoreBlocksForItem(ctx, "p1", "main", "hello.txt", []*model.Block{b}))

	tb := terms.NewInMemoryStore()
	require.NoError(t, tb.AddConcept(ctx, terms.Concept{
		ID: "c1",
		Terms: []terms.Term{
			{Text: "software", Locale: "en", Status: model.TermPreferred},
			{Text: "logiciel", Locale: "fr", Status: model.TermPreferred},
		},
	}))
	wsStores := newWorkspaceStores()
	wsStores.termsFactory = func() terms.Store { return &testTermStore{tb} }

	profile := &coreprofile.VoiceProfile{
		ID:   "bp-1",
		Name: "Acme Voice",
		Tone: coreprofile.ToneProfile{Formality: "casual", Guidelines: "Address the reader as a peer"},
		Locales: map[model.LocaleID]coreprofile.LocaleOverride{
			"fr": {Formality: "formal"},
		},
	}
	voiceCtx := editorVoiceContext{
		Voice:            &editorFakeVoiceStore{profiles: map[string]*coreprofile.VoiceProfile{"bp-1": profile}},
		WorkspaceDefault: editorFakeWorkspaceDefault{id: "bp-1"},
		Stores:           wsStores,
	}
	return cs, voiceCtx, proj
}

// TestEditorTranslateConfigCarriesVoiceContext proves the interactive editor
// translate config carries the same standing brand context a worker job does:
// the voice profile resolved through the voicescope ladder (with the target
// locale's override applied) and the terms store-derived rules — the exact
// fields the AI translate tool injects into every prompt.
func TestEditorTranslateConfigCarriesVoiceContext(t *testing.T) {
	cs, voiceCtx, proj := editorVoiceContextFixture(t)

	cfg := editorTranslateConfig(t.Context(), cs, voiceCtx, proj,
		"p1", "main", "hello.txt", "ws-1", "acme", TranslateRequest{TargetLocale: "fr", Provider: "demo"})

	require.NotNil(t, cfg.Profile)
	assert.Equal(t, "Acme Voice", cfg.Profile.Name)
	assert.Equal(t, "formal", cfg.Profile.Tone.Formality, "the target locale's override must be applied")
	guide := coreprofile.RenderVoiceGuideCompact(cfg.Profile)
	assert.Contains(t, guide, "formality: formal")
	assert.Contains(t, guide, "Address the reader as a peer")
	assert.Equal(t, []coreprofile.TermRule{{Term: "software", Replacement: "logiciel"}}, cfg.TermRules)
	assert.Equal(t, model.LocaleID("en"), cfg.SourceLocale)
	assert.Equal(t, model.LocaleID("fr"), cfg.TargetLocale)
}

// TestEditorTranslateConfigBareWithoutVoiceContext pins the control case: a
// server with no voice/terms wiring (or a workspace with nothing bound)
// constructs exactly the pre-existing bare config — no profile, no term rules,
// locales and batching intact.
func TestEditorTranslateConfigBareWithoutVoiceContext(t *testing.T) {
	cs, err := bstore.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	proj := &platstore.Project{ID: "p1", DefaultSourceLanguage: "en"}
	require.NoError(t, cs.CreateProject(t.Context(), proj))

	cfg := editorTranslateConfig(t.Context(), cs, editorVoiceContext{}, proj,
		"p1", "main", "hello.txt", "ws-1", "acme", TranslateRequest{TargetLocale: "fr", BatchSize: 7, Concurrency: 2})

	assert.Nil(t, cfg.Profile)
	assert.Nil(t, cfg.TermRules)
	assert.Nil(t, cfg.DNT, "no dnt_terms in project settings → no DNT list")
	assert.Equal(t, model.LocaleID("en"), cfg.SourceLocale)
	assert.Equal(t, model.LocaleID("fr"), cfg.TargetLocale)
	assert.Equal(t, 7, cfg.BatchSize)
	assert.Equal(t, 2, cfg.BatchConcurrency)
}

// TestEditorTranslateConfigCarriesDNTTerms proves the interactive editor
// translate config carries the project's do-not-translate terms exactly as a
// worker job does (jobs.ProjectDNTTerms over Properties["dnt_terms"]), so a
// per-block editor translation cannot mangle a term a batch job protects.
func TestEditorTranslateConfigCarriesDNTTerms(t *testing.T) {
	cs, err := bstore.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	proj := &platstore.Project{
		ID:                    "p1",
		DefaultSourceLanguage: "en",
		Properties:            map[string]string{"dnt_terms": " Kapi , Bowrain , "},
	}
	require.NoError(t, cs.CreateProject(t.Context(), proj))

	cfg := editorTranslateConfig(t.Context(), cs, editorVoiceContext{}, proj,
		"p1", "main", "hello.txt", "ws-1", "acme", TranslateRequest{TargetLocale: "fr"})

	assert.Equal(t, []string{"Kapi", "Bowrain"}, cfg.DNT)
	assert.Equal(t, jobs.ProjectDNTTerms(proj), cfg.DNT,
		"editor and worker must derive the DNT list identically")
}

// TestEditorTranslateConfigDegradesGracefully pins the never-fail contract on
// the interactive path: an unbound workspace default, an unresolvable profile,
// or a terms store that cannot be opened all degrade to a bare translation.
func TestEditorTranslateConfigDegradesGracefully(t *testing.T) {
	cs, err := bstore.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	proj := &platstore.Project{ID: "p1", WorkspaceID: "ws-1", DefaultSourceLanguage: "en"}
	require.NoError(t, cs.CreateProject(t.Context(), proj))

	voiceCtx := editorVoiceContext{
		// The workspace default names a profile the store cannot resolve.
		Voice:            &editorFakeVoiceStore{profiles: map[string]*coreprofile.VoiceProfile{}},
		WorkspaceDefault: editorFakeWorkspaceDefault{id: "gone"},
		// No pgDB and no factory: getTerms fails.
		Stores: newWorkspaceStores(),
	}

	cfg := editorTranslateConfig(t.Context(), cs, voiceCtx, proj,
		"p1", "main", "hello.txt", "ws-1", "acme", TranslateRequest{TargetLocale: "fr"})

	assert.Nil(t, cfg.Profile, "unresolvable profile → bare, not fatal")
	assert.Nil(t, cfg.TermRules, "unopenable terms → bare, not fatal")
}

// TestEditorAITranslateWithVoiceContext runs the full editorAITranslate path
// with the deterministic demo provider and a bound governing context, proving the
// brand-context binding never breaks the interactive translation: blocks are
// translated and stored, and stats report the work.
func TestEditorAITranslateWithVoiceContext(t *testing.T) {
	ctx := t.Context()
	cs, voiceCtx, _ := editorVoiceContextFixture(t)

	stats, err := editorAITranslate(ctx, cs, nil, nil, "p1", "main", "hello.txt",
		TranslateRequest{TargetLocale: "fr", Provider: "demo"},
		nil, "ws-1", "acme", jobs.PlatformProviderConfig{}, voiceCtx)
	require.NoError(t, err)
	require.NotNil(t, stats)
	assert.Equal(t, 1, stats.TotalBlocks)
	assert.Equal(t, 1, stats.TranslatedBlocks)

	stored, err := cs.GetBlocks(ctx, platstore.BlockQuery{ProjectID: "p1", Stream: "main", ItemName: "hello.txt"})
	require.NoError(t, err)
	require.Len(t, stored, 1)
	assert.NotEmpty(t, stored[0].Block.TargetText("fr"), "the translated target must be stored")
}
