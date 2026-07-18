package jobs

import (
	"context"
	"errors"
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/termbase"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeBrandStore resolves profiles by ID from a fixed set. Embedding the
// interface means only GetProfile is implemented — the resolution path uses
// nothing else.
type fakeBrandStore struct {
	brand.BrandStore
	profiles map[string]*brand.VoiceProfile
}

func (f *fakeBrandStore) GetProfile(_ context.Context, id string) (*brand.VoiceProfile, error) {
	if p, ok := f.profiles[id]; ok {
		return p, nil
	}
	return nil, errors.New("profile not found: " + id)
}

// fakeWorkspaceDefault returns a fixed workspace-level default profile ID.
type fakeWorkspaceDefault struct{ id string }

func (f fakeWorkspaceDefault) WorkspaceBrandProfileID(context.Context, string) (string, error) {
	return f.id, nil
}

// seedConcept adds a concept with a source term and a preferred target term to
// the in-memory termbase. projectID "" is workspace-scoped.
func seedConcept(t *testing.T, tb termbase.TermBase, id, projectID, source, target string, targetStatus model.TermStatus) {
	t.Helper()
	terms := []termbase.Term{
		{Text: source, Locale: "en", Status: model.TermPreferred},
	}
	if target != "" {
		terms = append(terms, termbase.Term{Text: target, Locale: "fr", Status: targetStatus})
	}
	require.NoError(t, tb.AddConcept(t.Context(), termbase.Concept{
		ID:        id,
		ProjectID: projectID,
		Terms:     terms,
	}))
}

// TestResolveJobGlossary_BuildsFromTermbase proves the glossary derivation:
// workspace-scoped concepts and this project's concepts contribute
// source→preferred-target pairs; other projects' concepts are excluded; a
// concept with no target-locale rendering contributes nothing; and a
// project-scoped rendering beats a workspace-scoped one for the same source
// term.
func TestResolveJobGlossary_BuildsFromTermbase(t *testing.T) {
	tb := termbase.NewInMemoryTermBase()
	seedConcept(t, tb, "c1", "", "dashboard", "tableau de bord", model.TermPreferred)
	seedConcept(t, tb, "c2", "proj-1", "berth", "poste d'amarrage", model.TermPreferred)
	seedConcept(t, tb, "c3", "proj-OTHER", "vessel", "navire", model.TermPreferred)
	seedConcept(t, tb, "c4", "", "no-rendering", "", model.TermPreferred)
	// Same source term at both scopes: the project-scoped rendering must win
	// regardless of concept iteration order.
	seedConcept(t, tb, "c5", "", "alert", "alerte", model.TermPreferred)
	seedConcept(t, tb, "c6", "proj-1", "alert", "avis de vigilance", model.TermPreferred)

	deps := &WorkerDeps{
		TBResolver: TBResolverFunc(func(slug string) (termbase.TermBase, error) {
			assert.Equal(t, "acme", slug)
			return tb, nil
		}),
	}
	job := &TranslationJob{ID: "j1", WorkspaceSlug: "acme", ProjectID: "proj-1", TargetLocale: "fr"}

	got := resolveJobGlossary(t.Context(), deps, job, "en", "fr")
	assert.Equal(t, map[string]string{
		"dashboard": "tableau de bord",
		"berth":     "poste d'amarrage",
		"alert":     "avis de vigilance",
	}, got)
}

// TestResolveJobGlossary_PrefersApprovedTargetTerm confirms the target
// rendering follows PreferredTerm semantics: a preferred term wins over other
// candidates and forbidden/deprecated renderings are never mandated.
func TestResolveJobGlossary_PrefersApprovedTargetTerm(t *testing.T) {
	tb := termbase.NewInMemoryTermBase()
	require.NoError(t, tb.AddConcept(t.Context(), termbase.Concept{
		ID: "c1",
		Terms: []termbase.Term{
			{Text: "sync", Locale: "en", Status: model.TermPreferred},
			{Text: "synchronisation interdite", Locale: "fr", Status: model.TermForbidden},
			{Text: "rapprochement des données", Locale: "fr", Status: model.TermPreferred},
		},
	}))
	deps := &WorkerDeps{TBResolver: TBResolverFunc(func(string) (termbase.TermBase, error) { return tb, nil })}
	job := &TranslationJob{ID: "j1", WorkspaceSlug: "acme", ProjectID: "p", TargetLocale: "fr"}

	got := resolveJobGlossary(t.Context(), deps, job, "en", "fr")
	assert.Equal(t, map[string]string{"sync": "rapprochement des données"}, got)
}

// TestResolveJobGlossary_DegradesGracefully pins the never-fail contract: no
// resolver, a failing resolver, or an empty termbase all yield a nil glossary
// and never an error surface.
func TestResolveJobGlossary_DegradesGracefully(t *testing.T) {
	job := &TranslationJob{ID: "j1", WorkspaceSlug: "acme", ProjectID: "p", TargetLocale: "fr"}

	assert.Nil(t, resolveJobGlossary(t.Context(), &WorkerDeps{}, job, "en", "fr"), "no TBResolver → bare")

	failing := &WorkerDeps{TBResolver: TBResolverFunc(func(string) (termbase.TermBase, error) {
		return nil, errors.New("db down")
	})}
	assert.Nil(t, resolveJobGlossary(t.Context(), failing, job, "en", "fr"), "resolver failure → bare, not fatal")

	empty := &WorkerDeps{TBResolver: TBResolverFunc(func(string) (termbase.TermBase, error) {
		return termbase.NewInMemoryTermBase(), nil
	})}
	assert.Nil(t, resolveJobGlossary(t.Context(), empty, job, "en", "fr"), "no terms for the pair → nil, not empty map")

	assert.Nil(t, resolveJobGlossary(t.Context(), empty, job, "", "fr"), "missing source locale → bare")
}

// TestResolveJobBrandProfile_WorkspaceDefault resolves the base rung of the
// ladder: no project/stream binding, but the workspace carries a default
// profile — the job gets it, with the target locale's override applied.
func TestResolveJobBrandProfile_WorkspaceDefault(t *testing.T) {
	profile := &brand.VoiceProfile{
		ID:   "bp-1",
		Name: "Acme Voice",
		Tone: brand.ToneProfile{Formality: "neutral"},
		Locales: map[model.LocaleID]brand.LocaleOverride{
			"fr": {Formality: "formal"},
		},
	}
	deps := &WorkerDeps{
		BrandStore:       &fakeBrandStore{profiles: map[string]*brand.VoiceProfile{"bp-1": profile}},
		WorkspaceDefault: fakeWorkspaceDefault{id: "bp-1"},
	}
	job := &TranslationJob{ID: "j1", WorkspaceID: "ws-1", ProjectID: "p", TargetLocale: "fr"}

	got := resolveJobBrandProfile(t.Context(), deps, job)
	require.NotNil(t, got)
	assert.Equal(t, "Acme Voice", got.Name)
	assert.Equal(t, "formal", got.Tone.Formality, "the target locale's override must be applied")
}

// TestResolveJobBrandProfile_DegradesGracefully pins the never-fail contract:
// no brand store, or nothing bound at any rung, yields nil — never an error.
func TestResolveJobBrandProfile_DegradesGracefully(t *testing.T) {
	job := &TranslationJob{ID: "j1", WorkspaceID: "ws-1", ProjectID: "p", TargetLocale: "fr"}

	assert.Nil(t, resolveJobBrandProfile(t.Context(), &WorkerDeps{}, job), "no BrandStore → bare")

	unbound := &WorkerDeps{
		BrandStore:       &fakeBrandStore{profiles: map[string]*brand.VoiceProfile{}},
		WorkspaceDefault: fakeWorkspaceDefault{id: ""},
	}
	assert.Nil(t, resolveJobBrandProfile(t.Context(), unbound, job), "nothing bound anywhere → bare")
}

// TestJobTranslateConfig_BareWithoutBrandDeps proves a deployment without the
// brand context deps (or a project with none bound) constructs exactly the
// pre-existing bare config: no profile, no glossary, DNT and locales intact.
func TestJobTranslateConfig_BareWithoutBrandDeps(t *testing.T) {
	proj := &store.Project{
		ID:                    "p",
		DefaultSourceLanguage: "en",
		Properties:            map[string]string{"dnt_terms": "Kapi, Bowrain"},
	}
	job := &TranslationJob{ID: "j1", ProjectID: "p", TargetLocale: "fr"}

	cfg := jobTranslateConfig(t.Context(), &WorkerDeps{}, job, proj)
	assert.Nil(t, cfg.Profile)
	assert.Nil(t, cfg.Glossary)
	assert.Equal(t, model.LocaleID("en"), cfg.SourceLocale)
	assert.Equal(t, model.LocaleID("fr"), cfg.TargetLocale)
	assert.Equal(t, []string{"Kapi", "Bowrain"}, cfg.DNT)
	assert.Equal(t, 20, cfg.BatchSize)
	assert.Equal(t, 5, cfg.BatchConcurrency)
}

// TestJobTranslateConfig_CarriesBrandContext proves the constructed translate
// config carries the resolved voice profile (rendering to the expected guide
// text) and the termbase-derived glossary — the exact fields the AI translate
// tool injects into every prompt.
func TestJobTranslateConfig_CarriesBrandContext(t *testing.T) {
	profile := &brand.VoiceProfile{
		ID:   "bp-1",
		Name: "Acme Voice",
		Tone: brand.ToneProfile{Formality: "casual", Guidelines: "Address the reader as a peer"},
		Vocabulary: brand.VocabularyRules{
			ForbiddenTerms: []brand.TermRule{{Term: "utilize", Replacement: "use"}},
		},
	}
	tb := termbase.NewInMemoryTermBase()
	seedConcept(t, tb, "c1", "", "dashboard", "tableau de bord", model.TermPreferred)

	deps := &WorkerDeps{
		BrandStore:       &fakeBrandStore{profiles: map[string]*brand.VoiceProfile{"bp-1": profile}},
		WorkspaceDefault: fakeWorkspaceDefault{id: "bp-1"},
		TBResolver:       TBResolverFunc(func(string) (termbase.TermBase, error) { return tb, nil }),
	}
	proj := &store.Project{ID: "p", DefaultSourceLanguage: "en"}
	job := &TranslationJob{ID: "j1", WorkspaceID: "ws-1", WorkspaceSlug: "acme", ProjectID: "p", TargetLocale: "fr"}

	cfg := jobTranslateConfig(t.Context(), deps, job, proj)

	require.NotNil(t, cfg.Profile)
	guide := brand.RenderVoiceGuideCompact(cfg.Profile)
	assert.Contains(t, guide, "formality: casual")
	assert.Contains(t, guide, "Address the reader as a peer")
	assert.Contains(t, guide, `"utilize" → "use"`)
	assert.Equal(t, map[string]string{"dashboard": "tableau de bord"}, cfg.Glossary)
}
