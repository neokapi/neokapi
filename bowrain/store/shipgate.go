package store

import (
	"context"
	"fmt"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/store/internal/storeutil"
)

// The ship gate's verdicts, counted in SQL.
//
// See core/store/shipgate.go for why they are stored at all. What lives here is
// the half that keeps a dashboard load's cost proportional to what changed: two
// grouped queries that never leave the database, and an upsert that records the
// pairs the caller had to compute.
//
// pgShipBasis is the token that decides whether a stored verdict still holds.
// It names the two things a verdict was computed from and neither the caller
// nor the store may forget: the block's source hash, and the exact revision of
// the target row. The timestamp is rendered as microseconds since the epoch
// rather than as text, so the token does not depend on the session's DateStyle
// or TimeZone — a basis that compared equal on one connection and not on
// another would silently recompute the whole corpus on every load.
const pgShipBasis = `(b.content_hash || '|' || ((extract(epoch from t.updated_at) * 1000000)::bigint)::text)`

// ShipGateRollup implements platstore.ShipVerdictStore. It answers two
// questions in two queries: what do the verdicts that still hold add up to, and
// which pairs no longer have one.
//
// Both are scoped to translatable blocks whose target is a locale the pass
// rates, which is exactly the set the in-process derivation visits — a variant
// carrying a tone or channel is neither counted nor reported, matching the
// dashboard, whose locale scopes are locale-only.
func (s *PostgresStore) ShipGateRollup(ctx context.Context, q platstore.ShipGateQuery) (platstore.ShipGateRollup, error) {
	out := platstore.ShipGateRollup{Scopes: map[string]map[string]platstore.ShipGateCounts{}}
	stream := storeutil.DefaultStream(q.Stream)
	if len(q.Locales) == 0 {
		return out, nil
	}

	scoredBlocks := make([]string, 0, len(q.Scores))
	scoredLocales := make([]string, 0, len(q.Scores))
	scoredBelow := make([]bool, 0, len(q.Scores))
	for _, sc := range q.Scores {
		scoredBlocks = append(scoredBlocks, sc.BlockID)
		scoredLocales = append(scoredLocales, sc.Locale)
		scoredBelow = append(scoredBelow, sc.BelowBar)
	}

	// The counting query. Everything it touches is a boolean or an id: no
	// source runs, no target payloads, nothing that grows with how much a block
	// says. The scored pairs arrive as three parallel arrays and are unnested
	// into a joinable relation, so the voice adjustment is grouped by the
	// database alongside the rest rather than folded in afterwards.
	const rollupSQL = `
		WITH scored AS (
			SELECT * FROM unnest($4::text[], $5::text[], $6::bool[]) AS s(block_id, locale, below_bar)
		), held AS (
			SELECT t.block_id, t.locale, v.fails, COALESCE(i.collection_id, '') AS collection_id
			FROM translations t
			JOIN blocks b ON b.project_id = t.project_id AND b.id = t.block_id
			JOIN ship_verdicts v ON v.project_id = t.project_id AND v.stream = t.stream
				AND v.block_id = t.block_id AND v.locale = t.locale
			LEFT JOIN items i ON i.project_id = b.project_id AND i.stream = t.stream AND i.name = b.item_name
			WHERE t.project_id = $1 AND t.stream = $2 AND b.translatable
				AND t.locale = ANY($3) AND v.gate = $7 AND v.basis = ` + pgShipBasis + `
		)
		SELECT h.collection_id, h.locale,
			count(*) FILTER (WHERE h.fails),
			count(*) FILTER (WHERE NOT h.fails),
			count(*) FILTER (WHERE s.block_id IS NOT NULL),
			count(*) FILTER (WHERE NOT h.fails AND s.below_bar)
		FROM held h
		LEFT JOIN scored s ON s.block_id = h.block_id AND s.locale = h.locale
		GROUP BY h.collection_id, h.locale`

	rows, err := s.db.QueryContext(ctx, rollupSQL,
		q.ProjectID, stream, q.Locales, scoredBlocks, scoredLocales, scoredBelow, q.Gate)
	if err != nil {
		return out, fmt.Errorf("roll up ship verdicts: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid, loc string
		var c platstore.ShipGateCounts
		if err := rows.Scan(&cid, &loc, &c.Failing, &c.Clean, &c.Scored, &c.CleanBelowBar); err != nil {
			return out, fmt.Errorf("scan ship verdict rollup: %w", err)
		}
		if out.Scopes[cid] == nil {
			out.Scopes[cid] = map[string]platstore.ShipGateCounts{}
		}
		out.Scopes[cid][loc] = c
	}
	if err := rows.Err(); err != nil {
		return out, err
	}

	// The pairs with no verdict that still holds. Ordered by block so the
	// caller's read of them groups a block's locales into one hydration.
	const staleSQL = `
		SELECT t.block_id, t.locale, ` + pgShipBasis + `
		FROM translations t
		JOIN blocks b ON b.project_id = t.project_id AND b.id = t.block_id
		LEFT JOIN ship_verdicts v ON v.project_id = t.project_id AND v.stream = t.stream
			AND v.block_id = t.block_id AND v.locale = t.locale AND v.gate = $4
		WHERE t.project_id = $1 AND t.stream = $2 AND b.translatable AND t.locale = ANY($3)
			AND (v.block_id IS NULL OR v.basis <> ` + pgShipBasis + `)
		ORDER BY t.block_id, t.locale`

	staleRows, err := s.db.QueryContext(ctx, staleSQL, q.ProjectID, stream, q.Locales, q.Gate)
	if err != nil {
		return out, fmt.Errorf("list stale ship verdicts: %w", err)
	}
	defer staleRows.Close()
	for staleRows.Next() {
		var st platstore.ShipGateStale
		if err := staleRows.Scan(&st.BlockID, &st.Locale, &st.Basis); err != nil {
			return out, fmt.Errorf("scan stale ship verdict: %w", err)
		}
		out.Stale = append(out.Stale, st)
	}
	return out, staleRows.Err()
}

// PutShipGateVerdicts implements platstore.ShipVerdictStore. A verdict is
// recorded only where the basis it was computed against is still the basis the
// project holds: the join re-reads the block and the target row and requires the
// same token the caller was handed. A pair whose content moved while the pass
// was computing is therefore not recorded at all, and the next load recomputes
// it — the one outcome that cannot leave a verdict claiming to have judged
// content it never saw.
func (s *PostgresStore) PutShipGateVerdicts(ctx context.Context, projectID, stream, gate string, verdicts []platstore.ShipGateVerdict) error {
	if len(verdicts) == 0 {
		return nil
	}
	stream = storeutil.DefaultStream(stream)

	blockIDs := make([]string, 0, len(verdicts))
	locales := make([]string, 0, len(verdicts))
	bases := make([]string, 0, len(verdicts))
	fails := make([]bool, 0, len(verdicts))
	for _, v := range verdicts {
		blockIDs = append(blockIDs, v.BlockID)
		locales = append(locales, v.Locale)
		bases = append(bases, v.Basis)
		fails = append(fails, v.Fails)
	}

	const upsertSQL = `
		INSERT INTO ship_verdicts (project_id, stream, block_id, locale, gate, basis, fails, updated_at)
		SELECT $1, $2, p.block_id, p.locale, $3, p.basis, p.fails, NOW()
		FROM unnest($4::text[], $5::text[], $6::text[], $7::bool[]) AS p(block_id, locale, basis, fails)
		JOIN blocks b ON b.project_id = $1 AND b.id = p.block_id
		JOIN translations t ON t.project_id = $1 AND t.stream = $2
			AND t.block_id = p.block_id AND t.locale = p.locale
		WHERE ` + pgShipBasis + ` = p.basis
		ON CONFLICT (project_id, stream, block_id, locale) DO UPDATE
		SET gate = EXCLUDED.gate, basis = EXCLUDED.basis,
			fails = EXCLUDED.fails, updated_at = EXCLUDED.updated_at`

	if _, err := s.db.ExecContext(ctx, upsertSQL,
		projectID, stream, gate, blockIDs, locales, bases, fails); err != nil {
		return fmt.Errorf("store ship verdicts: %w", err)
	}
	return nil
}
