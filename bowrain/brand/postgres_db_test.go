package brand

// Real Postgres-backed coverage for PostgresBrandStore. These exercise the
// store against a throwaway postgres:16-alpine testcontainer (pgtest), applying
// the brand migrations via NewPostgresBrandStore, and pin four behaviours the
// thin marshaling tests never touched: profile version archiving, the
// rule-decision promotion round-trip, profile tags, and GetScores' empty-locale
// contract. Skipped in -short unless BOWRAIN_TEST_POSTGRES_URL is set.

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	corebrand "github.com/neokapi/neokapi/core/brand"
	"github.com/neokapi/neokapi/core/model"
)

// newBrandStore returns a PostgresBrandStore on a fresh, isolated schema with
// the brand migrations already applied (NewPostgresBrandStore runs them).
func newBrandStore(t *testing.T) *PostgresBrandStore {
	t.Helper()
	db := pgtest.NewTestDB(t) // skips under -short without BOWRAIN_TEST_POSTGRES_URL
	store, err := NewPostgresBrandStore(db)
	require.NoError(t, err)
	return store
}

// newTestProfile builds a minimal but valid VoiceProfile. CreateProfile fills
// ID, Version, and timestamps.
func newTestProfile(ws, name string) *corebrand.VoiceProfile {
	return &corebrand.VoiceProfile{
		WorkspaceID: ws,
		Name:        name,
		Description: "desc",
		Tone:        corebrand.ToneProfile{Personality: []string{"friendly"}, Formality: "neutral"},
		Style:       corebrand.StyleRules{ActiveVoice: true, SentenceLength: "medium"},
		Vocabulary:  corebrand.VocabularyRules{},
		Examples:    []corebrand.VoiceExample{},
		CreatedBy:   "tester",
	}
}

// TestPostgresBrandStore_ProfileVersioning pins that UpdateProfile archives the
// pre-edit state as an immutable ProfileVersion and bumps the live version, and
// that the archived row carries the note describing the edit that superseded it.
func TestPostgresBrandStore_ProfileVersioning(t *testing.T) {
	store := newBrandStore(t)
	ctx := t.Context()

	p := newTestProfile("ws-ver", "Acme")
	require.NoError(t, store.CreateProfile(ctx, p))
	require.NotEmpty(t, p.ID)
	require.Equal(t, 1, p.Version, "CreateProfile pins the baseline at version 1")

	got, err := store.GetProfile(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, got.Version)
	assert.Equal(t, "Acme", got.Name)

	// No versions archived until the first edit.
	versions, err := store.ListProfileVersions(ctx, p.ID)
	require.NoError(t, err)
	assert.Empty(t, versions)

	// First edit: rename and bump. The prior state (name "Acme", version 1) is
	// archived; the live profile becomes version 2.
	p.Name = "Acme Renamed"
	p.VersionNote = "first edit"
	require.NoError(t, store.UpdateProfile(ctx, p))
	assert.Equal(t, 2, p.Version, "UpdateProfile bumps the in-memory version to 2")

	got, err = store.GetProfile(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, got.Version)
	assert.Equal(t, "Acme Renamed", got.Name)

	versions, err = store.ListProfileVersions(ctx, p.ID)
	require.NoError(t, err)
	require.Len(t, versions, 1)
	v1 := versions[0]
	assert.Equal(t, 1, v1.Version)
	assert.Equal(t, "Acme", v1.Snapshot.Name, "archive holds the PRE-edit snapshot")
	assert.Equal(t, 1, v1.Snapshot.Version)
	// The note on the archived version is the note describing the edit that
	// replaced it (UpdateProfile records profile.VersionNote against the row it
	// is archiving), not a note authored when version 1 was created.
	assert.Equal(t, "first edit", v1.Note)

	// GetProfileVersion returns the same archived snapshot by number.
	pv1, err := store.GetProfileVersion(ctx, p.ID, 1)
	require.NoError(t, err)
	assert.Equal(t, "Acme", pv1.Snapshot.Name)

	// Second edit: version 2's state is archived, live becomes version 3.
	p.Name = "Acme Rebrand"
	p.VersionNote = "second edit"
	require.NoError(t, store.UpdateProfile(ctx, p))
	assert.Equal(t, 3, p.Version)

	versions, err = store.ListProfileVersions(ctx, p.ID)
	require.NoError(t, err)
	require.Len(t, versions, 2)
	assert.Equal(t, 2, versions[0].Version, "ListProfileVersions is newest-version-first")
	assert.Equal(t, 1, versions[1].Version)
	assert.Equal(t, "Acme Renamed", versions[0].Snapshot.Name)
	assert.Equal(t, "second edit", versions[0].Note)

	// Missing version and missing profile are surfaced as errors.
	_, err = store.GetProfileVersion(ctx, p.ID, 99)
	require.Error(t, err)

	orphan := newTestProfile("ws-ver", "Ghost")
	orphan.ID = "does-not-exist"
	err = store.UpdateProfile(ctx, orphan)
	require.Error(t, err, "UpdateProfile of an unknown profile fails at the versioning read")
}

