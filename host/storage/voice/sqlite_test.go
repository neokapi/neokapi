package voice

import (
	"fmt"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	db, err := storage.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	store, err := NewSQLiteStore(db)
	require.NoError(t, err)
	return store
}

func testProfile() *coreprofile.VoiceProfile {
	return &coreprofile.VoiceProfile{
		ID:          "p1",
		Scope:       "ws1",
		Name:        "Friendly Tech",
		Description: "A friendly tech voice profile",
		Tone: coreprofile.ToneProfile{
			Personality: []string{"friendly", "knowledgeable"},
			Formality:   "neutral",
			Emotion:     "warm",
			Humor:       "light",
		},
		Style: coreprofile.StyleRules{
			ActiveVoice:    true,
			SentenceLength: "medium",
			PersonPOV:      "second",
			Contractions:   "sometimes",
		},
		Vocabulary: coreprofile.VocabularyRules{
			PreferredTerms: []coreprofile.TermRule{
				{Term: "use", Replacement: "", Note: "prefer over utilize"},
			},
			ForbiddenTerms: []coreprofile.TermRule{
				{Term: "utilize", Replacement: "use", Severity: "major"},
			},
		},
		Examples: []coreprofile.VoiceExample{
			{Before: "Utilize the feature", After: "Use the feature", Explanation: "simpler"},
		},
		CreatedBy: "test-user",
	}
}

func TestProfileCRUD(t *testing.T) {
	ctx := t.Context()
	store := newTestStore(t)

	profile := testProfile()

	// Create
	require.NoError(t, store.CreateProfile(ctx, profile))
	assert.False(t, profile.CreatedAt.IsZero())
	assert.Equal(t, 1, profile.Version)

	// Get
	got, err := store.GetProfile(ctx, "p1")
	require.NoError(t, err)
	assert.Equal(t, "Friendly Tech", got.Name)
	assert.Equal(t, "neutral", got.Tone.Formality)
	assert.True(t, got.Style.ActiveVoice)
	assert.Len(t, got.Vocabulary.PreferredTerms, 1)
	assert.Len(t, got.Vocabulary.ForbiddenTerms, 1)
	assert.Len(t, got.Examples, 1)
	assert.Equal(t, "test-user", got.CreatedBy)

	// Update
	profile.Name = "Professional Tech"
	profile.Tone.Formality = "formal"
	require.NoError(t, store.UpdateProfile(ctx, profile))
	assert.Equal(t, 2, profile.Version)
	got, err = store.GetProfile(ctx, "p1")
	require.NoError(t, err)
	assert.Equal(t, "Professional Tech", got.Name)
	assert.Equal(t, "formal", got.Tone.Formality)

	// Delete
	require.NoError(t, store.DeleteProfile(ctx, "p1"))
	_, err = store.GetProfile(ctx, "p1")
	require.Error(t, err)
}

func TestProfileNotFound(t *testing.T) {
	ctx := t.Context()
	store := newTestStore(t)

	_, err := store.GetProfile(ctx, "nonexistent")
	require.Error(t, err)

	require.Error(t, store.DeleteProfile(ctx, "nonexistent"))
}

func TestListProfiles(t *testing.T) {
	ctx := t.Context()
	store := newTestStore(t)

	p1 := testProfile()
	p2 := &coreprofile.VoiceProfile{
		ID: "p2", Scope: "ws1", Name: "Casual",
		Tone: coreprofile.ToneProfile{Formality: "casual"},
	}
	p3 := &coreprofile.VoiceProfile{
		ID: "p3", Scope: "ws2", Name: "Other",
		Tone: coreprofile.ToneProfile{Formality: "formal"},
	}

	require.NoError(t, store.CreateProfile(ctx, p1))
	require.NoError(t, store.CreateProfile(ctx, p2))
	require.NoError(t, store.CreateProfile(ctx, p3))

	profiles, err := store.ListProfiles(ctx, "ws1")
	require.NoError(t, err)
	assert.Len(t, profiles, 2)

	profiles, err = store.ListProfiles(ctx, "ws2")
	require.NoError(t, err)
	assert.Len(t, profiles, 1)
}

