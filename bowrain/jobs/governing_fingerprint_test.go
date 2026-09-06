package jobs

import (
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// governingBinding is a binding over a project whose workspace binds a voice
// profile and mandates one rendering.
func governingBinding(t *testing.T) TranslateBinding {
	t.Helper()
	tb := terms.NewInMemoryStore()
	seedConcept(t, tb, "c1", "", "software", "logiciel", model.TermPreferred)
	profile := &coreprofile.VoiceProfile{
		ID:      "bp-1",
		Name:    "Acme Voice",
		Version: 3,
		Tone:    coreprofile.ToneProfile{Formality: "casual", Guidelines: "Address the reader as a peer"},
	}
	return TranslateBinding{
		Voice:            &fakeVoiceStore{profiles: map[string]*coreprofile.VoiceProfile{"bp-1": profile}},
		WorkspaceDefault: fakeWorkspaceDefault{id: "bp-1"},
		Terms:            tb,
		Project:          &store.Project{ID: "p1", WorkspaceID: "ws-1", DefaultSourceLanguage: "en"},
		WorkspaceID:      "ws-1",
		ProjectID:        "p1",
		TargetLocale:     "fr",
	}
}

// TestGoverningFingerprintFoldsTheSameContextATranslationCarries proves the
// fingerprint a decision records is the one a translation of the same unit
// under the same binding is stamped with: one definition of the context in
// force, whichever surface asks.
func TestGoverningFingerprintFoldsTheSameContextATranslationCarries(t *testing.T) {
	b := governingBinding(t)

	cfg := BuildTranslateConfig(t.Context(), b)
	require.NotNil(t, cfg.Profile)
	require.NotEmpty(t, cfg.TermRules)
	_, _, want := coreprofile.GovernanceContext(cfg.Profile, cfg.TermRules)

	assert.Equal(t, want, b.GoverningFingerprint(t.Context()))
	assert.NotEmpty(t, b.GoverningFingerprint(t.Context()))
}

// TestGoverningFingerprintMovesWithTheContext proves the fingerprint answers
// the question it exists for: it moves when the voice moves and when the
// terminology moves, and an ungoverned point folds to nothing.
func TestGoverningFingerprintMovesWithTheContext(t *testing.T) {
	base := governingBinding(t).GoverningFingerprint(t.Context())
	require.NotEmpty(t, base)

	moved := governingBinding(t)
	moved.Voice = &fakeVoiceStore{profiles: map[string]*coreprofile.VoiceProfile{
		"bp-1": {ID: "bp-1", Name: "Acme Voice", Version: 4, Tone: coreprofile.ToneProfile{Formality: "formal"}},
	}}
	assert.NotEqual(t, base, moved.GoverningFingerprint(t.Context()), "moved voice, moved context")

	retermed := governingBinding(t)
	tb := terms.NewInMemoryStore()
	seedConcept(t, tb, "c1", "", "software", "programme", model.TermPreferred)
	retermed.Terms = tb
	assert.NotEqual(t, base, retermed.GoverningFingerprint(t.Context()), "moved terminology, moved context")

	var ungoverned TranslateBinding
	assert.Empty(t, ungoverned.GoverningFingerprint(t.Context()), "no voice and no terms is no context")
}
