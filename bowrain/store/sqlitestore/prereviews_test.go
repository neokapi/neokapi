package sqlitestore

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
)

// A pre-review is one row per translation: newer advice takes the place of
// older, a stream keeps its own, and a read names the blocks it wants.
func TestPreReviews_OneRowPerTranslation(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)
	at := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)

	require.NoError(t, s.RecordPreReview(ctx, p.ID, "main", platstore.PreReview{
		BlockID: "b1", Locale: "fr", Score: 40, Reviewer: "agent/one", Reasons: []string{"stiff"}, Revision: "r:1", At: at,
	}))
	require.NoError(t, s.RecordPreReview(ctx, p.ID, "main", platstore.PreReview{
		BlockID: "b1", Locale: "fr", Score: 85, Reviewer: "agent/two", Revision: "r:2", At: at.Add(time.Minute),
	}))
	require.NoError(t, s.RecordPreReview(ctx, p.ID, "main", platstore.PreReview{
		BlockID: "b1", Locale: "de", Score: 60, Reviewer: "agent/one", Revision: "r:3", At: at,
	}))
	require.NoError(t, s.RecordPreReview(ctx, p.ID, "main", platstore.PreReview{
		BlockID: "b2", Locale: "fr", Score: 10, Reviewer: "agent/one", Revision: "r:4", At: at,
	}))

	got, err := s.PreReviews(ctx, p.ID, "main", []string{"b1"})
	require.NoError(t, err)
	require.Len(t, got, 2, "the newer advice on fr took the older one's place")
	assert.Equal(t, "de", got[0].Locale)
	assert.Equal(t, platstore.PreReview{
		BlockID: "b1", Locale: "fr", Score: 85, Reviewer: "agent/two", Reasons: []string{}, Revision: "r:2", At: at.Add(time.Minute),
	}, got[1])

	other, err := s.PreReviews(ctx, p.ID, "feature", []string{"b1", "b2"})
	require.NoError(t, err)
	assert.Empty(t, other, "another stream keeps its own")

	none, err := s.PreReviews(ctx, p.ID, "main", nil)
	require.NoError(t, err)
	assert.Empty(t, none)
}
