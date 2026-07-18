package jobs

import (
	"testing"
	"time"

	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModelSweepStore_UpsertAndQueries(t *testing.T) {
	db := pgtest.NewTestDB(t)
	s, err := NewModelSweepStore(db)
	require.NoError(t, err)
	ctx := t.Context()

	now := time.Now().UTC().Truncate(time.Second)
	seed := []*ModelSweepResult{
		{ProjectID: "p1", Locale: "fr", Model: "m1", FixtureDigest: "d1",
			FixtureCount: 12, Adherence: 0.6, AdherenceBare: 0.5, Lift: 0.1,
			TokensUsed: 100, MeasuredAt: now.Add(-48 * time.Hour)},
		// A newer measurement of the same model under a fresh digest: the
		// latest-per-model query must prefer it.
		{ProjectID: "p1", Locale: "fr", Model: "m1", FixtureDigest: "d2",
			FixtureCount: 14, Adherence: 0.9, AdherenceBare: 0.5, Lift: 0.4,
			TokensUsed: 120, MeasuredAt: now},
		{ProjectID: "p1", Locale: "fr", Model: "m2", FixtureDigest: "d2",
			FixtureCount: 14, Adherence: 0.8, AdherenceBare: 0.6, Lift: 0.2,
			TokensUsed: 110, MeasuredAt: now},
		{ProjectID: "p1", Locale: "de", Model: "m1", FixtureDigest: "d3",
			FixtureCount: 14, Adherence: 0.75, AdherenceBare: 0.7, Lift: 0.05,
			TokensUsed: 90, MeasuredAt: now},
		// Another project must never leak into p1's results.
		{ProjectID: "p2", Locale: "fr", Model: "m9", FixtureDigest: "d9",
			FixtureCount: 14, Adherence: 1, AdherenceBare: 0, Lift: 1,
			TokensUsed: 10, MeasuredAt: now},
	}
	for _, r := range seed {
		require.NoError(t, s.UpsertModelSweepResult(ctx, r))
	}

	// Upsert refreshes the same (project, locale, model, digest) key in place.
	require.NoError(t, s.UpsertModelSweepResult(ctx, &ModelSweepResult{
		ProjectID: "p1", Locale: "fr", Model: "m2", FixtureDigest: "d2",
		FixtureCount: 14, Adherence: 0.85, AdherenceBare: 0.6, Lift: 0.25,
		TokensUsed: 111, MeasuredAt: now.Add(time.Minute),
	}))

	latest, err := s.LatestModelSweepResults(ctx, "p1", "fr")
	require.NoError(t, err)
	require.Len(t, latest, 2, "one latest row per model")
	// Ordered by lift descending.
	assert.Equal(t, "m1", latest[0].Model)
	assert.Equal(t, "d2", latest[0].FixtureDigest, "the newer digest's measurement wins")
	assert.InDelta(t, 0.4, latest[0].Lift, 1e-9)
	assert.Equal(t, "m2", latest[1].Model)
	assert.InDelta(t, 0.25, latest[1].Lift, 1e-9, "upsert refreshed the row in place")

	// The recommendation policy over the latest rows picks the lift winner.
	best := RecommendModel(latest)
	require.NotNil(t, best)
	assert.Equal(t, "m1", best.Model)

	all, err := s.ListModelSweepResults(ctx, "p1")
	require.NoError(t, err)
	require.Len(t, all, 3, "latest per (locale, model), p2 never leaks")
	assert.Equal(t, "de", all[0].Locale, "locales are grouped ascending")
	for _, r := range all {
		assert.Equal(t, "p1", r.ProjectID)
	}
}