// TestPostgresBrandStore_RuleDecisions pins the rule-decision persistence
// contract: RecordRuleDecision upserts on (profile_id, term), GetRuleDecision
// matches the term case-insensitively and returns (nil, nil) when absent, and
// ListRuleDecisions is ordered newest-decision-first.
func TestPostgresBrandStore_RuleDecisions(t *testing.T) {
	store := newBrandStore(t)
	ctx := t.Context()

	p := newTestProfile("ws-dec", "Acme")
	require.NoError(t, store.CreateProfile(ctx, p))

	t0 := time.Now().UTC().Truncate(time.Second)

	// Record an approved-but-not-yet-promoted decision.
	require.NoError(t, store.RecordRuleDecision(ctx, &corebrand.RuleDecision{
		ProfileID:       p.ID,
		Term:            "Utilize",
		Replacement:     "use",
		Dimension:       corebrand.DimensionVocabulary,
		Status:          corebrand.RuleDecisionApproved,
		CorrectionCount: 3,
		DecidedBy:       "reviewer",
		DecidedAt:       t0.Add(-time.Minute),
	}))

	// GetRuleDecision matches term case-insensitively (stored "Utilize", queried "utilize").
	got, err := store.GetRuleDecision(ctx, p.ID, "utilize")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, corebrand.RuleDecisionApproved, got.Status)
	assert.Equal(t, corebrand.DimensionVocabulary, got.Dimension)
	assert.Equal(t, 3, got.CorrectionCount)

	// Promote the same term: the upsert on (profile_id, term) overwrites in place
	// rather than inserting a second row.
	require.NoError(t, store.RecordRuleDecision(ctx, &corebrand.RuleDecision{
		ProfileID:       p.ID,
		Term:            "Utilize",
		Replacement:     "use",
		Dimension:       corebrand.DimensionVocabulary,
		Status:          corebrand.RuleDecisionPromoted,
		CorrectionCount: 5,
		PromotedVersion: 2,
		ConceptID:       "concept-1",
		DecidedBy:       "reviewer",
		DecidedAt:       t0,
	}))

	got, err = store.GetRuleDecision(ctx, p.ID, "Utilize")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, corebrand.RuleDecisionPromoted, got.Status, "upsert overwrote the earlier decision")
	assert.Equal(t, 2, got.PromotedVersion)
	assert.Equal(t, 5, got.CorrectionCount)
	assert.Equal(t, "concept-1", got.ConceptID)

	// A second, distinct term coexists; unknown term returns (nil, nil).
	require.NoError(t, store.RecordRuleDecision(ctx, &corebrand.RuleDecision{
		ProfileID: p.ID,
		Term:      "leverage",
		Status:    corebrand.RuleDecisionRejected,
		DecidedBy: "reviewer",
		DecidedAt: t0.Add(time.Minute),
	}))

	missing, err := store.GetRuleDecision(ctx, p.ID, "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, missing, "absent decision is (nil, nil), not an error")

	list, err := store.ListRuleDecisions(ctx, p.ID)
	require.NoError(t, err)
	require.Len(t, list, 2, "one row per (profile, term); the upsert did not add a duplicate")
	// Ordered by decided_at DESC: "leverage" (t0+1m) before "Utilize" (t0).
	assert.Equal(t, "leverage", list[0].Term)
	assert.Equal(t, "Utilize", list[1].Term)
}

