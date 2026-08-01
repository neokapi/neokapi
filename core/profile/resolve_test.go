package profile

import (
	"context"
	"fmt"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockBrandStore is a minimal BrandStore for testing profile resolution.
type mockBrandStore struct {
	profiles map[string]*VoiceProfile
}

func (m *mockBrandStore) GetProfile(_ context.Context, id string) (*VoiceProfile, error) {
	p, ok := m.profiles[id]
	if !ok {
		return nil, fmt.Errorf("profile not found: %s", id)
	}
	return p, nil
}

func (m *mockBrandStore) CreateProfile(context.Context, *VoiceProfile) error { return nil }
func (m *mockBrandStore) UpdateProfile(context.Context, *VoiceProfile) error { return nil }
func (m *mockBrandStore) DeleteProfile(context.Context, string) error        { return nil }
func (m *mockBrandStore) ListProfiles(context.Context, string) ([]*VoiceProfile, error) {
	return nil, nil
}
func (m *mockBrandStore) ListProfileVersions(context.Context, string) ([]*ProfileVersion, error) {
	return nil, nil
}
func (m *mockBrandStore) GetProfileVersion(context.Context, string, int) (*ProfileVersion, error) {
	return nil, nil
}
func (m *mockBrandStore) GetProfileAtTag(context.Context, string, string) (*VoiceProfile, error) {
	return nil, nil
}
func (m *mockBrandStore) CreateProfileTag(context.Context, *ProfileTag) error { return nil }
func (m *mockBrandStore) ListProfileTags(context.Context, string) ([]*ProfileTag, error) {
	return nil, nil
}
func (m *mockBrandStore) DeleteProfileTag(context.Context, string, string) error { return nil }
func (m *mockBrandStore) StoreScore(context.Context, *StoredScore) error         { return nil }
func (m *mockBrandStore) GetScores(context.Context, string, model.LocaleID) ([]*StoredScore, error) {
	return nil, nil
}
func (m *mockBrandStore) GetScoreTrends(context.Context, string, int) ([]*ScoreTrend, error) {
	return nil, nil
}
func (m *mockBrandStore) GetScoresByStream(context.Context, string, string) ([]*StoredScore, error) {
	return nil, nil
}
func (m *mockBrandStore) StoreCorrection(context.Context, *Correction) error      { return nil }
func (m *mockBrandStore) RecordRuleDecision(context.Context, *RuleDecision) error { return nil }
func (m *mockBrandStore) GetRuleDecision(context.Context, string, string) (*RuleDecision, error) {
	return nil, nil
}
func (m *mockBrandStore) ListRuleDecisions(context.Context, string) ([]*RuleDecision, error) {
	return nil, nil
}
func (m *mockBrandStore) GetSuggestedRules(context.Context, string, int) ([]*SuggestedRule, error) {
	return nil, nil
}
func (m *mockBrandStore) Close() error { return nil }

func TestResolveProfile_Nil(t *testing.T) {
	assert.Nil(t, ResolveProfile(nil, "en", "web", ""))
}

func TestResolveProfile_NoOverrides(t *testing.T) {
	profile := &VoiceProfile{
		ID:   "test",
		Name: "Test Profile",
		Tone: ToneProfile{Formality: "casual", Humor: "light"},
	}

	resolved := ResolveProfile(profile, "", "", "")

	require.NotNil(t, resolved)
	assert.Equal(t, "casual", resolved.Tone.Formality)
	assert.Equal(t, "light", resolved.Tone.Humor)
}

func TestResolveProfile_LocaleOverride(t *testing.T) {
	profile := &VoiceProfile{
		ID:    "test",
		Name:  "Test Profile",
		Tone:  ToneProfile{Formality: "casual", Humor: "light"},
		Style: StyleRules{PersonPOV: "second"},
		Vocabulary: VocabularyRules{
			PreferredTerms: []TermRule{{Term: "app", Replacement: "application"}},
		},
		Locales: map[model.LocaleID]LocaleOverride{
			"ja-JP": {
				Formality:           "formal",
				Humor:               "none",
				PersonPOV:           "third",
				VocabularyOverrides: []TermRule{{Term: "san", Note: "use honorifics"}},
				ExampleOverrides:    []VoiceExample{{Before: "Hey!", After: "Dear customer,"}},
			},
		},
	}

	resolved := ResolveProfile(profile, "ja-JP", "", "")

	require.NotNil(t, resolved)
	assert.Equal(t, "formal", resolved.Tone.Formality)
	assert.Equal(t, "none", resolved.Tone.Humor)
	assert.Equal(t, "third", resolved.Style.PersonPOV)
	assert.Len(t, resolved.Vocabulary.PreferredTerms, 2) // original + override
	assert.Len(t, resolved.Examples, 1)
}

func TestResolveProfile_ChannelOverride(t *testing.T) {
	profile := &VoiceProfile{
		ID:    "test",
		Name:  "Test Profile",
		Tone:  ToneProfile{Formality: "casual", Humor: "light", Personality: []string{"friendly"}},
		Style: StyleRules{PersonPOV: "second", ActiveVoice: true},
		Channels: map[string]ChannelOverride{
			"support": {
				Tone:  &ToneProfile{Formality: "formal", Emotion: "empathetic", Personality: []string{"caring"}},
				Style: &StyleRules{PersonPOV: "first_plural", ActiveVoice: false},
			},
		},
	}

	resolved := ResolveProfile(profile, "", "support", "")

	require.NotNil(t, resolved)
	assert.Equal(t, "formal", resolved.Tone.Formality)
	assert.Equal(t, "empathetic", resolved.Tone.Emotion)
	assert.Equal(t, "first_plural", resolved.Style.PersonPOV)
	assert.False(t, resolved.Style.ActiveVoice)
}

func TestResolveProfile_LocaleAndChannel(t *testing.T) {
	profile := &VoiceProfile{
		ID:   "test",
		Name: "Test Profile",
		Tone: ToneProfile{Formality: "casual", Humor: "light"},
		Locales: map[model.LocaleID]LocaleOverride{
			"de-DE": {Formality: "formal"},
		},
		Channels: map[string]ChannelOverride{
			"marketing": {
				Tone: &ToneProfile{Formality: "casual", Humor: "frequent"},
			},
		},
	}

	// Channel override replaces tone entirely, so locale's formality override
	// is applied first, then channel replaces the whole tone.
	resolved := ResolveProfile(profile, "de-DE", "marketing", "")

	require.NotNil(t, resolved)
	// Channel override replaces the entire tone
	assert.Equal(t, "casual", resolved.Tone.Formality)
	assert.Equal(t, "frequent", resolved.Tone.Humor)
}

func TestResolveProfile_UnknownLocale(t *testing.T) {
	profile := &VoiceProfile{
		ID:   "test",
		Tone: ToneProfile{Formality: "casual"},
		Locales: map[model.LocaleID]LocaleOverride{
			"ja-JP": {Formality: "formal"},
		},
	}

	resolved := ResolveProfile(profile, "fr-FR", "", "")

	require.NotNil(t, resolved)
	assert.Equal(t, "casual", resolved.Tone.Formality) // unchanged
}

// TestResolveProfile_LocaleNormalization verifies that override lookup tolerates
// BCP-47 formatting differences between the profile's keys and the requested
// locale, so a casing/format mismatch no longer silently drops the override.
func TestResolveProfile_LocaleNormalization(t *testing.T) {
	tests := []struct {
		name      string
		key       model.LocaleID // how the profile stores the override
		lookup    model.LocaleID // how the caller requests it
		wantMatch bool
	}{
		{"region casing", "pt-BR", "pt-br", true},
		{"lowercase request", "pt-BR", "PT-BR", true},
		{"language casing", "en", "EN", true},
		{"canonical request against odd key", "fr-fr", "fr-FR", true},
		// Region specificity must be preserved — normalization fixes form, not granularity.
		{"language does not match region", "en-US", "en", false},
		{"different region", "pt-BR", "pt-PT", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := &VoiceProfile{
				ID:   "test",
				Tone: ToneProfile{Formality: "casual"},
				Locales: map[model.LocaleID]LocaleOverride{
					tt.key: {Formality: "formal"},
				},
			}

			resolved := ResolveProfile(profile, tt.lookup, "", "")

			require.NotNil(t, resolved)
			if tt.wantMatch {
				assert.Equal(t, "formal", resolved.Tone.Formality,
					"expected override keyed %q to match lookup %q", tt.key, tt.lookup)
			} else {
				assert.Equal(t, "casual", resolved.Tone.Formality,
					"expected override keyed %q NOT to match lookup %q", tt.key, tt.lookup)
			}
		})
	}
}

