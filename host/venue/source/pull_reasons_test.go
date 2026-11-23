package source

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
	bowrainconn "github.com/neokapi/neokapi/core/venue/connector"
	"github.com/neokapi/neokapi/host"
)

// A pulled rejection keeps its reasons in the checkout's record, and the
// push conversion sends them back as the venue sent them, under the identity
// the venue gives the decision.
func TestPull_ARejectionKeepsItsReasons(t *testing.T) {
	const projectID = "proj-reasons"
	served := venue.UnitDecision{
		ItemName:    "locales/en.json",
		Unit:        "greeting",
		Variant:     "fr",
		Status:      string(model.TargetStatusDraft),
		ReviewState: venue.ReviewStateRejected,
		DecidedBy:   "reviewer@example.test",
		DecidedAt:   "2026-11-23T08:00:00Z",
		Note:        "Too formal.",
		Reasons:     []string{"meaning", "tone"},
		Updated:     "2026-11-23T08:00:00Z",
	}
	srv := newDecisionLedgerServer(t, projectID, []venue.UnitDecision{served})

	a := &host.App{}
	defer a.Shutdown()
	proj := decisionsProject(t, srv.Server, projectID)
	conn := decisionsConnector(t, a, proj, srv.Server, projectID)

	res, err := conn.Pull(context.Background(), bowrainconn.PullOptions{})
	require.NoError(t, err)
	require.Equal(t, 1, res.DecisionsStaged)

	st, err := a.OpenProjectState(t.Context(), proj.Root)
	require.NoError(t, err)
	all, err := st.All(t.Context())
	require.NoError(t, err)
	var reasons []string
	for _, u := range all {
		if u.Unit == "greeting" {
			reasons = u.Decision.Reasons
		}
	}
	assert.Equal(t, []string{"meaning", "tone"}, reasons, "the record holds the reasons")

	pushed, err := conn.projectDecisions(context.Background())
	require.NoError(t, err)
	require.Len(t, pushed, 1)
	assert.Equal(t, served.Reasons, pushed[0].Reasons)
	assert.Equal(t, served.Note, pushed[0].Note)
	assert.Equal(t, venue.DecisionIdentity(served), venue.DecisionIdentity(pushed[0]))
	require.NoError(t, conn.Close())
}