// TestPostgresBrandStore_PromotionClosedLoop drives the full correction→promotion
// round-trip through the real store: corrections aggregate into a suggested rule,
// PromoteAndSave applies it (bumping the profile version and archiving the prior),
// the decision is recorded at that version, and GetSuggestedRules back-fills the
// concept a promoted/decided term already denotes.
func TestPostgresBrandStore_PromotionClosedLoop(t *testing.T) {
	store := newBrandStore(t)
	ctx := t.Context()

	p := newTestProfile("ws-loop", "Acme")
	require.NoError(t, store.CreateProfile(ctx, p))

	// Three corrections of the same utilize→use edit.
	for i := 0; i < 3; i++ {
		require.NoError(t, store.StoreCorrection(ctx, &corebrand.Correction{
			ProfileID:     p.ID,
			BlockID:       "block-1",
			Dimension:     corebrand.DimensionVocabulary,
			OriginalText:  "utilize",
			CorrectedText: "use",
			CorrectedBy:   "editor",
		}))
	}

	suggestions, err := store.GetSuggestedRules(ctx, "ws-loop", 2)
	require.NoError(t, err)
	require.Len(t, suggestions, 1)
	assert.Equal(t, "utilize", suggestions[0].Term)
	assert.Equal(t, "use", suggestions[0].Replacement)
	assert.Equal(t, 3, suggestions[0].CorrectionCount)

	// Promote the reviewed suggestion (concept-backed). PromoteAndSave adds the
	// forbidden term and bumps + archives the profile via the store.
	rule := corebrand.SuggestedRule{
		Term: "utilize", Replacement: "use", CorrectionCount: 3,
		Dimension: corebrand.DimensionVocabulary, ConceptID: "concept-42",
	}
	updated, changed, err := corebrand.PromoteAndSave(ctx, store, p.ID, rule)
	require.NoError(t, err)
	require.True(t, changed)
	assert.Equal(t, 2, updated.Version, "promotion bumps the live profile to version 2")

	reloaded, err := store.GetProfile(ctx, p.ID)
	require.NoError(t, err)
	require.Len(t, reloaded.Vocabulary.ForbiddenTerms, 1)
	assert.Equal(t, "utilize", reloaded.Vocabulary.ForbiddenTerms[0].Term)
	assert.Equal(t, "use", reloaded.Vocabulary.ForbiddenTerms[0].Replacement)
	assert.Equal(t, "concept-42", reloaded.Vocabulary.ForbiddenTerms[0].ConceptID)

	// Record the governance decision at the version it landed in.
	require.NoError(t, store.RecordRuleDecision(ctx, &corebrand.RuleDecision{
		ProfileID:       p.ID,
		Term:            "utilize",
		Replacement:     "use",
		Dimension:       corebrand.DimensionVocabulary,
		Status:          corebrand.RuleDecisionPromoted,
		CorrectionCount: 3,
		PromotedVersion: updated.Version,
		ConceptID:       "concept-42",
		DecidedBy:       "reviewer",
	}))
	decision, err := store.GetRuleDecision(ctx, p.ID, "utilize")
	require.NoError(t, err)
	require.NotNil(t, decision)
	assert.Equal(t, corebrand.RuleDecisionPromoted, decision.Status)
	assert.Equal(t, updated.Version, decision.PromotedVersion)

	// The candidate still surfaces (corrections remain), now with its concept
	// back-filled from the live profile vocabulary / durable decision (AD-021).
	suggestions, err = store.GetSuggestedRules(ctx, "ws-loop", 2)
	require.NoError(t, err)
	require.Len(t, suggestions, 1)
	assert.Equal(t, "concept-42", suggestions[0].ConceptID, "GetSuggestedRules back-fills the term's concept")
}

