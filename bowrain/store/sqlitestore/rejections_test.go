package sqlitestore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// The SQLite twin of the PostgreSQL TestRejectionTallies: each rejected draft
// counts once however often it is restated, a repeated draft counts, a changed
// source starts again, and one project's count never shows in another.
func TestRejectionTallies_SQLite(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)
	other := createTestProject(t, s)
	for _, proj := range []string{p.ID, other.ID} {
		b := blockWithText("greeting", "Hello")
		b.SetTargetText("nb", "Hei")
		require.NoError(t, s.StoreBlocksForItem(ctx, proj, "main", "en.json", []*model.Block{b}))
	}
	reject := func(project, target, source, note, updated string) {
		t.Helper()
		_, err := s.UpsertUnitDecisions(ctx, project, "main", []venue.UnitDecision{{
			ItemName: "en.json", Unit: "greeting", Variant: "nb",
			ReviewState: venue.ReviewStateRejected, Reasons: []string{"tone"}, Note: note,
			Revision: nbTextRevision(target), Basis: sourceRevision(source), Updated: updated,
		}})
		require.NoError(t, err)
	}
	tallies := func(project string) []platstore.RejectionTally {
		t.Helper()
		got, err := s.ListRejectionTallies(ctx, project, "main")
		require.NoError(t, err)
		return got
	}
	want := func(basis string, count int) []platstore.RejectionTally {
		return []platstore.RejectionTally{{ItemName: "en.json", Unit: "greeting", Variant: "nb", Basis: sourceRevision(basis), Count: count}}
	}

	reject(p.ID, "Hei", "Hello", "Too formal.", "2026-08-04T10:00:00Z")
	assert.Equal(t, want("Hello", 1), tallies(p.ID))
	reject(p.ID, "Hei", "Hello", "Too formal, use tu.", "2026-08-04T10:01:00Z")
	assert.Equal(t, want("Hello", 1), tallies(p.ID), "the same rejected draft restated is one rejection")

	require.NoError(t, s.RecordRepeatedDrafts(ctx, p.ID, "main", []platstore.DraftBasis{
		{ItemName: "en.json", Unit: "greeting", Variant: "nb", Basis: sourceRevision("Hello")},
		{ItemName: "en.json", Unit: "absent", Variant: "nb", Basis: sourceRevision("Hello")},
	}))
	assert.Equal(t, want("Hello", 2), tallies(p.ID), "a repeated draft counts, and a unit with no row gets none")
	assert.Equal(t, 1, rejectedOwed(t, s, p.ID), "below the stop the rejected unit owes a draft")

	reject(p.ID, "Hallo", "Hello", "Still too formal.", "2026-08-04T10:02:00Z")
	assert.Equal(t, want("Hello", 3), tallies(p.ID))
	assert.Zero(t, rejectedOwed(t, s, p.ID), "at the stop the unit is a person's to write")
	assert.Empty(t, tallies(other.ID), "another project holding the same item and unit counts nothing")

	reject(p.ID, "Hei der", "Hello there", "Wrong meaning.", "2026-08-04T10:03:00Z")
	assert.Equal(t, want("Hello there", 1), tallies(p.ID), "a rejection against another source starts again")
}

// The SQLite twin of TestRejectionTallies_ARejectionNamingNoSourceCountsAgainstTheBlocks.
func TestRejectionTallies_SQLiteARejectionNamingNoSourceCountsAgainstTheBlocks(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)
	b := blockWithText("greeting", "Hello")
	b.SetTargetText("nb", "Hei")
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{b}))
	for i, target := range []string{"Hei", "Hallo"} {
		_, err := s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{{
			ItemName: "en.json", Unit: "greeting", Variant: "nb",
			ReviewState: venue.ReviewStateRejected, Note: "No.",
			Revision: nbTextRevision(target), Updated: "2026-08-04T10:0" + string(rune('0'+i)) + ":00Z",
		}})
		require.NoError(t, err)
	}
	got, err := s.ListRejectionTallies(ctx, p.ID, "main")
	require.NoError(t, err)
	assert.Equal(t, []platstore.RejectionTally{{ItemName: "en.json", Unit: "greeting", Variant: "nb", Basis: sourceRevision("Hello"), Count: 2}}, got)
}

// rejectedOwed is the stream's RejectedOwed, summed over its scopes.
func rejectedOwed(t *testing.T, s platstore.DecisionStore, projectID string) int {
	t.Helper()
	tallies, err := s.TallyDecisionBasis(t.Context(), projectID, "main")
	require.NoError(t, err)
	n := 0
	for _, tl := range tallies {
		n += tl.RejectedOwed
	}
	return n
}
