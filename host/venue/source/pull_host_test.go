package source

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/venue"
	bowrainconn "github.com/neokapi/neokapi/core/venue/connector"
	"github.com/neokapi/neokapi/host"
)

// The agent host a person recorded a decision from travels beside the note,
// through the pull into the checkout's record and back out through the push,
// under the identity the venue gives the decision. A venue older than the
// field sends the host inside the note; the pull reads it out, and the push
// sends the note alone with the host beside it, under the same identity.
func TestPull_ADecisionKeepsItsAgentHostApartFromTheNote(t *testing.T) {
	for name, served := range map[string]venue.UnitDecision{
		"held apart": {Note: "Too formal.", AgentHost: "claude-code"},
		"folded in":  {Note: "Too formal. (recorded as a person in a claude-code shell)"},
	} {
		t.Run(name, func(t *testing.T) {
			const projectID = "proj-host"
			served.ItemName = "locales/en.json"
			served.Unit = "greeting"
			served.Variant = "fr"
			served.Status = string(model.TargetStatusDraft)
			served.ReviewState = venue.ReviewStateRejected
			served.DecidedBy = "reviewer@example.test"
			served.DecidedAt = "2026-11-23T08:00:00Z"
			served.Updated = "2026-11-23T08:00:00Z"
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
			var got state.Decision
			for _, u := range all {
				if u.Unit == "greeting" {
					got = u.Decision
				}
			}
			assert.Equal(t, "Too formal.", got.Note, "the record holds the note the person typed")
			assert.Equal(t, "claude-code", got.AgentHost)

			pushed, err := conn.projectDecisions(context.Background())
			require.NoError(t, err)
			require.Len(t, pushed, 1)
			assert.Equal(t, "Too formal.", pushed[0].Note)
			assert.Equal(t, "claude-code", pushed[0].AgentHost)
			assert.Equal(t, venue.DecisionIdentity(served), venue.DecisionIdentity(pushed[0]))
			require.NoError(t, conn.Close())
		})
	}
}