// personaTestProfile is a brand profile with a forbidden term, a channel
// override, and two personas: one that layers tone/style/vocab cleanly, and one
// that tries to re-allow the brand-forbidden term via a Preferred rule.
func personaTestProfile() *VoiceProfile {
	return &VoiceProfile{
		ID:    "test",
		Name:  "Test Profile",
		Tone:  ToneProfile{Formality: "casual", Humor: "light", Personality: []string{"friendly"}},
		Style: StyleRules{PersonPOV: "second", ActiveVoice: true},
		Vocabulary: VocabularyRules{
			ForbiddenTerms: []TermRule{{Term: "utilize", Replacement: "use", Severity: "major"}},
			PreferredTerms: []TermRule{{Term: "sign in"}},
		},
		Channels: map[string]ChannelOverride{
			"email": {Tone: &ToneProfile{Formality: "formal", Humor: "none"}},
		},
		Personas: map[string]PersonaOverride{
			"jordan": {
				Tone:      &ToneProfile{Formality: "neutral", Humor: "frequent"},
				Style:     &StyleRules{PersonPOV: "first_plural"},
				Preferred: []TermRule{{Term: "let's"}},
				Avoided:   []TermRule{{Term: "very", Replacement: ""}},
			},
			// A persona that tries to prefer a brand-forbidden term — the
			// guardrail must refuse to re-allow it.
			"rogue": {
				Preferred: []TermRule{{Term: "utilize", Note: "I like this word"}},
			},
		},
	}
}

