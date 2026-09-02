package jobs

import (
	"context"
	"errors"
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeVoiceStore resolves profiles by ID from a fixed set. Embedding the
// interface means only GetProfile is implemented — the resolution path uses
// nothing else.
type fakeVoiceStore struct {
	coreprofile.Store
	profiles map[string]*coreprofile.VoiceProfile
}

func (f *fakeVoiceStore) GetProfile(_ context.Context, id string) (*coreprofile.VoiceProfile, error) {
	if p, ok := f.profiles[id]; ok {
		return p, nil
	}
	return nil, errors.New("profile not found: " + id)
}

// fakeWorkspaceDefault returns a fixed workspace-level default profile ID.
type fakeWorkspaceDefault struct{ id string }

func (f fakeWorkspaceDefault) WorkspaceVoiceProfileID(context.Context, string) (string, error) {
	return f.id, nil
}

// seedConcept adds a concept with a source term and a preferred target term to
// the in-memory terms store. projectID "" is workspace-scoped.
func seedConcept(t *testing.T, tb terms.Terminology, id, projectID, source, target string, targetStatus model.TermStatus) {
	t.Helper()
	ts := []terms.Term{
		{Text: source, Locale: "en", Status: model.TermPreferred},
	}
	if target != "" {
		ts = append(ts, terms.Term{Text: target, Locale: "fr", Status: targetStatus})
	}
	require.NoError(t, tb.AddConcept(t.Context(), terms.Concept{
		ID:        id,
		ProjectID: projectID,
		Terms:     ts,
	}))
}

// TestResolveJobTermRules_BuildsFromConcepts proves the rule derivation:
// workspace-scoped concepts and this project's concepts contribute
// source→preferred-target pairs; other projects' concepts are excluded; a
// concept with no target-locale rendering contributes nothing; and a
// project-scoped rendering beats a workspace-scoped one for the same source
// jobTermRules is the term-rule half of the shared assembly, resolved the way a
// job resolves it: the worker's terms resolver, this job's project, and the
// locale pair under test.
func jobTermRules(ctx context.Context, deps *WorkerDeps, job *TranslationJob, source, target model.LocaleID) []coreprofile.TermRule {
	b := jobTranslateBinding(deps, job, nil)
	b.TargetLocale = target
	return b.termRules(ctx, source)
}

// term.
func TestResolveJobTermRules_BuildsFromConcepts(t *testing.T) {
	tb := terms.NewInMemoryStore()
	seedConcept(t, tb, "c1", "", "dashboard", "tableau de bord", model.TermPreferred)
	seedConcept(t, tb, "c2", "proj-1", "berth", "poste d'amarrage", model.TermPreferred)
	seedConcept(t, tb, "c3", "proj-OTHER", "vessel", "navire", model.TermPreferred)
	seedConcept(t, tb, "c4", "", "no-rendering", "", model.TermPreferred)
	// Same source term at both scopes: the project-scoped rendering must win
	// regardless of concept iteration order.
	seedConcept(t, tb, "c5", "", "alert", "alerte", model.TermPreferred)
	seedConcept(t, tb, "c6", "proj-1", "alert", "avis de vigilance", model.TermPreferred)

	deps := &WorkerDeps{
		TermsResolver: TermsResolverFunc(func(slug string) (terms.Terminology, error) {
			assert.Equal(t, "acme", slug)
			return tb, nil
		}),
	}
	job := &TranslationJob{ID: "j1", WorkspaceSlug: "acme", ProjectID: "proj-1", TargetLocale: "fr"}

	got := jobTermRules(t.Context(), deps, job, "en", "fr")
	// Ordered by term, so one terms store yields one prompt every run.
	assert.Equal(t, []coreprofile.TermRule{
		{Term: "alert", Replacement: "avis de vigilance"},
		{Term: "berth", Replacement: "poste d'amarrage"},
		{Term: "dashboard", Replacement: "tableau de bord"},
	}, got)
}

// TestResolveJobTermRules_PrefersApprovedTargetTerm confirms the target
// rendering follows PreferredTerm semantics: a preferred term wins over other
// candidates and forbidden/deprecated renderings are never mandated.
func TestResolveJobTermRules_PrefersApprovedTargetTerm(t *testing.T) {
	tb := terms.NewInMemoryStore()
	require.NoError(t, tb.AddConcept(t.Context(), terms.Concept{
		ID: "c1",
		Terms: []terms.Term{
			{Text: "sync", Locale: "en", Status: model.TermPreferred},
			{Text: "synchronisation interdite", Locale: "fr", Status: model.TermForbidden},
			{Text: "rapprochement des données", Locale: "fr", Status: model.TermPreferred},
		},
	}))
	deps := &WorkerDeps{TermsResolver: TermsResolverFunc(func(string) (terms.Terminology, error) { return tb, nil })}
	job := &TranslationJob{ID: "j1", WorkspaceSlug: "acme", ProjectID: "p", TargetLocale: "fr"}

	got := jobTermRules(t.Context(), deps, job, "en", "fr")
	assert.Equal(t, []coreprofile.TermRule{{Term: "sync", Replacement: "rapprochement des données"}}, got)
}

// TestResolveJobTermRules_DegradesGracefully pins the never-fail contract: no
// resolver, a failing resolver, or an empty terms store all yield nil rules
// and never an error surface.
func TestResolveJobTermRules_DegradesGracefully(t *testing.T) {
	job := &TranslationJob{ID: "j1", WorkspaceSlug: "acme", ProjectID: "p", TargetLocale: "fr"}

	assert.Nil(t, jobTermRules(t.Context(), &WorkerDeps{}, job, "en", "fr"), "no TermsResolver → bare")

	failing := &WorkerDeps{TermsResolver: TermsResolverFunc(func(string) (terms.Terminology, error) {
		return nil, errors.New("db down")
	})}
	assert.Nil(t, jobTermRules(t.Context(), failing, job, "en", "fr"), "resolver failure → bare, not fatal")

	empty := &WorkerDeps{TermsResolver: TermsResolverFunc(func(string) (terms.Terminology, error) {
		return terms.NewInMemoryStore(), nil
	})}
	assert.Nil(t, jobTermRules(t.Context(), empty, job, "en", "fr"), "no terms for the pair → nil, not an empty slice")

	assert.Nil(t, jobTermRules(t.Context(), empty, job, "", "fr"), "missing source locale → bare")
}

// TestResolveJobVoiceProfile_WorkspaceDefault resolves the base rung of the
// ladder: no project/stream binding, but the workspace carries a default
// profile — the job gets it, with the target locale's override applied.
func TestResolveJobVoiceProfile_WorkspaceDefault(t *testing.T) {
	profile := &coreprofile.VoiceProfile{
		ID:   "bp-1",
		Name: "Acme Voice",
		Tone: coreprofile.ToneProfile{Formality: "neutral"},
		Locales: map[model.LocaleID]coreprofile.LocaleOverride{
			"fr": {Formality: "formal"},
		},
	}
	deps := &WorkerDeps{
		VoiceStore:       &fakeVoiceStore{profiles: map[string]*coreprofile.VoiceProfile{"bp-1": profile}},
		WorkspaceDefault: fakeWorkspaceDefault{id: "bp-1"},
	}
	job := &TranslationJob{ID: "j1", WorkspaceID: "ws-1", ProjectID: "p", TargetLocale: "fr"}

	got := resolveJobVoiceProfile(t.Context(), deps, job)
	require.NotNil(t, got)
	assert.Equal(t, "Acme Voice", got.Name)
	assert.Equal(t, "formal", got.Tone.Formality, "the target locale's override must be applied")
}

// TestResolveJobVoiceProfile_DegradesGracefully pins the never-fail contract:
// no voice store, or nothing bound at any rung, yields nil — never an error.
func TestResolveJobVoiceProfile_DegradesGracefully(t *testing.T) {
	job := &TranslationJob{ID: "j1", WorkspaceID: "ws-1", ProjectID: "p", TargetLocale: "fr"}

	assert.Nil(t, resolveJobVoiceProfile(t.Context(), &WorkerDeps{}, job), "no VoiceStore → bare")

	unbound := &WorkerDeps{
		VoiceStore:       &fakeVoiceStore{profiles: map[string]*coreprofile.VoiceProfile{}},
		WorkspaceDefault: fakeWorkspaceDefault{id: ""},
	}
	assert.Nil(t, resolveJobVoiceProfile(t.Context(), unbound, job), "nothing bound anywhere → bare")
}

// TestJobTranslateConfig_BareWithoutVoiceDeps proves a deployment without the
// brand context deps (or a project with none bound) constructs exactly the
// pre-existing bare config: no profile, no term rules, DNT and locales intact.
func TestJobTranslateConfig_BareWithoutVoiceDeps(t *testing.T) {
	proj := &store.Project{
		ID:                    "p",
		DefaultSourceLanguage: "en",
		Properties:            map[string]string{"dnt_terms": "Kapi, Bowrain"},
	}
	job := &TranslationJob{ID: "j1", ProjectID: "p", TargetLocale: "fr"}

	cfg := jobTranslateConfig(t.Context(), &WorkerDeps{}, job, proj)
	assert.Nil(t, cfg.Profile)
	assert.Nil(t, cfg.TermRules)
	assert.Equal(t, model.LocaleID("en"), cfg.SourceLocale)
	assert.Equal(t, model.LocaleID("fr"), cfg.TargetLocale)
	assert.Equal(t, []string{"Kapi", "Bowrain"}, cfg.DNT)
	assert.Equal(t, 20, cfg.BatchSize)
	assert.Equal(t, 5, cfg.BatchConcurrency)
}

// TestJobTranslateConfig_CarriesVoiceContext proves the constructed translate
// config carries the resolved voice profile (rendering to the expected guide
// text) and the terms store-derived rules — the exact fields the AI translate
// tool injects into every prompt.
func TestJobTranslateConfig_CarriesVoiceContext(t *testing.T) {
	profile := &coreprofile.VoiceProfile{
		ID:   "bp-1",
		Name: "Acme Voice",
		Tone: coreprofile.ToneProfile{Formality: "casual", Guidelines: "Address the reader as a peer"},
		Vocabulary: coreprofile.VocabularyRules{
			ForbiddenTerms: []coreprofile.TermRule{{Term: "utilize", Replacement: "use"}},
		},
	}
	tb := terms.NewInMemoryStore()
	seedConcept(t, tb, "c1", "", "dashboard", "tableau de bord", model.TermPreferred)

	deps := &WorkerDeps{
		VoiceStore:       &fakeVoiceStore{profiles: map[string]*coreprofile.VoiceProfile{"bp-1": profile}},
		WorkspaceDefault: fakeWorkspaceDefault{id: "bp-1"},
		TermsResolver:    TermsResolverFunc(func(string) (terms.Terminology, error) { return tb, nil }),
	}
	proj := &store.Project{ID: "p", DefaultSourceLanguage: "en"}
	job := &TranslationJob{ID: "j1", WorkspaceID: "ws-1", WorkspaceSlug: "acme", ProjectID: "p", TargetLocale: "fr"}

	cfg := jobTranslateConfig(t.Context(), deps, job, proj)

	require.NotNil(t, cfg.Profile)
	guide := coreprofile.RenderVoiceGuideCompact(cfg.Profile)
	assert.Contains(t, guide, "formality: casual")
	assert.Contains(t, guide, "Address the reader as a peer")
	assert.Contains(t, guide, `"utilize" → "use"`)
	assert.Equal(t, []coreprofile.TermRule{{Term: "dashboard", Replacement: "tableau de bord"}}, cfg.TermRules)
}
