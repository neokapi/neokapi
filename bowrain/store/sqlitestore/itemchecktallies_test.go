package sqlitestore

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
)

// A tally is one row per item of a stream: recording it again replaces it,
// another stream and another project hold their own, and an empty stream is
// the main one.
func TestItemCheckTallies_OnePerItem(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)
	other := createTestProject(t, s)
	at := time.Date(2026, 10, 31, 9, 0, 0, 0, time.UTC)

	none, err := s.ListItemCheckTallies(ctx, p.ID, "main")
	require.NoError(t, err)
	assert.Empty(t, none)

	require.NoError(t, s.PutItemCheckTally(ctx, platstore.ItemCheckTally{ProjectID: p.ID, ItemID: "item-b", Findings: 3, Fixable: 1, CheckedAt: at}))
	require.NoError(t, s.PutItemCheckTally(ctx, platstore.ItemCheckTally{ProjectID: p.ID, Stream: "main", ItemID: "item-a", Findings: 0, CheckedAt: at}))
	require.NoError(t, s.PutItemCheckTally(ctx, platstore.ItemCheckTally{ProjectID: p.ID, Stream: "main", ItemID: "item-b", Findings: 1, Fixable: 1, CheckedAt: at.Add(time.Hour)}))
	require.NoError(t, s.PutItemCheckTally(ctx, platstore.ItemCheckTally{ProjectID: p.ID, Stream: "draft", ItemID: "item-a", Findings: 7, CheckedAt: at}))

	got, err := s.ListItemCheckTallies(ctx, p.ID, "")
	require.NoError(t, err)
	assert.Equal(t, []platstore.ItemCheckTally{
		{ProjectID: p.ID, Stream: "main", ItemID: "item-a", Findings: 0, CheckedAt: at},
		{ProjectID: p.ID, Stream: "main", ItemID: "item-b", Findings: 1, Fixable: 1, CheckedAt: at.Add(time.Hour)},
	}, got, "recording again replaces the tally")

	draft, err := s.ListItemCheckTallies(ctx, p.ID, "draft")
	require.NoError(t, err)
	require.Len(t, draft, 1)
	assert.Equal(t, 7, draft[0].Findings)

	elsewhere, err := s.ListItemCheckTallies(ctx, other.ID, "main")
	require.NoError(t, err)
	assert.Empty(t, elsewhere, "another project holds its own tallies")

	require.Error(t, s.PutItemCheckTally(ctx, platstore.ItemCheckTally{ProjectID: p.ID}), "a tally names its item")
}