func TestScoreStorage(t *testing.T) {
	ctx := t.Context()
	store := newTestStore(t)

	score := &coreprofile.StoredScore{
		ID:        "s1",
		ProjectID: "proj1",
		Stream:    "main",
		BlockID:   "block1",
		ProfileID: "p1",
		Locale:    "en-US",
		Score:     85,
		Dimensions: []coreprofile.DimensionScore{
			{Dimension: coreprofile.DimensionTone, Score: 90, Penalty: 1, Issues: 1},
			{Dimension: coreprofile.DimensionStyle, Score: 80, Penalty: 5, Issues: 1},
		},
		Findings: []coreprofile.VoiceFinding{
			{Category: string(coreprofile.DimensionTone), Severity: coreprofile.SeverityMinor, Message: "too informal"},
		},
		CheckedAt: time.Now(),
	}

	require.NoError(t, store.StoreScore(ctx, score))

	scores, err := store.GetScores(ctx, "proj1", "en-US")
	require.NoError(t, err)
	require.Len(t, scores, 1)
	assert.Equal(t, 85, scores[0].Score)
	assert.Len(t, scores[0].Dimensions, 2)
	assert.Len(t, scores[0].Findings, 1)

	// No scores for different locale
	scores, err = store.GetScores(ctx, "proj1", "fr-FR")
	require.NoError(t, err)
	assert.Empty(t, scores)

	// Empty locale means ALL locales (the project-wide read the score
	// endpoints and the brand rollup use), not "rows with an empty locale".
	scores, err = store.GetScores(ctx, "proj1", "")
	require.NoError(t, err)
	assert.Len(t, scores, 1)
}

// TestScoreLocaleNormalization verifies scores are stored and queried under the
// canonical BCP-47 locale, so a score written as "pt-br" is found by a "pt-BR"
// query and vice versa — no silent miss from a casing/format mismatch.
func TestScoreLocaleNormalization(t *testing.T) {
	ctx := t.Context()
	store := newTestStore(t)

	require.NoError(t, store.StoreScore(ctx, &coreprofile.StoredScore{
		ID: "s1", ProjectID: "proj1", Stream: "main", BlockID: "b1",
		ProfileID: "p1", Locale: "pt-br", Score: 70, CheckedAt: time.Now(),
	}))

	// Canonical query finds the non-canonically-written score.
	scores, err := store.GetScores(ctx, "proj1", "pt-BR")
	require.NoError(t, err)
	require.Len(t, scores, 1)
	assert.Equal(t, model.LocaleID("pt-BR"), scores[0].Locale) // stored canonical

	// Non-canonical query also finds it (query side normalizes too).
	scores, err = store.GetScores(ctx, "proj1", "PT-br")
	require.NoError(t, err)
	assert.Len(t, scores, 1)
}

func TestScoreTrends(t *testing.T) {
	ctx := t.Context()
	store := newTestStore(t)

	now := time.Now()
	for i := range 3 {
		require.NoError(t, store.StoreScore(ctx, &coreprofile.StoredScore{
			ID:        fmt.Sprintf("s%d", i),
			ProjectID: "proj1",
			BlockID:   fmt.Sprintf("b%d", i),
			ProfileID: "p1",
			Locale:    "en-US",
			Score:     80 + i*5,
			CheckedAt: now.Add(-time.Duration(i) * 24 * time.Hour),
		}))
	}

	trends, err := store.GetScoreTrends(ctx, "proj1", 30)
	require.NoError(t, err)
	assert.NotEmpty(t, trends)
}

