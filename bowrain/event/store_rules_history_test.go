package event

import (
	"testing"
	"time"

	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/id"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestRuleStore(t *testing.T) *RuleStore {
	t.Helper()
	db := pgtest.NewTestDB(t)
	_, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	return NewRuleStore(db.DB)
}

// recordHistory writes n entries for a project, all at startedAt, so a page
// boundary lands inside a group of rows sharing one timestamp.
func recordHistory(t *testing.T, s *RuleStore, projectID string, n int, startedAt time.Time) {
	t.Helper()
	for range n {
		require.NoError(t, s.RecordExecution(t.Context(), &HistoryEntry{
			ID:        id.New(),
			RuleID:    "rule-1",
			ProjectID: projectID,
			Status:    "success",
			StartedAt: startedAt,
			EndedAt:   startedAt,
		}))
	}
}

func TestRuleStoreListHistory_Paging(t *testing.T) {
	s := newTestRuleStore(t)
	ctx := t.Context()

	// Two batches an hour apart, each fanned out within one instant — the shape
	// one event triggering several rules produces.
	older := time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond)
	newer := time.Now().UTC().Truncate(time.Microsecond)
	recordHistory(t, s, "proj-1", 4, older)
	recordHistory(t, s, "proj-1", 4, newer)
	recordHistory(t, s, "proj-2", 3, newer)

	t.Run("walks every entry exactly once across pages", func(t *testing.T) {
		seen := map[string]int{}
		cursor := ""
		for pages := 0; ; pages++ {
			require.Less(t, pages, 10, "paging did not terminate")
			page, err := s.ListHistory(ctx, HistoryQuery{ProjectID: "proj-1", Limit: 3, Cursor: cursor})
			require.NoError(t, err)
			for _, e := range page.Entries {
				seen[e.ID]++
			}
			if page.NextCursor == "" {
				break
			}
			assert.Len(t, page.Entries, 3)
			cursor = page.NextCursor
		}
		assert.Len(t, seen, 8)
		for entryID, n := range seen {
			assert.Equal(t, 1, n, "entry %s appeared more than once", entryID)
		}
	})

	t.Run("newest first", func(t *testing.T) {
		page, err := s.ListHistory(ctx, HistoryQuery{ProjectID: "proj-1", Limit: 4})
		require.NoError(t, err)
		require.Len(t, page.Entries, 4)
		for _, e := range page.Entries {
			assert.Equal(t, newer, e.StartedAt.UTC())
		}
		assert.NotEmpty(t, page.NextCursor)
	})

	t.Run("scoped to the project", func(t *testing.T) {
		page, err := s.ListHistory(ctx, HistoryQuery{ProjectID: "proj-2", Limit: 50})
		require.NoError(t, err)
		assert.Len(t, page.Entries, 3)
		assert.Empty(t, page.NextCursor)
	})

	t.Run("last page carries no cursor", func(t *testing.T) {
		page, err := s.ListHistory(ctx, HistoryQuery{ProjectID: "proj-1", Limit: 8})
		require.NoError(t, err)
		assert.Len(t, page.Entries, 8)
		assert.Empty(t, page.NextCursor)
	})

	t.Run("an unparseable cursor reads as the first page", func(t *testing.T) {
		page, err := s.ListHistory(ctx, HistoryQuery{ProjectID: "proj-1", Limit: 3, Cursor: "nonsense"})
		require.NoError(t, err)
		assert.Len(t, page.Entries, 3)
		assert.NotEmpty(t, page.NextCursor)
	})

	t.Run("limit is clamped", func(t *testing.T) {
		page, err := s.ListHistory(ctx, HistoryQuery{ProjectID: "proj-1", Limit: 10_000})
		require.NoError(t, err)
		assert.Len(t, page.Entries, 8)
	})
}
