package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
)

// SlugReservationWindow is the default grace period during which a renamed
// workspace's old slug stays reserved (cannot be claimed by another workspace).
const SlugReservationWindow = 30 * 24 * time.Hour

// ReserveSlug records that `slug` was previously held by `workspaceID` and
// must not be reused until `until`. If a reservation already exists for the
// slug, it is overwritten (most recent rename wins).
func (s *PostgresAuthStore) ReserveSlug(ctx context.Context, workspaceID, slug string, until time.Time) error {
	if slug == "" {
		return errors.New("slug is required")
	}
	if workspaceID == "" {
		return errors.New("workspace ID is required")
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO workspace_slug_reservations (slug, workspace_id, reserved_until)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (slug) DO UPDATE
		   SET workspace_id = EXCLUDED.workspace_id,
		       reserved_until = EXCLUDED.reserved_until`,
		slug, workspaceID, until)
	if err != nil {
		return fmt.Errorf("reserve slug: %w", err)
	}
	return nil
}

// IsSlugReserved reports whether the given slug is currently reserved (i.e.,
// has an active, non-expired reservation). Expired reservations are ignored
// and treated as not-reserved; PurgeExpiredSlugReservations removes them.
func (s *PostgresAuthStore) IsSlugReserved(ctx context.Context, slug string) (string, time.Time, bool, error) {
	var workspaceID string
	var until time.Time
	err := s.db.QueryRowContext(ctx,
		`SELECT workspace_id, reserved_until FROM workspace_slug_reservations
		 WHERE slug = $1 AND reserved_until > NOW()`, slug).Scan(&workspaceID, &until)
	if errors.Is(err, sql.ErrNoRows) {
		return "", time.Time{}, false, nil
	}
	if err != nil {
		return "", time.Time{}, false, fmt.Errorf("check slug reservation: %w", err)
	}
	return workspaceID, until, true, nil
}

// PurgeExpiredSlugReservations removes reservations whose grace period has ended.
func (s *PostgresAuthStore) PurgeExpiredSlugReservations(ctx context.Context) (int, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM workspace_slug_reservations WHERE reserved_until <= NOW()`)
	if err != nil {
		return 0, fmt.Errorf("purge slug reservations: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// ListSlugReservations returns all currently active reservations, most
// recently created first. Expired reservations are skipped.
func (s *PostgresAuthStore) ListSlugReservations(ctx context.Context) ([]*platauth.SlugReservation, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT slug, workspace_id, reserved_until, created_at
		 FROM workspace_slug_reservations
		 WHERE reserved_until > NOW()
		 ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list slug reservations: %w", err)
	}
	defer rows.Close()
	out := make([]*platauth.SlugReservation, 0)
	for rows.Next() {
		var r platauth.SlugReservation
		if err := rows.Scan(&r.Slug, &r.WorkspaceID, &r.ReservedUntil, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan slug reservation: %w", err)
		}
		out = append(out, &r)
	}
	return out, rows.Err()
}

// ListTakenSlugs returns the subset of `candidates` that are currently
// unavailable — either an active workspace slug or a live rename
// reservation. The result map is keyed by slug; a missing key means the
// candidate is free.
//
// One round trip is used regardless of how many candidates are passed; the
// query plan hits the unique indexes on `workspaces.slug` and
// `workspace_slug_reservations.slug` so it scales linearly with the input
// list, not with the table sizes.
func (s *PostgresAuthStore) ListTakenSlugs(ctx context.Context, candidates []string) (map[string]bool, error) {
	if len(candidates) == 0 {
		return map[string]bool{}, nil
	}
	args := make([]any, len(candidates))
	phs := make([]string, len(candidates))
	for i, c := range candidates {
		args[i] = c
		phs[i] = fmt.Sprintf("$%d", i+1)
	}
	list := strings.Join(phs, ",")
	q := fmt.Sprintf(`
		SELECT slug FROM workspaces WHERE slug IN (%s)
		UNION
		SELECT slug FROM workspace_slug_reservations
			WHERE reserved_until > NOW() AND slug IN (%s)`,
		list, list)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list taken slugs: %w", err)
	}
	defer rows.Close()
	taken := make(map[string]bool, len(candidates))
	for rows.Next() {
		var slug string
		if err := rows.Scan(&slug); err != nil {
			return nil, fmt.Errorf("scan taken slug: %w", err)
		}
		taken[slug] = true
	}
	return taken, rows.Err()
}

// ReleaseSlugReservation removes a single reservation, freeing the slug for
// immediate reuse. Used by ctrl admins to override the grace period.
func (s *PostgresAuthStore) ReleaseSlugReservation(ctx context.Context, slug string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM workspace_slug_reservations WHERE slug = $1`, slug)
	if err != nil {
		return fmt.Errorf("release slug reservation: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("no reservation for slug %q", slug)
	}
	return nil
}