func TestProfileVersioning(t *testing.T) {
	ctx := t.Context()
	store := newTestStore(t)

	profile := testProfile()
	require.NoError(t, store.CreateProfile(ctx, profile))
	assert.Equal(t, 1, profile.Version)

	// Update profile — should archive version 1.
	profile.Name = "Professional Tech"
	profile.Tone.Formality = "formal"
	profile.VersionNote = "switched to formal tone"
	require.NoError(t, store.UpdateProfile(ctx, profile))
	assert.Equal(t, 2, profile.Version)

	// Update again — should archive version 2.
	profile.Name = "Enterprise Tech"
	profile.VersionNote = "enterprise rebrand"
	require.NoError(t, store.UpdateProfile(ctx, profile))
	assert.Equal(t, 3, profile.Version)

	// List versions — should have 2 archived versions (v1 and v2).
	versions, err := store.ListProfileVersions(ctx, "p1")
	require.NoError(t, err)
	require.Len(t, versions, 2)
	assert.Equal(t, 2, versions[0].Version) // DESC order
	assert.Equal(t, 1, versions[1].Version)
	assert.Equal(t, "enterprise rebrand", versions[0].Note)
	assert.Equal(t, "Friendly Tech", versions[1].Snapshot.Name)
	assert.Equal(t, "Professional Tech", versions[0].Snapshot.Name)

	// Get specific version.
	v1, err := store.GetProfileVersion(ctx, "p1", 1)
	require.NoError(t, err)
	assert.Equal(t, "Friendly Tech", v1.Snapshot.Name)
	assert.Equal(t, "neutral", v1.Snapshot.Tone.Formality)

	// Get nonexistent version.
	_, err = store.GetProfileVersion(ctx, "p1", 99)
	require.Error(t, err)
}

func TestProfileTags(t *testing.T) {
	ctx := t.Context()
	store := newTestStore(t)

	profile := testProfile()
	require.NoError(t, store.CreateProfile(ctx, profile))

	// Update to create a version.
	profile.Name = "V2 Name"
	profile.VersionNote = "v2 update"
	require.NoError(t, store.UpdateProfile(ctx, profile))

	// Tag version 1.
	tag := &coreprofile.ProfileTag{
		ProfileID: "p1",
		Name:      "v1-release",
		Version:   1,
		CreatedBy: "test-user",
	}
	require.NoError(t, store.CreateProfileTag(ctx, tag))
	assert.False(t, tag.CreatedAt.IsZero())

	// List tags.
	tags, err := store.ListProfileTags(ctx, "p1")
	require.NoError(t, err)
	require.Len(t, tags, 1)
	assert.Equal(t, "v1-release", tags[0].Name)
	assert.Equal(t, 1, tags[0].Version)

	// Get profile at tag.
	atTag, err := store.GetProfileAtTag(ctx, "p1", "v1-release")
	require.NoError(t, err)
	assert.Equal(t, "Friendly Tech", atTag.Name)

	// Get nonexistent tag.
	_, err = store.GetProfileAtTag(ctx, "p1", "nonexistent")
	require.Error(t, err)

	// Delete tag.
	require.NoError(t, store.DeleteProfileTag(ctx, "p1", "v1-release"))
	tags, err = store.ListProfileTags(ctx, "p1")
	require.NoError(t, err)
	assert.Empty(t, tags)

	// Delete nonexistent tag.
	require.Error(t, store.DeleteProfileTag(ctx, "p1", "nonexistent"))
}

func TestGetScoresByStream(t *testing.T) {
	ctx := t.Context()
	store := newTestStore(t)

	now := time.Now()
	for i := range 3 {
		require.NoError(t, store.StoreScore(ctx, &coreprofile.StoredScore{
			ID:             fmt.Sprintf("s%d", i),
			ProjectID:      "proj1",
			Stream:         "main",
			BlockID:        fmt.Sprintf("b%d", i),
			ProfileID:      "p1",
			ProfileVersion: 1,
			Locale:         "en-US",
			Score:          80 + i*5,
			CheckedAt:      now.Add(-time.Duration(i) * time.Hour),
		}))
	}

	// Add a score on a different stream.
	require.NoError(t, store.StoreScore(ctx, &coreprofile.StoredScore{
		ID:             "s-exp",
		ProjectID:      "proj1",
		Stream:         "experiment",
		BlockID:        "b0",
		ProfileID:      "p1",
		ProfileVersion: 2,
		Locale:         "en-US",
		Score:          92,
		CheckedAt:      now,
	}))

	// Get scores for main stream.
	scores, err := store.GetScoresByStream(ctx, "proj1", "main")
	require.NoError(t, err)
	assert.Len(t, scores, 3)
	assert.Equal(t, 1, scores[0].ProfileVersion)

	// Get scores for experiment stream.
	scores, err = store.GetScoresByStream(ctx, "proj1", "experiment")
	require.NoError(t, err)
	require.Len(t, scores, 1)
	assert.Equal(t, 92, scores[0].Score)
	assert.Equal(t, 2, scores[0].ProfileVersion)
}