func TestResolveProfile_PersonaToneOverridesChannel(t *testing.T) {
	// Persona is applied after channel, so its tone wins over the channel's.
	resolved := ResolveProfile(personaTestProfile(), "", "email", "jordan")

	require.NotNil(t, resolved)
	assert.Equal(t, "neutral", resolved.Tone.Formality, "persona tone must override channel tone")
	assert.Equal(t, "frequent", resolved.Tone.Humor)
	assert.Equal(t, "first_plural", resolved.Style.PersonPOV, "persona style must apply")
}

func TestResolveProfile_PersonaVocabAddsButNeverRemoves(t *testing.T) {
	resolved := ResolveProfile(personaTestProfile(), "", "", "jordan")
	require.NotNil(t, resolved)

	// Avoided term is added to the forbidden set on top of the brand's own.
	forbidden := make(map[string]bool)
	for _, r := range resolved.Vocabulary.ForbiddenTerms {
		forbidden[r.Term] = true
	}
	assert.True(t, forbidden["utilize"], "brand forbidden term must survive persona resolution")
	assert.True(t, forbidden["very"], "persona avoided term must be added to the forbidden set")

	// The persona's clean Preferred term is added; the brand's stays.
	preferred := make(map[string]bool)
	for _, r := range resolved.Vocabulary.PreferredTerms {
		preferred[r.Term] = true
	}
	assert.True(t, preferred["sign in"], "brand preferred term must survive")
	assert.True(t, preferred["let's"], "persona preferred term must be added")

	// The brand's forbidden term is still flagged by the matcher under the persona.
	hits := MatchVocabulary(resolved, "Please utilize the very fast path")
	terms := make(map[string]bool)
	for _, h := range hits {
		terms[h.Term] = true
	}
	assert.True(t, terms["utilize"], "brand forbidden term must still be caught under a persona")
	assert.True(t, terms["very"], "persona avoided term must be caught")
}

func TestResolveProfile_PersonaCannotReAllowForbiddenTerm(t *testing.T) {
	// The "rogue" persona lists the brand-forbidden term "utilize" as preferred.
	resolved := ResolveProfile(personaTestProfile(), "", "", "rogue")
	require.NotNil(t, resolved)

	for _, r := range resolved.Vocabulary.PreferredTerms {
		assert.NotEqual(t, "utilize", r.Term,
			"a persona must not be able to re-allow a brand-forbidden term as preferred")
	}
	// The guardrail is about the preferred list; the term stays forbidden.
	hits := MatchVocabulary(resolved, "utilize this")
	require.Len(t, hits, 1)
	assert.Equal(t, "utilize", hits[0].Term)
}

