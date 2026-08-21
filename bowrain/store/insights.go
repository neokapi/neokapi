package store

import (
	"context"
	"fmt"

	"github.com/neokapi/neokapi/bowrain/core/insights"
	"github.com/neokapi/neokapi/bowrain/store/internal/storeutil"
	coreprofile "github.com/neokapi/neokapi/core/profile"
)

// ContextFunnelCounts gathers the four stages of the context funnel for one
// project (see bowrain/core/insights).
//
// The queries are kept here, thin, and the arithmetic lives in core/insights so
// it can be tested without a database — the interesting logic is what the
// numbers MEAN, not how they are fetched.
//
// A note on "governed": profile binding is a property on the ladder
// (workspace → project → stream → collection), not a column on a block. This
// counts a block as governed when its project or its stream binds a profile,
// which is the resolution that exists today. Once coordinates are declared on
// collections, this query grows a per-collection arm and the number gets more
// honest — it can only under-report now, never over-report, because a
// collection binding without a project binding is not yet expressible.
func (s *PostgresStore) ContextFunnelCounts(ctx context.Context, projectID, stream string) (insights.Counts, error) {
	var c insights.Counts
	stream = storeutil.DefaultStream(stream)

	// Indexed: every translatable block. Non-translatable blocks are skeleton
	// and structure; counting them would inflate the denominator with content
	// no profile could ever govern.
	if err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM blocks WHERE project_id = $1 AND stream = $2 AND translatable = TRUE`,
		projectID, stream).Scan(&c.Indexed); err != nil {
		return c, fmt.Errorf("insights: count indexed blocks: %w", err)
	}

	// Governed: all of them, if the project (or its stream) binds a profile;
	// none otherwise. Coarse by construction — see the note above.
	var bound bool
	if err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(properties::jsonb ->> $2, '') <> ''
		   FROM projects WHERE id = $1`,
		projectID, coreprofile.PropertyProfileID).Scan(&bound); err != nil {
		return c, fmt.Errorf("insights: read project profile binding: %w", err)
	}
	if bound {
		c.Governed = c.Indexed
	}

	// Checked: distinct blocks with at least one recorded score. DISTINCT
	// matters — a block scored in six locales is one checked block, not six.
	if err := s.db.QueryRowContext(ctx,
		`SELECT count(DISTINCT block_id) FROM brand_voice_scores
		  WHERE project_id = $1 AND stream = $2`,
		projectID, stream).Scan(&c.Checked); err != nil {
		return c, fmt.Errorf("insights: count checked blocks: %w", err)
	}

	// Produced under context: targets whose provenance carries a profile.
	// This is the stamp added in neokapi#1514 — the only evidence that the
	// guidance actually reached the producer, rather than merely existing.
	if err := s.db.QueryRowContext(ctx,
		`SELECT
		     count(*) FILTER (WHERE COALESCE(target_json -> 'origin' ->> 'profile', '') <> ''),
		     count(*)
		   FROM translations WHERE project_id = $1 AND stream = $2`,
		projectID, stream).Scan(&c.ProducedUnderContext, &c.TargetsTotal); err != nil {
		return c, fmt.Errorf("insights: count produced-under-context targets: %w", err)
	}

	return c, nil
}

// ContextFunnel computes the funnel for one project.
func (s *PostgresStore) ContextFunnel(ctx context.Context, projectID, stream string) (insights.Funnel, error) {
	counts, err := s.ContextFunnelCounts(ctx, projectID, stream)
	if err != nil {
		return insights.Funnel{}, err
	}
	return insights.Compute(counts), nil
}
