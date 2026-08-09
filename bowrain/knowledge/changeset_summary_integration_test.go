//go:build integration

package knowledge

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestListChangeSetSummariesCountsOps proves the list answers "N changes" from
// its own query — the reason a dashboard no longer reads every change-set's
// detail to render a row.
func TestListChangeSetSummariesCountsOps(t *testing.T) {
	store := newTestStore(t)
	ctx := t.Context()

	empty := &ChangeSet{WorkspaceID: testWS, Name: "empty", CreatedBy: "alice"}
	require.NoError(t, store.CreateChangeSet(ctx, empty))
	full := &ChangeSet{WorkspaceID: testWS, Name: "full", CreatedBy: "alice"}
	require.NoError(t, store.CreateChangeSet(ctx, full))
	for range 3 {
		require.NoError(t, store.AppendOp(ctx, &ChangeSetOp{
			WorkspaceID: testWS, ChangesetID: full.ID, Op: OpConceptDelete,
			Payload: mustJSON(t, ConceptDeletePayload{ConceptID: "c1"}), CreatedBy: "alice",
		}))
	}

	summaries, err := store.ListChangeSetSummaries(ctx, testWS, "")
	require.NoError(t, err)
	counts := map[string]int{}
	for _, cs := range summaries {
		counts[cs.Name] = cs.OpsCount
	}
	assert.Equal(t, map[string]int{"empty": 0, "full": 3}, counts)

	// The status filter narrows the same query.
	require.NoError(t, store.SetChangeSetStatus(ctx, testWS, full.ID, ChangeSetInReview))
	inReview, err := store.ListChangeSetSummaries(ctx, testWS, ChangeSetInReview)
	require.NoError(t, err)
	require.Len(t, inReview, 1)
	assert.Equal(t, "full", inReview[0].Name)
	assert.Equal(t, 3, inReview[0].OpsCount)
}

func TestCountChangeSetsByStatus(t *testing.T) {
	store := newTestStore(t)
	ctx := t.Context()

	submitted := &ChangeSet{WorkspaceID: testWS, Name: "submitted", CreatedBy: "alice"}
	require.NoError(t, store.CreateChangeSet(ctx, submitted))
	require.NoError(t, store.AppendOp(ctx, &ChangeSetOp{
		WorkspaceID: testWS, ChangesetID: submitted.ID, Op: OpConceptDelete,
		Payload: mustJSON(t, ConceptDeletePayload{ConceptID: "c1"}), CreatedBy: "alice",
	}))
	require.NoError(t, store.SetChangeSetStatus(ctx, testWS, submitted.ID, ChangeSetInReview))
	require.NoError(t, store.CreateChangeSet(ctx,
		&ChangeSet{WorkspaceID: testWS, Name: "draft", CreatedBy: "alice"}))

	counts, err := store.CountChangeSetsByStatus(ctx, testWS)
	require.NoError(t, err)
	assert.Equal(t, 1, counts[ChangeSetDraft])
	assert.Equal(t, 1, counts[ChangeSetInReview])
	assert.Zero(t, counts[ChangeSetMerged])
}

// TestSetChangeSetImpactRoundtrips proves the summary survives the JSONB column
// and reaches every read of the change-set, and that storing it is not an edit.
func TestSetChangeSetImpactRoundtrips(t *testing.T) {
	store := newTestStore(t)
	ctx := t.Context()

	cs := &ChangeSet{WorkspaceID: testWS, Name: "impact", CreatedBy: "alice"}
	require.NoError(t, store.CreateChangeSet(ctx, cs))

	before, err := store.GetChangeSet(ctx, testWS, cs.ID)
	require.NoError(t, err)
	assert.Nil(t, before.Impact, "a change-set carries no summary until one is computed")

	summary := &ImpactSummary{
		TotalBlocks: 900, AffectedBlocks: 12, NewViolations: 7, Words: 340, Projects: 2,
		Partial: true, PartialReason: "time budget exhausted",
		ComputedAt: time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC),
	}
	require.NoError(t, store.SetChangeSetImpact(ctx, testWS, cs.ID, summary))

	got, err := store.GetChangeSet(ctx, testWS, cs.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Impact)
	assert.Equal(t, *summary, *got.Impact)
	assert.WithinDuration(t, before.UpdatedAt, got.UpdatedAt, 0,
		"a computed summary is not an edit and must not move the change-set up a recency list")

	// The list read carries it too, so a dashboard needs no second call.
	summaries, err := store.ListChangeSetSummaries(ctx, testWS, "")
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	require.NotNil(t, summaries[0].Impact)
	assert.Equal(t, 12, summaries[0].Impact.AffectedBlocks)

	require.NoError(t, store.SetChangeSetImpact(ctx, testWS, cs.ID, nil))
	cleared, err := store.GetChangeSet(ctx, testWS, cs.ID)
	require.NoError(t, err)
	assert.Nil(t, cleared.Impact)
}
