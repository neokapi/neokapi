package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
)

// latestChangeBlockIDs lists the blocks change entries name, in order.
func latestChangeBlockIDs(changes []platstore.ChangeEntry) []string {
	ids := make([]string, 0, len(changes))
	for _, c := range changes {
		ids = append(ids, c.BlockID)
	}
	return ids
}

// seedTargetsAfterSources stores b1, b2 and b3, then a Norwegian target for b1
// and b2, so those two have a later change than b3.
func seedTargetsAfterSources(t *testing.T, s *PostgresStore, projectID string) {
	t.Helper()
	ctx := t.Context()
	require.NoError(t, s.StoreBlocks(ctx, projectID, "", []*model.Block{
		model.NewBlock("b1", "One"), model.NewBlock("b2", "Two"), model.NewBlock("b3", "Three"),
	}))
	one, two := model.NewBlock("b1", "One"), model.NewBlock("b2", "Two")
	one.SetTargetText("nb", "En")
	two.SetTargetText("nb", "To")
	require.NoError(t, s.StoreBlocks(ctx, projectID, "", []*model.Block{one, two}))
}

// Each block has one entry, at its latest change within the locale scope, and
// the entries are ordered by that change.
func TestGetLatestChanges_OneEntryPerBlockAtItsLatestChange(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)
	seedTargetsAfterSources(t, s, p.ID)

	all, err := s.GetChanges(ctx, p.ID, "", 0, nil, 100)
	require.NoError(t, err)
	require.Len(t, all.Changes, 5, "three source changes and two target changes")

	latest, err := s.GetLatestChanges(ctx, p.ID, "", 0, nil, 100)
	require.NoError(t, err)
	assert.Equal(t, []string{"b3", "b1", "b2"}, latestChangeBlockIDs(latest.Changes))
	assert.Equal(t, all.NewCursor, latest.NewCursor)
	assert.False(t, latest.HasMore)

	german, err := s.GetLatestChanges(ctx, p.ID, "", 0, []string{"de"}, 100)
	require.NoError(t, err)
	assert.Equal(t, []string{"b1", "b2", "b3"}, latestChangeBlockIDs(german.Changes),
		"a target outside the locale scope leaves a block's entry at its source change")
}

// Paging one entry at a time serves each block once, in the order of its
// latest change.
func TestGetLatestChanges_PagesServeEachBlockOnce(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)
	seedTargetsAfterSources(t, s, p.ID)

	var got []string
	cursor := int64(0)
	for pages := 0; ; pages++ {
		require.Less(t, pages, 10, "the paging ends")
		cs, err := s.GetLatestChanges(ctx, p.ID, "", cursor, nil, 1)
		require.NoError(t, err)
		got = append(got, latestChangeBlockIDs(cs.Changes)...)
		cursor = cs.NewCursor
		if !cs.HasMore {
			break
		}
	}
	assert.Equal(t, []string{"b3", "b1", "b2"}, got)
}
