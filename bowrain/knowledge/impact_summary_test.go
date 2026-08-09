package knowledge

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSummarizeKeepsTotalsAndDropsTheBreakdown(t *testing.T) {
	at := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	impact := ChangeSetImpact{
		TotalBlocks: 900, AffectedBlocks: 12, NewViolations: 7, Resolved: 2, Words: 340,
		Projects: []ProjectImpact{{ProjectID: "p1"}, {ProjectID: "p2"}},
		Samples:  []BlockSample{{BlockID: "b1"}},
	}

	got := impact.Summarize(at)

	assert.Equal(t, ImpactSummary{
		TotalBlocks: 900, AffectedBlocks: 12, NewViolations: 7, Resolved: 2, Words: 340,
		Projects: 2, ComputedAt: at,
	}, got)
}

// TestSummarizeKeepsPartial: a walk that ran out of budget is worth storing,
// but only if the floor still reads as a floor.
func TestSummarizeKeepsPartial(t *testing.T) {
	got := ChangeSetImpact{
		AffectedBlocks: 4, Partial: true, PartialReason: "time budget exhausted",
	}.Summarize(time.Now())

	assert.True(t, got.Partial)
	assert.Equal(t, "time budget exhausted", got.PartialReason)
}

func TestReportMarksItselfStored(t *testing.T) {
	at := time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC)
	summary := ImpactSummary{AffectedBlocks: 12, Words: 340, Projects: 2, ComputedAt: at}

	got := summary.Report()

	assert.True(t, got.Stored)
	require.NotNil(t, got.ComputedAt)
	assert.Equal(t, at, *got.ComputedAt)
	assert.Equal(t, 12, got.AffectedBlocks)
	assert.Empty(t, got.Projects, "the summary never carried the breakdown")
	assert.Empty(t, got.Samples)
}

// TestImpactSummaryRoundtripsThroughJSONB exercises the exact path the store
// uses to persist the summary in the JSONB column.
func TestImpactSummaryRoundtripsThroughJSONB(t *testing.T) {
	in := ImpactSummary{
		TotalBlocks: 900, AffectedBlocks: 12, NewViolations: 7, Resolved: 2,
		Words: 340, Projects: 2, Partial: true, PartialReason: "time budget exhausted",
		ComputedAt: time.Date(2026, 8, 9, 10, 0, 0, 0, time.UTC),
	}

	raw, err := json.Marshal(in)
	require.NoError(t, err)
	var out ImpactSummary
	require.NoError(t, json.Unmarshal(raw, &out))
	assert.Equal(t, in, out)
}

func TestStoreSummariesCarryOpCounts(t *testing.T) {
	s := newMemStore()
	ctx := t.Context()

	empty := &ChangeSet{WorkspaceID: "ws", ID: "cs-empty", Name: "empty"}
	require.NoError(t, s.CreateChangeSet(ctx, empty))
	full := &ChangeSet{WorkspaceID: "ws", ID: "cs-full", Name: "full"}
	require.NoError(t, s.CreateChangeSet(ctx, full))
	for range 3 {
		require.NoError(t, s.AppendOp(ctx, &ChangeSetOp{
			WorkspaceID: "ws", ChangesetID: full.ID, Op: OpConceptCreate, Payload: json.RawMessage(`null`),
		}))
	}

	got, err := s.ListChangeSetSummaries(ctx, "ws", "")
	require.NoError(t, err)
	counts := map[string]int{}
	for _, cs := range got {
		counts[cs.ID] = cs.OpsCount
	}
	assert.Equal(t, map[string]int{"cs-empty": 0, "cs-full": 3}, counts)
}

func TestCountChangeSetsByStatusBuckets(t *testing.T) {
	s := newMemStore()
	ctx := t.Context()
	for _, id := range []string{"a", "b"} {
		require.NoError(t, s.CreateChangeSet(ctx, &ChangeSet{WorkspaceID: "ws", ID: id}))
	}
	require.NoError(t, s.CreateChangeSet(ctx, &ChangeSet{WorkspaceID: "other", ID: "c"}))
	require.NoError(t, s.AppendOp(ctx, &ChangeSetOp{
		WorkspaceID: "ws", ChangesetID: "a", Op: OpConceptCreate, Payload: json.RawMessage(`null`),
	}))
	require.NoError(t, s.SetChangeSetStatus(ctx, "ws", "a", ChangeSetInReview))

	got, err := s.CountChangeSetsByStatus(ctx, "ws")
	require.NoError(t, err)
	assert.Equal(t, map[ChangeSetStatus]int{ChangeSetDraft: 1, ChangeSetInReview: 1}, got,
		"another workspace's change-sets are not this workspace's")
}
