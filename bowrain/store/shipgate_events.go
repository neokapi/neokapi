package store

import (
	"context"
	"fmt"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/store/internal/storeutil"
)

// OpenShipGateFailure implements platstore.ShipGateFailureStore. The upsert
// changes an open failure only when its not-checked flag differs, and RETURNING
// yields a row only for an insert or that change. A second derivation recording
// the same failure at the same moment waits on the first one's row and gets no
// row back.
func (s *PostgresStore) OpenShipGateFailure(ctx context.Context, f platstore.ShipGateFailure) (bool, error) {
	const openSQL = `
		INSERT INTO ship_gate_failures (project_id, stream, locale, gate, not_checked, actual, required, opened_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
		ON CONFLICT (project_id, stream, locale, gate) DO UPDATE
		SET not_checked = EXCLUDED.not_checked, actual = EXCLUDED.actual,
			required = EXCLUDED.required, opened_at = EXCLUDED.opened_at
		WHERE ship_gate_failures.not_checked IS DISTINCT FROM EXCLUDED.not_checked
		RETURNING gate`

	rows, err := s.db.QueryContext(ctx, openSQL,
		f.ProjectID, storeutil.DefaultStream(f.Stream), f.Locale, f.Gate, f.NotChecked, f.Actual, f.Required)
	if err != nil {
		return false, fmt.Errorf("record ship gate failure: %w", err)
	}
	defer rows.Close()
	changed := rows.Next()
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("record ship gate failure: %w", err)
	}
	return changed, nil
}

// CloseShipGateFailure implements platstore.ShipGateFailureStore.
func (s *PostgresStore) CloseShipGateFailure(ctx context.Context, projectID, stream, locale, gate string) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM ship_gate_failures WHERE project_id = $1 AND stream = $2 AND locale = $3 AND gate = $4`,
		projectID, storeutil.DefaultStream(stream), locale, gate)
	if err != nil {
		return false, fmt.Errorf("clear ship gate failure: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("clear ship gate failure: %w", err)
	}
	return n > 0, nil
}

var _ platstore.ShipGateFailureStore = (*PostgresStore)(nil)