func TestScoreProfileVersion(t *testing.T) {
	ctx := t.Context()
	store := newTestStore(t)

	require.NoError(t, store.StoreScore(ctx, &coreprofile.StoredScore{
		ID:             "s1",
		ProjectID:      "proj1",
		Stream:         "main",
		BlockID:        "b1",
		ProfileID:      "p1",
		ProfileVersion: 3,
		Locale:         "en-US",
		Score:          88,
		CheckedAt:      time.Now(),
	}))

	scores, err := store.GetScoresByStream(ctx, "proj1", "main")
	require.NoError(t, err)
	require.Len(t, scores, 1)
	assert.Equal(t, 3, scores[0].ProfileVersion)
}

func TestCorrectionStorage(t *testing.T) {
	ctx := t.Context()
	store := newTestStore(t)

	// Need a profile for the workspace join
	require.NoError(t, store.CreateProfile(ctx, &coreprofile.VoiceProfile{
		ID: "p1", Scope: "ws1", Name: "Test",
		Tone: coreprofile.ToneProfile{Formality: "neutral"},
	}))

	// Store multiple corrections with same original/corrected text
	for i := range 3 {
		require.NoError(t, store.StoreCorrection(ctx, &coreprofile.Correction{
			ID:            fmt.Sprintf("c%d", i),
			ProfileID:     "p1",
			BlockID:       fmt.Sprintf("b%d", i),
			Dimension:     coreprofile.DimensionVocabulary,
			OriginalText:  "utilize",
			CorrectedText: "use",
			CorrectedBy:   "editor",
			CorrectedAt:   time.Now(),
		}))
	}

	rules, err := store.GetSuggestedRules(ctx, "ws1", 2)
	require.NoError(t, err)
	require.Len(t, rules, 1)
	assert.Equal(t, "utilize", rules[0].Term)
	assert.Equal(t, "use", rules[0].Replacement)
	assert.Equal(t, 3, rules[0].CorrectionCount)
	assert.Equal(t, coreprofile.DimensionVocabulary, rules[0].Dimension)

	// With higher threshold: no results
	rules, err = store.GetSuggestedRules(ctx, "ws1", 5)
	require.NoError(t, err)
	assert.Empty(t, rules)
}