// TestPostgresBrandStore_Tags pins the profile-tag CRUD and its dependency on an
// archived version: a tag names a specific ProfileVersion, GetProfileAtTag
// resolves it to that snapshot, duplicate/missing tags error, and a tag pointing
// at a version that was never archived surfaces "profile version not found".
func TestPostgresBrandStore_Tags(t *testing.T) {
	store := newBrandStore(t)
	ctx := t.Context()

	p := newTestProfile("ws-tag", "Acme")
	require.NoError(t, store.CreateProfile(ctx, p))

	// Edit once so version 1 gets archived (tags can only resolve archived versions).
	p.Name = "Acme v2"
	p.VersionNote = "edit"
	require.NoError(t, store.UpdateProfile(ctx, p)) // live is now version 2, version 1 archived

	require.NoError(t, store.CreateProfileTag(ctx, &corebrand.ProfileTag{
		ProfileID: p.ID, Name: "launch", Version: 1, CreatedBy: "tester",
	}))

	tags, err := store.ListProfileTags(ctx, p.ID)
	require.NoError(t, err)
	require.Len(t, tags, 1)
	assert.Equal(t, "launch", tags[0].Name)
	assert.Equal(t, 1, tags[0].Version)

	// GetProfileAtTag resolves the tag to the archived version-1 snapshot.
	atTag, err := store.GetProfileAtTag(ctx, p.ID, "launch")
	require.NoError(t, err)
	assert.Equal(t, "Acme", atTag.Name, "tag resolves to the archived snapshot, not the live profile")

	// Duplicate tag name is rejected (composite PK on profile_id, name).
	err = store.CreateProfileTag(ctx, &corebrand.ProfileTag{ProfileID: p.ID, Name: "launch", Version: 1})
	require.Error(t, err)

	// Unknown tag and a tag pointing at a never-archived version both error.
	_, err = store.GetProfileAtTag(ctx, p.ID, "missing")
	require.Error(t, err)

	require.NoError(t, store.CreateProfileTag(ctx, &corebrand.ProfileTag{
		ProfileID: p.ID, Name: "tip", Version: 2, CreatedBy: "tester",
	}))
	_, err = store.GetProfileAtTag(ctx, p.ID, "tip")
	require.Error(t, err, "the live version is not archived yet, so the tag cannot resolve a snapshot")

	// Delete is idempotent-checked: first delete succeeds, second reports missing.
	require.NoError(t, store.DeleteProfileTag(ctx, p.ID, "launch"))
	tags, err = store.ListProfileTags(ctx, p.ID)
	require.NoError(t, err)
	require.Len(t, tags, 1)
	assert.Equal(t, "tip", tags[0].Name)

	err = store.DeleteProfileTag(ctx, p.ID, "launch")
	require.Error(t, err, "deleting an already-removed tag reports not found")
}

// storeScore is a small helper that persists one StoredScore with explicit id,
// locale, and checked_at so ordering and locale filtering are deterministic.
func storeScore(t *testing.T, store *PostgresBrandStore, project, id, locale string, checkedAt time.Time) {
	t.Helper()
	require.NoError(t, store.StoreScore(t.Context(), &corebrand.StoredScore{
		ID:        id,
		ProjectID: project,
		BlockID:   "block-" + id,
		ProfileID: "profile-1",
		Locale:    model.LocaleID(locale),
		Score:     90,
		Dimensions: []corebrand.DimensionScore{
			{Dimension: corebrand.DimensionVocabulary, Score: 90},
		},
		Findings:  []corebrand.BrandVoiceFinding{},
		CheckedAt: checkedAt,
	}))
}

