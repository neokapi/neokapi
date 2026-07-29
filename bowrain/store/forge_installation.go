package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrForgeInstallationNotFound is returned when no installation row matches —
// the id is unknown, or it is known but owned by another workspace. Handlers map
// it to HTTP 404: an installation belonging to someone else must be
// indistinguishable from one that does not exist, so no tenant learns another
// tenant's installation ids.
var ErrForgeInstallationNotFound = errors.New("forge installation not found")

// ErrForgeInstallationClaimed is returned when a workspace tries to claim an
// installation another workspace already owns. First claim wins; a second
// claimant is refused rather than silently rebinding the installation.
var ErrForgeInstallationClaimed = errors.New("forge installation is already claimed by another workspace")

// ForgeInstallation records which workspace a GitHub App installation belongs
// to. One registered app serves every workspace on a shared instance, and the
// installation id in a request URL is a bare integer that proves nothing, so
// this row is what makes "may this workspace act on installation N?" answerable
// at all.
//
// WorkspaceID is empty while the installation is UNCLAIMED — recorded (the
// app-level webhook saw GitHub create it) but not yet attributed to a
// workspace. An unclaimed installation is reachable from no workspace; only a
// claim through the signed setup state attributes it.
type ForgeInstallation struct {
	InstallationID int64
	WorkspaceID    string
	// Account is the GitHub account (organization or user) the app is installed
	// on, carried for display and operator diagnosis. It is never used to decide
	// access — WorkspaceID is.
	Account   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Claimed reports whether the installation has been attributed to a workspace.
func (i ForgeInstallation) Claimed() bool { return i.WorkspaceID != "" }

// ForgeInstallationStore persists GitHub App installation ownership. Concrete
// *sql.DB store (mirrors ConnectorConfigStore/SourceProposalStore): the table
// exists on both SQLite and Postgres and the timestamps are TEXT RFC3339, so one
// store scans identically on either backend.
type ForgeInstallationStore struct {
	db *sql.DB
}

// NewForgeInstallationStore wraps a *sql.DB whose schema already carries the
// forge_installations table (created by the store migrations).
func NewForgeInstallationStore(db *sql.DB) *ForgeInstallationStore {
	return &ForgeInstallationStore{db: db}
}

const forgeInstallationSelect = `SELECT installation_id, workspace_id, account, created_at, updated_at
	FROM forge_installations`

// Record notes that an installation exists, without touching ownership. It is
// the webhook's write: the 'installation' and 'installation_repositories'
// deliveries are authentic (GitHub signs them with the app's webhook secret)
// but name no workspace, so they can only ever record — a recorded installation
// stays unclaimed until the signed setup state attributes it. Re-recording an
// already-claimed installation refreshes its account and leaves the owner
// alone, so a later 'installation_repositories' delivery can never unbind one.
func (s *ForgeInstallationStore) Record(ctx context.Context, installationID int64, account string) error {
	if installationID == 0 {
		return errors.New("forge installation: installation id required")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	// Explicit UPDATE-then-INSERT keeps the SQL dialect-neutral (no ON CONFLICT
	// differences between PostgreSQL and SQLite), as ConnectorConfigStore does.
	res, err := s.db.ExecContext(ctx,
		`UPDATE forge_installations SET account=$1, updated_at=$2 WHERE installation_id=$3`,
		account, now, installationID)
	if err != nil {
		return fmt.Errorf("record forge installation: %w", err)
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return nil
	}
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO forge_installations (installation_id, workspace_id, account, created_at, updated_at)
		 VALUES ($1, '', $2, $3, $3)`,
		installationID, account, now); err != nil {
		return fmt.Errorf("insert forge installation: %w", err)
	}
	return nil
}

// Claim attributes an installation to a workspace, creating the row when the
// webhook has not arrived yet — the redirect and the delivery race, and either
// may win. It writes ownership only; Account stays the webhook's to maintain.
//
// First claim wins. Re-claiming from the owning workspace is idempotent — a
// customer who re-runs the install flow lands back on their own installation —
// while a claim from any other workspace returns ErrForgeInstallationClaimed
// and changes nothing.
func (s *ForgeInstallationStore) Claim(ctx context.Context, installationID int64, workspaceID string) (ForgeInstallation, error) {
	if installationID == 0 {
		return ForgeInstallation{}, errors.New("forge installation: installation id required")
	}
	if workspaceID == "" {
		return ForgeInstallation{}, errors.New("forge installation: workspace_id required")
	}
	now := time.Now().UTC().Format(time.RFC3339)

	// Claim only an unclaimed row (or re-confirm this workspace's own). The
	// workspace_id predicate makes the write itself the arbiter, so two
	// concurrent claims can never both succeed.
	res, err := s.db.ExecContext(ctx,
		`UPDATE forge_installations SET workspace_id=$1, updated_at=$2
		 WHERE installation_id=$3 AND (workspace_id='' OR workspace_id=$1)`,
		workspaceID, now, installationID)
	if err != nil {
		return ForgeInstallation{}, fmt.Errorf("claim forge installation: %w", err)
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return s.Get(ctx, installationID)
	}

	// Nothing updated: either the row does not exist yet (the webhook has not
	// landed) or it belongs to another workspace. Distinguish by reading it.
	if _, gErr := s.Get(ctx, installationID); gErr == nil {
		return ForgeInstallation{}, ErrForgeInstallationClaimed
	} else if !errors.Is(gErr, ErrForgeInstallationNotFound) {
		return ForgeInstallation{}, gErr
	}

	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO forge_installations (installation_id, workspace_id, account, created_at, updated_at)
		 VALUES ($1, $2, '', $3, $3)`,
		installationID, workspaceID, now); err != nil {
		// A webhook that landed between the read and the INSERT loses the race
		// on the primary key; retry the claim against the row it created, which
		// is unclaimed by construction.
		res, retryErr := s.db.ExecContext(ctx,
			`UPDATE forge_installations SET workspace_id=$1, updated_at=$2
			 WHERE installation_id=$3 AND workspace_id=''`,
			workspaceID, now, installationID)
		if retryErr != nil {
			return ForgeInstallation{}, fmt.Errorf("insert forge installation: %w", err)
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ForgeInstallation{}, ErrForgeInstallationClaimed
		}
	}
	return s.Get(ctx, installationID)
}

// Get returns one installation row by id, claimed or not. Returns
// ErrForgeInstallationNotFound when the installation is unknown.
func (s *ForgeInstallationStore) Get(ctx context.Context, installationID int64) (ForgeInstallation, error) {
	row := s.db.QueryRowContext(ctx, forgeInstallationSelect+` WHERE installation_id=$1`, installationID)
	return scanForgeInstallation(row)
}

// OwnedBy reports whether the installation is claimed by the given workspace.
// It is the tenancy gate the GitHub App setup handlers call before minting an
// installation token: an unknown, unclaimed, or differently-owned installation
// all answer false, and the caller fails closed the same way for each.
func (s *ForgeInstallationStore) OwnedBy(ctx context.Context, installationID int64, workspaceID string) (bool, error) {
	if workspaceID == "" {
		return false, nil
	}
	inst, err := s.Get(ctx, installationID)
	if errors.Is(err, ErrForgeInstallationNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return inst.WorkspaceID == workspaceID, nil
}

// List returns the installations a workspace owns, oldest first. Unclaimed
// installations belong to no workspace and are never listed.
func (s *ForgeInstallationStore) List(ctx context.Context, workspaceID string) ([]ForgeInstallation, error) {
	if workspaceID == "" {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx,
		forgeInstallationSelect+` WHERE workspace_id=$1 ORDER BY created_at, installation_id`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list forge installations: %w", err)
	}
	defer rows.Close()
	out := make([]ForgeInstallation, 0)
	for rows.Next() {
		inst, err := scanForgeInstallation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, inst)
	}
	return out, rows.Err()
}

// Forget removes an installation. It is what the 'installation' deleted
// delivery calls: an uninstall on GitHub revokes the binding immediately rather
// than leaving a workspace holding a claim on an installation it no longer has,
// and a later re-install starts unclaimed. A missing row is not an error — the
// desired end state is already true.
func (s *ForgeInstallationStore) Forget(ctx context.Context, installationID int64) error {
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM forge_installations WHERE installation_id=$1`, installationID); err != nil {
		return fmt.Errorf("forget forge installation: %w", err)
	}
	return nil
}

// scanForgeInstallation reads one row into a ForgeInstallation.
func scanForgeInstallation(sc scanner) (ForgeInstallation, error) {
	var inst ForgeInstallation
	var createdStr, updatedStr string
	err := sc.Scan(&inst.InstallationID, &inst.WorkspaceID, &inst.Account, &createdStr, &updatedStr)
	if errors.Is(err, sql.ErrNoRows) {
		return ForgeInstallation{}, ErrForgeInstallationNotFound
	}
	if err != nil {
		return ForgeInstallation{}, fmt.Errorf("scan forge installation: %w", err)
	}
	inst.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	inst.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
	return inst, nil
}