func TestResolveProfile_UnknownPersonaIsBaseProfile(t *testing.T) {
	base := ResolveProfile(personaTestProfile(), "", "", "")
	unknown := ResolveProfile(personaTestProfile(), "", "", "nobody")

	require.NotNil(t, unknown)
	assert.Equal(t, base.Tone, unknown.Tone, "unknown persona must not change tone")
	assert.Equal(t, base.Style, unknown.Style, "unknown persona must not change style")
	assert.Len(t, unknown.Vocabulary.ForbiddenTerms, len(base.Vocabulary.ForbiddenTerms),
		"unknown persona must not change vocabulary")
	assert.Len(t, unknown.Vocabulary.PreferredTerms, len(base.Vocabulary.PreferredTerms))
}

func TestResolveProfile_PersonaDoesNotMutateSource(t *testing.T) {
	profile := personaTestProfile()
	forbiddenBefore := len(profile.Vocabulary.ForbiddenTerms)
	preferredBefore := len(profile.Vocabulary.PreferredTerms)

	_ = ResolveProfile(profile, "", "", "jordan")

	assert.Len(t, profile.Vocabulary.ForbiddenTerms, forbiddenBefore,
		"resolving a persona must not mutate the source profile's forbidden terms")
	assert.Len(t, profile.Vocabulary.PreferredTerms, preferredBefore,
		"resolving a persona must not mutate the source profile's preferred terms")
}

func TestResolveProfileFromContext(t *testing.T) {
	store := &mockBrandStore{
		profiles: map[string]*VoiceProfile{
			"ws-default": {ID: "ws-default", Name: "Workspace Default", Tone: ToneProfile{Formality: "formal"}},
			"proj-voice": {ID: "proj-voice", Name: "Project Voice", Tone: ToneProfile{Formality: "neutral"}},
			"stream-exp": {ID: "stream-exp", Name: "Stream Experiment", Tone: ToneProfile{Formality: "casual"}},
			"col-voice":  {ID: "col-voice", Name: "Collection Voice", Tone: ToneProfile{Formality: "technical"}},
			"explicit":   {ID: "explicit", Name: "Explicit", Tone: ToneProfile{Formality: "formal"}},
			"with-channel": {
				ID: "with-channel", Name: "With Channel", Tone: ToneProfile{Formality: "formal"},
				Channels: map[string]ChannelOverride{
					"email": {Tone: &ToneProfile{Formality: "casual", Personality: []string{"friendly"}}},
				},
			},
		},
	}

	// A profile the caller loaded itself — what a kapi recipe binds, since its
	// `profiles:` may name a profile file or a starter pack rather than a store
	// row. It enters at the collection tier.
	loaded := &VoiceProfile{
		ID: "recipe-voice", Name: "Recipe Voice", Tone: ToneProfile{Formality: "formal"},
		Channels: map[string]ChannelOverride{
			"docs":  {Tone: &ToneProfile{Formality: "technical"}},
			"email": {Tone: &ToneProfile{Formality: "casual"}},
		},
	}

	tests := []struct {
		name          string
		rc            ResolveContext
		wantName      string
		wantFormality string
		wantNil       bool
	}{
		{
			name:    "no bindings returns nil",
			rc:      ResolveContext{},
			wantNil: true,
		},
		{
			name:          "a loaded collection profile is the collection tier",
			rc:            ResolveContext{CollectionProfile: loaded},
			wantName:      "Recipe Voice",
			wantFormality: "formal",
		},
		{
			name: "a loaded collection profile overrides stream, project and workspace",
			rc: ResolveContext{
				CollectionProfile:  loaded,
				StreamProperties:   map[string]string{PropertyProfileID: "stream-exp"},
				ProjectProperties:  map[string]string{PropertyProfileID: "proj-voice"},
				WorkspaceProfileID: "ws-default",
			},
			wantName:      "Recipe Voice",
			wantFormality: "formal",
		},
		{
			name: "explicit overrides a loaded collection profile",
			rc: ResolveContext{
				ExplicitProfileID: "explicit",
				CollectionProfile: loaded,
			},
			wantName:      "Explicit",
			wantFormality: "formal",
		},
		{
			name: "a channel bound at the collection tier applies to a loaded profile",
			rc: ResolveContext{
				CollectionProfile: loaded,
				CollectionConfig:  map[string]string{PropertyChannel: "docs"},
			},
			wantName:      "Recipe Voice",
			wantFormality: "technical",
		},
		{
			name: "an explicit channel overrides the bound one",
			rc: ResolveContext{
				CollectionProfile: loaded,
				CollectionConfig:  map[string]string{PropertyChannel: "docs"},
				Channel:           "email",
			},
			wantName:      "Recipe Voice",
			wantFormality: "casual",
		},
		{
			name:          "workspace default",
			rc:            ResolveContext{WorkspaceProfileID: "ws-default"},
			wantName:      "Workspace Default",
			wantFormality: "formal",
		},
		{
			name: "project overrides workspace",
			rc: ResolveContext{
				WorkspaceProfileID: "ws-default",
				ProjectProperties:  map[string]string{PropertyProfileID: "proj-voice"},
			},
			wantName:      "Project Voice",
			wantFormality: "neutral",
		},
		{
			name: "stream overrides project",
			rc: ResolveContext{
				ProjectProperties: map[string]string{PropertyProfileID: "proj-voice"},
				StreamProperties:  map[string]string{PropertyProfileID: "stream-exp"},
			},
			wantName:      "Stream Experiment",
			wantFormality: "casual",
		},
		{
			name: "collection overrides stream",
			rc: ResolveContext{
				StreamProperties: map[string]string{PropertyProfileID: "stream-exp"},
				CollectionConfig: map[string]string{PropertyProfileID: "col-voice"},
			},
			wantName:      "Collection Voice",
			wantFormality: "technical",
		},
		{
			name: "explicit overrides everything",
			rc: ResolveContext{
				ExplicitProfileID:  "explicit",
				WorkspaceProfileID: "ws-default",
				ProjectProperties:  map[string]string{PropertyProfileID: "proj-voice"},
				StreamProperties:   map[string]string{PropertyProfileID: "stream-exp"},
				CollectionConfig:   map[string]string{PropertyProfileID: "col-voice"},
			},
			wantName:      "Explicit",
			wantFormality: "formal",
		},
		{
			name: "channel override applied from collection config",
			rc: ResolveContext{
				ProjectProperties: map[string]string{PropertyProfileID: "with-channel"},
				CollectionConfig:  map[string]string{PropertyChannel: "email"},
			},
			wantName:      "With Channel",
			wantFormality: "casual", // channel override replaces tone
		},
		{
			name: "channel resolution: collection wins over project",
			rc: ResolveContext{
				ProjectProperties: map[string]string{
					PropertyProfileID: "with-channel",
					PropertyChannel:   "nonexistent",
				},
				CollectionConfig: map[string]string{PropertyChannel: "email"},
			},
			wantName:      "With Channel",
			wantFormality: "casual",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile, err := ResolveProfileFromContext(t.Context(), tt.rc, store)
			require.NoError(t, err)

			if tt.wantNil {
				assert.Nil(t, profile)
				return
			}

			require.NotNil(t, profile)
			assert.Equal(t, tt.wantName, profile.Name)
			assert.Equal(t, tt.wantFormality, profile.Tone.Formality)
		})
	}
}

