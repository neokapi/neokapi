package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// A pushed decision a released client wrote, with the agent host folded into
// its note, is stored as the note the person typed and the host as its own
// column. A client that pulled it and echoes it without the host leaves the
// host in place, and a row a released client wrote before the column existed
// is left as it is: the split record is the same record.
func TestUnitDecisions_TheAgentHostIsStoredApartFromTheNote(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{blockWithTarget("greeting", "Hello", "Hei", model.TargetStatusTranslated)}))

	legacy := venue.UnitDecision{
		ItemName: "en.json", Unit: "greeting", Variant: "nb",
		Status:      string(model.TargetStatusDraft),
		Revision:    nbTextRevision("Hei"),
		Basis:       sourceRevision("Hello"),
		ReviewState: venue.ReviewStateRejected,
		DecidedBy:   "reviewer@example.com",
		DecidedAt:   "2026-11-23T10:00:00Z",
		Note:        "Too formal. (recorded as a person in a claude-code shell)",
		Updated:     "2026-11-23T10:00:00Z",
	}
	changed, err := s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{legacy})
	require.NoError(t, err)
	assert.Equal(t, 1, changed)
	got, err := s.GetUnitDecision(ctx, p.ID, "main", "en.json", "greeting", "nb")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "Too formal.", got.Note, "the note is what the person typed")
	assert.Equal(t, "claude-code", got.AgentHost)

	echo := *got
	echo.AgentHost = ""
	changed, err = s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{echo})
	require.NoError(t, err)
	assert.Zero(t, changed, "an echo without the host is the same decision")
	got, err = s.GetUnitDecision(ctx, p.ID, "main", "en.json", "greeting", "nb")
	require.NoError(t, err)
	assert.Equal(t, "claude-code", got.AgentHost, "and keeps the host")

	changed, err = s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{legacy})
	require.NoError(t, err)
	assert.Zero(t, changed, "the released client's form is the same record")

	// A row a released client wrote before the column existed.
	_, err = s.SQLDB().ExecContext(ctx, `UPDATE unit_decisions SET note=$1, agent_host='' WHERE project_id=$2`, legacy.Note, p.ID)
	require.NoError(t, err)
	changed, err = s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{venue.SplitNote(legacy)})
	require.NoError(t, err)
	assert.Zero(t, changed, "the split record is the record the store holds")
	got, err = s.GetUnitDecision(ctx, p.ID, "main", "en.json", "greeting", "nb")
	require.NoError(t, err)
	assert.Equal(t, legacy.Note, got.Note, "a stored row is not rewritten")
	said := venue.SplitNote(*got)
	assert.Equal(t, "Too formal.", said.Note, "and is read as the note alone")
	assert.Equal(t, "claude-code", said.AgentHost)
}
