package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	coresync "github.com/neokapi/neokapi/bowrain/core/sync"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
)

func newAliasStore(t *testing.T) *PostgresStore {
	t.Helper()
	s, err := NewPostgresStoreFromDB(pgtest.NewTestDB(t))
	require.NoError(t, err)
	return s
}

func TestChannelAliasProposalsRoundTrip(t *testing.T) {
	s := newAliasStore(t)
	ctx := t.Context()

	n, err := s.UpsertChannelAliasProposals(ctx, []platstore.ChannelAliasProposal{
		{
			WorkspaceID: "ws1", Profile: "acme",
			ProposedChannel: "website", ExistingChannel: "web",
			Evidence: coresync.EvidencePrefix, ProjectID: "p1", Collection: "site",
		},
		{
			WorkspaceID: "ws1", Profile: "acme",
			ProposedChannel: "marketing-site", ExistingChannel: "marketingsite",
			Evidence: coresync.EvidenceSameNormalized, ProjectID: "p1", Collection: "marketing",
		},
		// Another workspace's proposal is not this one's.
		{
			WorkspaceID: "ws2", Profile: "acme",
			ProposedChannel: "website", ExistingChannel: "web",
			Evidence: coresync.EvidencePrefix, ProjectID: "p9",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 3, n)

	got, err := s.ListChannelAliasProposals(ctx, "ws1", "")
	require.NoError(t, err)
	require.Len(t, got, 2)
	for _, p := range got {
		assert.Equal(t, "ws1", p.WorkspaceID)
		assert.Equal(t, platstore.ChannelAliasProposed, p.Status)
		assert.NotEmpty(t, p.CreatedAt)
	}
}

// The same fragmentation is observed on every push, so re-observing it must not
// reopen a judgement someone already made.
func TestChannelAliasProposalKeepsItsJudgement(t *testing.T) {
	s := newAliasStore(t)
	ctx := t.Context()
	proposal := platstore.ChannelAliasProposal{
		WorkspaceID: "ws1", Profile: "acme",
		ProposedChannel: "website", ExistingChannel: "web",
		Evidence: coresync.EvidencePrefix, ProjectID: "p1", Collection: "site",
	}
	_, err := s.UpsertChannelAliasProposals(ctx, []platstore.ChannelAliasProposal{proposal})
	require.NoError(t, err)

	dismissed := proposal
	dismissed.Status = platstore.ChannelAliasDismissed
	_, err = s.UpsertChannelAliasProposals(ctx, []platstore.ChannelAliasProposal{dismissed})
	require.NoError(t, err)
	// The upsert never carries a status onto an existing row, so the dismissal
	// has to be written the way a reviewer writes it.
	_, err = s.db.ExecContext(ctx,
		`UPDATE channel_alias_proposals SET status=$1 WHERE workspace_id=$2 AND proposed_channel=$3`,
		platstore.ChannelAliasDismissed, "ws1", "website")
	require.NoError(t, err)

	// Re-observing the same fragmentation refreshes where it was seen, not the
	// judgement.
	reseen := proposal
	reseen.ProjectID = "p2"
	reseen.Collection = "marketing"
	_, err = s.UpsertChannelAliasProposals(ctx, []platstore.ChannelAliasProposal{reseen})
	require.NoError(t, err)

	got, err := s.ListChannelAliasProposals(ctx, "ws1", "")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, platstore.ChannelAliasDismissed, got[0].Status)
	assert.Equal(t, "p2", got[0].ProjectID)

	open, err := s.ListChannelAliasProposals(ctx, "ws1", platstore.ChannelAliasProposed)
	require.NoError(t, err)
	assert.Empty(t, open)
}