// TestResolveProfileFromContext_NoStore covers the caller whose tiers are all
// already-loaded profiles: it passes no store, which resolves fine — until a
// tier binds an id, which then has nowhere to come from. Reporting that beats
// returning nil, which would read as "nothing is bound" and leave the content
// silently ungoverned.
func TestResolveProfileFromContext_NoStore(t *testing.T) {
	loaded := &VoiceProfile{ID: "recipe-voice", Name: "Recipe Voice"}

	profile, err := ResolveProfileFromContext(t.Context(),
		ResolveContext{CollectionProfile: loaded}, nil)
	require.NoError(t, err)
	require.NotNil(t, profile)
	assert.Equal(t, "Recipe Voice", profile.Name)

	_, err = ResolveProfileFromContext(t.Context(),
		ResolveContext{WorkspaceProfileID: "ws-default"}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"ws-default"`)
	assert.Contains(t, err.Error(), "no brand store")
}

func TestStoreProfileResolver(t *testing.T) {
	store := &mockBrandStore{
		profiles: map[string]*VoiceProfile{
			"test-id": {ID: "test-id", Name: "Test"},
		},
	}
	resolver := &StoreProfileResolver{Store: store}

	profile, err := resolver.ResolveProfile(t.Context(), ResolveContext{
		ExplicitProfileID: "test-id",
	})
	require.NoError(t, err)
	require.NotNil(t, profile)
	assert.Equal(t, "Test", profile.Name)
}