// TestPostgresBrandStore_GetScoresEmptyLocale pins the recently-fixed empty-locale
// contract: GetScores with an empty locale means ALL locales (the project-wide
// read), newest-first — including a row that happens to be stored with an empty
// locale — and is NOT "only rows stored with an empty locale". A non-empty locale
// filters (case-insensitively, at the normalized boundary), and results are
// scoped to the project.
func TestPostgresBrandStore_GetScoresEmptyLocale(t *testing.T) {
	store := newBrandStore(t)
	ctx := t.Context()

	base := time.Now().UTC().Truncate(time.Second)
	const proj = "proj-A"

	// Five scores in one project across distinct locales (including an
	// empty-locale row) and one score in a second project.
	storeScore(t, store, proj, "s1", "fr-FR", base.Add(-3*time.Minute))
	storeScore(t, store, proj, "s2", "fr-FR", base.Add(-2*time.Minute))
	storeScore(t, store, proj, "s3", "de-DE", base.Add(-1*time.Minute))
	storeScore(t, store, proj, "s4", "", base) // stored with an empty locale
	storeScore(t, store, proj, "s5", "en-US", base.Add(time.Minute))
	storeScore(t, store, "proj-B", "b1", "fr-FR", base.Add(time.Hour))

	// Empty locale => ALL locales in the project, newest checked_at first.
	all, err := store.GetScores(ctx, proj, "")
	require.NoError(t, err)
	require.Len(t, all, 5, "empty locale returns every locale in the project, including the empty-locale row")
	gotOrder := []string{all[0].ID, all[1].ID, all[2].ID, all[3].ID, all[4].ID}
	assert.Equal(t, []string{"s5", "s4", "s3", "s2", "s1"}, gotOrder, "newest checked_at first")

	// A concrete locale filters to just that locale's rows.
	fr, err := store.GetScores(ctx, proj, "fr-FR")
	require.NoError(t, err)
	require.Len(t, fr, 2)
	assert.Equal(t, []string{"s2", "s1"}, []string{fr[0].ID, fr[1].ID})

	de, err := store.GetScores(ctx, proj, "de-DE")
	require.NoError(t, err)
	require.Len(t, de, 1)
	assert.Equal(t, "s3", de[0].ID)

	// Locale lookup normalizes: querying "en-us" matches the stored "en-US".
	en, err := store.GetScores(ctx, proj, "en-us")
	require.NoError(t, err)
	require.Len(t, en, 1)
	assert.Equal(t, "s5", en[0].ID)

	// A locale with no rows returns empty (not the project-wide set).
	none, err := store.GetScores(ctx, proj, "nb-NO")
	require.NoError(t, err)
	assert.Empty(t, none)

	// The empty-locale, project-wide read does not leak across projects.
	other, err := store.GetScores(ctx, "proj-B", "")
	require.NoError(t, err)
	require.Len(t, other, 1)
	assert.Equal(t, "b1", other[0].ID)
}

// TestPostgresBrandStore_ScoresByStreamAndTrends exercises the stream default and
// the trend aggregation on real rows: StoreScore defaults an empty stream to
// "main", GetScoresByStream filters by stream, and GetScoreTrends buckets by day.
func TestPostgresBrandStore_ScoresByStreamAndTrends(t *testing.T) {
	store := newBrandStore(t)
	ctx := t.Context()

	now := time.Now().UTC()
	const proj = "proj-stream"

	// Two "main" scores (empty stream defaults to "main") and one "draft" score.
	storeScore(t, store, proj, "m1", "fr-FR", now.Add(-2*time.Minute))
	storeScore(t, store, proj, "m2", "fr-FR", now.Add(-time.Minute))
	require.NoError(t, store.StoreScore(ctx, &corebrand.StoredScore{
		ID: "d1", ProjectID: proj, Stream: "draft", BlockID: "b", ProfileID: "profile-1",
		Locale: "fr-FR", Score: 70,
		Dimensions: []corebrand.DimensionScore{}, Findings: []corebrand.BrandVoiceFinding{},
		CheckedAt: now,
	}))

	main, err := store.GetScoresByStream(ctx, proj, "main")
	require.NoError(t, err)
	require.Len(t, main, 2, "empty stream is stored as 'main'")
	assert.Equal(t, "main", main[0].Stream)

	draft, err := store.GetScoresByStream(ctx, proj, "draft")
	require.NoError(t, err)
	require.Len(t, draft, 1)
	assert.Equal(t, "d1", draft[0].ID)

	// Trends aggregate all three of today's scores into day buckets.
	trends, err := store.GetScoreTrends(ctx, proj, 7)
	require.NoError(t, err)
	require.NotEmpty(t, trends)
	total := 0
	for _, tr := range trends {
		total += tr.Count
	}
	assert.Equal(t, 3, total, "every score within the window is counted in a day bucket")
}