func TestRuleDecisions(t *testing.T) {
	ctx := t.Context()
	store := newTestStore(t)
	require.NoError(t, store.CreateProfile(ctx, testProfile()))

	// No decision yet.
	d, err := store.GetRuleDecision(ctx, "p1", "leverage")
	require.NoError(t, err)
	assert.Nil(t, d)

	// Record a rejection.
	now := time.Now().UTC().Truncate(time.Second)
	require.NoError(t, store.RecordRuleDecision(ctx, &coreprofile.RuleDecision{
		ProfileID: "p1", Term: "leverage", Dimension: coreprofile.DimensionVocabulary,
		Status: coreprofile.RuleDecisionRejected, CorrectionCount: 3, DecidedBy: "u1", DecidedAt: now,
	}))

	// Read back, case-insensitively.
	d, err = store.GetRuleDecision(ctx, "p1", "LEVERAGE")
	require.NoError(t, err)
	require.NotNil(t, d)
	assert.Equal(t, coreprofile.RuleDecisionRejected, d.Status)
	assert.Equal(t, 3, d.CorrectionCount)
	assert.Equal(t, "u1", d.DecidedBy)
	assert.False(t, d.Auto)

	// Upsert: the same term promoted (auto) overwrites the rejection.
	require.NoError(t, store.RecordRuleDecision(ctx, &coreprofile.RuleDecision{
		ProfileID: "p1", Term: "leverage", Dimension: coreprofile.DimensionVocabulary,
		Status: coreprofile.RuleDecisionPromoted, CorrectionCount: 5, PromotedVersion: 2,
		Auto: true, DecidedBy: "system", DecidedAt: now,
	}))
	d, err = store.GetRuleDecision(ctx, "p1", "leverage")
	require.NoError(t, err)
	require.NotNil(t, d)
	assert.Equal(t, coreprofile.RuleDecisionPromoted, d.Status)
	assert.Equal(t, 2, d.PromotedVersion)
	assert.True(t, d.Auto)

	// A second decision for another term, then list (one row per term).
	require.NoError(t, store.RecordRuleDecision(ctx, &coreprofile.RuleDecision{
		ProfileID: "p1", Term: "synergy", Status: coreprofile.RuleDecisionApproved, DecidedAt: now,
	}))
	all, err := store.ListRuleDecisions(ctx, "p1")
	require.NoError(t, err)
	assert.Len(t, all, 2)
}

func TestGetProfileMalformedJSON(t *testing.T) {
	ctx := t.Context()
	store := newTestStore(t)

	require.NoError(t, store.CreateProfile(ctx, testProfile()))

	// Corrupt a core descriptor column directly (bypassing the in-code writers,
	// which always emit valid JSON). The reader must surface the unmarshal
	// failure rather than silently returning a half-populated profile, matching
	// the Postgres sibling.
	_, err := store.db.ExecContext(ctx,
		`UPDATE brand_profiles SET tone = ? WHERE id = ?`, "{not json", "p1")
	require.NoError(t, err)

	_, err = store.GetProfile(ctx, "p1")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal tone")
}

func TestProfileAutonomyRoundTrip(t *testing.T) {
	ctx := t.Context()
	store := newTestStore(t)
	p := testProfile()
	p.Autonomy = coreprofile.AutonomyConfig{AutoPromoteAtCount: 5}
	require.NoError(t, store.CreateProfile(ctx, p))

	got, err := store.GetProfile(ctx, "p1")
	require.NoError(t, err)
	assert.Equal(t, 5, got.Autonomy.AutoPromoteAtCount)
}

func TestProfilePersonasRoundTrip(t *testing.T) {
	ctx := t.Context()
	store := newTestStore(t)
	p := testProfile()
	p.Personas = map[string]coreprofile.PersonaOverride{
		"jordan": {
			Tone:      &coreprofile.ToneProfile{Formality: "casual", Humor: "frequent"},
			Preferred: []coreprofile.TermRule{{Term: "let's"}},
			Avoided:   []coreprofile.TermRule{{Term: "synergy"}},
		},
	}
	require.NoError(t, store.CreateProfile(ctx, p))

	got, err := store.GetProfile(ctx, "p1")
	require.NoError(t, err)
	require.Len(t, got.Personas, 1)
	persona := got.Personas["jordan"]
	require.NotNil(t, persona.Tone)
	assert.Equal(t, "frequent", persona.Tone.Humor)
	require.Len(t, persona.Preferred, 1)
	assert.Equal(t, "let's", persona.Preferred[0].Term)
	require.Len(t, persona.Avoided, 1)
	assert.Equal(t, "synergy", persona.Avoided[0].Term)

	// An update preserves personas through the version/round-trip path.
	got.Personas["jordan"] = coreprofile.PersonaOverride{
		Avoided: []coreprofile.TermRule{{Term: "leverage"}},
	}
	require.NoError(t, store.UpdateProfile(ctx, got))
	updated, err := store.GetProfile(ctx, "p1")
	require.NoError(t, err)
	require.Len(t, updated.Personas["jordan"].Avoided, 1)
	assert.Equal(t, "leverage", updated.Personas["jordan"].Avoided[0].Term)
}
