package store

import (
	"context"
	"fmt"
	"time"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
)

// Channel-slug equivalence proposals — the workspace's record that two projects
// spell one channel differently.
//
// The table holds observations, not decisions about content. Nothing in
// resolution reads it: a project resolves its coordinates from its own recipe,
// offline, and it must resolve them the same way whether or not it has ever
// connected. What the workspace can see and a project cannot is the OTHER
// project, which is why the observation lives here and the resolution does not.

var _ platstore.ChannelAliasStore = (*PostgresStore)(nil)

// UpsertChannelAliasProposals implements platstore.ChannelAliasStore.
//
// A proposal a reviewer already judged keeps its status and its evidence: the
// same fragmentation is observed on every push, and re-observing it must not
// reopen a dismissal. Only the location of the latest sighting is refreshed.
func (s *PostgresStore) UpsertChannelAliasProposals(ctx context.Context, proposals []platstore.ChannelAliasProposal) (int, error) {
	if len(proposals) == 0 {
		return 0, nil
	}
	changed := 0
	for _, p := range proposals {
		if p.WorkspaceID == "" || p.ProposedChannel == "" || p.ExistingChannel == "" {
			continue
		}
		status := p.Status
		if status == "" {
			status = platstore.ChannelAliasProposed
		}
		tag, err := s.db.ExecContext(ctx,
			`INSERT INTO channel_alias_proposals
			   (workspace_id, profile, proposed_channel, existing_channel, evidence, project_id, collection, status)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			 ON CONFLICT (workspace_id, profile, proposed_channel, existing_channel) DO UPDATE SET
			   project_id = EXCLUDED.project_id,
			   collection = EXCLUDED.collection,
			   updated_at = NOW()`,
			p.WorkspaceID, p.Profile, p.ProposedChannel, p.ExistingChannel,
			p.Evidence, p.ProjectID, p.Collection, status)
		if err != nil {
			return changed, fmt.Errorf("upsert channel alias proposal: %w", err)
		}
		if n, aerr := tag.RowsAffected(); aerr == nil && n > 0 {
			changed++
		}
	}
	return changed, nil
}

// ListChannelAliasProposals implements platstore.ChannelAliasStore.
func (s *PostgresStore) ListChannelAliasProposals(ctx context.Context, workspaceID, status string) ([]platstore.ChannelAliasProposal, error) {
	query := `SELECT profile, proposed_channel, existing_channel, evidence, project_id, collection,
		         status, created_at, updated_at
		  FROM channel_alias_proposals WHERE workspace_id = $1`
	args := []any{workspaceID}
	if status != "" {
		query += ` AND status = $2`
		args = append(args, status)
	}
	query += ` ORDER BY updated_at DESC, proposed_channel, existing_channel`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list channel alias proposals: %w", err)
	}
	defer rows.Close()

	var out []platstore.ChannelAliasProposal
	for rows.Next() {
		p := platstore.ChannelAliasProposal{WorkspaceID: workspaceID}
		var created, updated time.Time
		if err := rows.Scan(&p.Profile, &p.ProposedChannel, &p.ExistingChannel, &p.Evidence,
			&p.ProjectID, &p.Collection, &p.Status, &created, &updated); err != nil {
			return nil, fmt.Errorf("scan channel alias proposal: %w", err)
		}
		p.CreatedAt = created.UTC().Format(time.RFC3339)
		p.UpdatedAt = updated.UTC().Format(time.RFC3339)
		out = append(out, p)
	}
	return out, rows.Err()
}
