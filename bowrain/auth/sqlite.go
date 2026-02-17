package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/gokapi/gokapi/bowrain/storage"
	"github.com/google/uuid"
)

var authMigrations = []storage.Migration{
	{
		Version:     1,
		Description: "create users table",
		SQL: `
			CREATE TABLE users (
				id         TEXT PRIMARY KEY,
				email      TEXT UNIQUE NOT NULL,
				name       TEXT NOT NULL,
				avatar_url TEXT NOT NULL DEFAULT '',
				created_at TEXT NOT NULL DEFAULT (datetime('now'))
			);
		`,
	},
	{
		Version:     2,
		Description: "create workspaces table",
		SQL: `
			CREATE TABLE workspaces (
				id          TEXT PRIMARY KEY,
				name        TEXT NOT NULL,
				slug        TEXT UNIQUE NOT NULL,
				description TEXT NOT NULL DEFAULT '',
				logo_url    TEXT NOT NULL DEFAULT '',
				created_at  TEXT NOT NULL DEFAULT (datetime('now')),
				updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
			);
		`,
	},
	{
		Version:     3,
		Description: "create workspace_members table",
		SQL: `
			CREATE TABLE workspace_members (
				workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
				user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				role         TEXT NOT NULL DEFAULT 'member',
				joined_at    TEXT NOT NULL DEFAULT (datetime('now')),
				PRIMARY KEY (workspace_id, user_id)
			);
		`,
	},
	{
		Version:     4,
		Description: "add workspace type column",
		SQL:         `ALTER TABLE workspaces ADD COLUMN type TEXT NOT NULL DEFAULT 'team';`,
	},
	{
		Version:     5,
		Description: "create unclaimed_projects table",
		SQL: `
			CREATE TABLE unclaimed_projects (
				project_id    TEXT PRIMARY KEY,
				claim_token   TEXT UNIQUE NOT NULL,
				name          TEXT NOT NULL,
				source_locale TEXT NOT NULL,
				target_locales TEXT NOT NULL,
				created_at    TEXT NOT NULL DEFAULT (datetime('now')),
				expires_at    TEXT NOT NULL
			);
			CREATE INDEX idx_unclaimed_expires ON unclaimed_projects(expires_at);
		`,
	},
	{
		Version:     6,
		Description: "create workspace_invites table",
		SQL: `
			CREATE TABLE workspace_invites (
				id           TEXT PRIMARY KEY,
				workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
				code         TEXT UNIQUE NOT NULL,
				email        TEXT,
				role         TEXT NOT NULL DEFAULT 'member',
				max_uses     INTEGER NOT NULL DEFAULT 1,
				use_count    INTEGER NOT NULL DEFAULT 0,
				created_by   TEXT NOT NULL REFERENCES users(id),
				expires_at   TEXT NOT NULL,
				created_at   TEXT NOT NULL DEFAULT (datetime('now'))
			);
		`,
	},
}

// SQLiteAuthStore implements AuthStore using SQLite.
type SQLiteAuthStore struct {
	db *storage.DB
}

// NewSQLiteAuthStore opens (or creates) a SQLite-backed AuthStore.
// If dbPath matches the content store path, the auth tables are
// created in the same database.
func NewSQLiteAuthStore(dbPath string) (*SQLiteAuthStore, error) {
	db, err := storage.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open auth database: %w", err)
	}
	if err := storage.Migrate(db, authMigrations); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate auth schema: %w", err)
	}
	return &SQLiteAuthStore{db: db}, nil
}

// NewSQLiteAuthStoreFromDB wraps an existing storage.DB for auth use.
// This is useful when sharing a single database file with the content store.
func NewSQLiteAuthStoreFromDB(db *storage.DB) (*SQLiteAuthStore, error) {
	if err := storage.Migrate(db, authMigrations); err != nil {
		return nil, fmt.Errorf("migrate auth schema: %w", err)
	}
	return &SQLiteAuthStore{db: db}, nil
}

func (s *SQLiteAuthStore) Close() error {
	return s.db.Close()
}

// ---------------------------------------------------------------------------
// Users
// ---------------------------------------------------------------------------

func (s *SQLiteAuthStore) CreateUser(ctx context.Context, u *User) error {
	if u.ID == "" {
		u.ID = uuid.NewString()
	}
	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO users (id, email, name, avatar_url, created_at) VALUES (?, ?, ?, ?, ?)`,
		u.ID, u.Email, u.Name, u.AvatarURL, u.CreatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("insert user: %w", err)
	}
	return nil
}

func (s *SQLiteAuthStore) GetUser(ctx context.Context, id string) (*User, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, email, name, avatar_url, created_at FROM users WHERE id = ?`, id)
	return scanUser(row)
}

func (s *SQLiteAuthStore) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, email, name, avatar_url, created_at FROM users WHERE email = ?`, email)
	return scanUser(row)
}

func (s *SQLiteAuthStore) UpdateUser(ctx context.Context, u *User) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE users SET email=?, name=?, avatar_url=? WHERE id=?`,
		u.Email, u.Name, u.AvatarURL, u.ID)
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("user %s not found", u.ID)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Workspaces
// ---------------------------------------------------------------------------

func (s *SQLiteAuthStore) CreateWorkspace(ctx context.Context, w *Workspace) error {
	if w.ID == "" {
		w.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	if w.CreatedAt.IsZero() {
		w.CreatedAt = now
	}
	w.UpdatedAt = now
	if w.Type == "" {
		w.Type = WorkspaceTypeTeam
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO workspaces (id, name, slug, description, logo_url, type, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		w.ID, w.Name, w.Slug, w.Description, w.LogoURL, string(w.Type),
		w.CreatedAt.Format(time.RFC3339), w.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("insert workspace: %w", err)
	}
	return nil
}

func (s *SQLiteAuthStore) GetWorkspace(ctx context.Context, id string) (*Workspace, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, slug, description, logo_url, type, created_at, updated_at
		 FROM workspaces WHERE id = ?`, id)
	return scanWorkspace(row)
}

func (s *SQLiteAuthStore) GetWorkspaceBySlug(ctx context.Context, slug string) (*Workspace, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, slug, description, logo_url, type, created_at, updated_at
		 FROM workspaces WHERE slug = ?`, slug)
	return scanWorkspace(row)
}

func (s *SQLiteAuthStore) ListWorkspaces(ctx context.Context, userID string) ([]*Workspace, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT w.id, w.name, w.slug, w.description, w.logo_url, w.type, w.created_at, w.updated_at
		 FROM workspaces w
		 JOIN workspace_members wm ON w.id = wm.workspace_id
		 WHERE wm.user_id = ?
		 ORDER BY w.name`, userID)
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	defer rows.Close()

	result := make([]*Workspace, 0)
	for rows.Next() {
		w, err := scanWorkspaceRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, w)
	}
	return result, rows.Err()
}

func (s *SQLiteAuthStore) UpdateWorkspace(ctx context.Context, w *Workspace) error {
	w.UpdatedAt = time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE workspaces SET name=?, slug=?, description=?, logo_url=?, updated_at=? WHERE id=?`,
		w.Name, w.Slug, w.Description, w.LogoURL,
		w.UpdatedAt.Format(time.RFC3339), w.ID)
	if err != nil {
		return fmt.Errorf("update workspace: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("workspace %s not found", w.ID)
	}
	return nil
}

func (s *SQLiteAuthStore) DeleteWorkspace(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM workspaces WHERE id=?`, id)
	if err != nil {
		return fmt.Errorf("delete workspace: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("workspace %s not found", id)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Membership
// ---------------------------------------------------------------------------

func (s *SQLiteAuthStore) AddMember(ctx context.Context, workspaceID, userID string, role Role) error {
	if !ValidRoles[role] {
		return fmt.Errorf("invalid role: %s", role)
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO workspace_members (workspace_id, user_id, role, joined_at) VALUES (?, ?, ?, ?)`,
		workspaceID, userID, string(role), time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("add member: %w", err)
	}
	return nil
}

func (s *SQLiteAuthStore) RemoveMember(ctx context.Context, workspaceID, userID string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM workspace_members WHERE workspace_id=? AND user_id=?`,
		workspaceID, userID)
	if err != nil {
		return fmt.Errorf("remove member: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("membership not found")
	}
	return nil
}

func (s *SQLiteAuthStore) UpdateRole(ctx context.Context, workspaceID, userID string, role Role) error {
	if !ValidRoles[role] {
		return fmt.Errorf("invalid role: %s", role)
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE workspace_members SET role=? WHERE workspace_id=? AND user_id=?`,
		string(role), workspaceID, userID)
	if err != nil {
		return fmt.Errorf("update role: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("membership not found")
	}
	return nil
}

func (s *SQLiteAuthStore) ListMembers(ctx context.Context, workspaceID string) ([]*Membership, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT workspace_id, user_id, role, joined_at
		 FROM workspace_members WHERE workspace_id = ?
		 ORDER BY joined_at`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	defer rows.Close()

	result := make([]*Membership, 0)
	for rows.Next() {
		m, err := scanMembershipRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

func (s *SQLiteAuthStore) GetMembership(ctx context.Context, workspaceID, userID string) (*Membership, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT workspace_id, user_id, role, joined_at
		 FROM workspace_members WHERE workspace_id = ? AND user_id = ?`,
		workspaceID, userID)
	return scanMembership(row)
}

// ---------------------------------------------------------------------------
// Scan helpers
// ---------------------------------------------------------------------------

type scanner interface {
	Scan(dest ...any) error
}

func scanUser(row scanner) (*User, error) {
	var u User
	var createdStr string
	err := row.Scan(&u.ID, &u.Email, &u.Name, &u.AvatarURL, &createdStr)
	if err != nil {
		return nil, fmt.Errorf("scan user: %w", err)
	}
	u.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	return &u, nil
}

func scanWorkspace(row scanner) (*Workspace, error) {
	var w Workspace
	var wsType, createdStr, updatedStr string
	err := row.Scan(&w.ID, &w.Name, &w.Slug, &w.Description, &w.LogoURL, &wsType, &createdStr, &updatedStr)
	if err != nil {
		return nil, fmt.Errorf("scan workspace: %w", err)
	}
	w.Type = WorkspaceType(wsType)
	w.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	w.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
	return &w, nil
}

func scanWorkspaceRow(rows scanner) (*Workspace, error) {
	return scanWorkspace(rows)
}

func scanMembership(row scanner) (*Membership, error) {
	var m Membership
	var role, joinedStr string
	err := row.Scan(&m.WorkspaceID, &m.UserID, &role, &joinedStr)
	if err != nil {
		return nil, fmt.Errorf("scan membership: %w", err)
	}
	m.Role = Role(role)
	m.JoinedAt, _ = time.Parse(time.RFC3339, joinedStr)
	return &m, nil
}

func scanMembershipRow(rows scanner) (*Membership, error) {
	return scanMembership(rows)
}

// ---------------------------------------------------------------------------
// Unclaimed Projects
// ---------------------------------------------------------------------------

func (s *SQLiteAuthStore) CreateUnclaimedProject(ctx context.Context, projectID, claimTokenHash, name, sourceLoc, targetLocs string, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO unclaimed_projects (project_id, claim_token, name, source_locale, target_locales, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		projectID, claimTokenHash, name, sourceLoc, targetLocs, expiresAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("insert unclaimed project: %w", err)
	}
	return nil
}

func (s *SQLiteAuthStore) GetUnclaimedByToken(ctx context.Context, claimTokenHash string) (*UnclaimedProject, error) {
	var p UnclaimedProject
	var createdStr, expiresStr string
	err := s.db.QueryRowContext(ctx,
		`SELECT project_id, claim_token, name, source_locale, target_locales, created_at, expires_at
		 FROM unclaimed_projects WHERE claim_token = ?`, claimTokenHash).
		Scan(&p.ProjectID, &p.ClaimToken, &p.Name, &p.SourceLocale, &p.TargetLocales, &createdStr, &expiresStr)
	if err != nil {
		return nil, fmt.Errorf("get unclaimed project: %w", err)
	}
	p.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	p.ExpiresAt, _ = time.Parse(time.RFC3339, expiresStr)
	return &p, nil
}

func (s *SQLiteAuthStore) DeleteUnclaimed(ctx context.Context, projectID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM unclaimed_projects WHERE project_id = ?`, projectID)
	if err != nil {
		return fmt.Errorf("delete unclaimed project: %w", err)
	}
	return nil
}

func (s *SQLiteAuthStore) PurgeExpiredUnclaimed(ctx context.Context) (int, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM unclaimed_projects WHERE expires_at < datetime('now')`)
	if err != nil {
		return 0, fmt.Errorf("purge expired unclaimed: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// ---------------------------------------------------------------------------
// Invitations
// ---------------------------------------------------------------------------

func (s *SQLiteAuthStore) CreateInvite(ctx context.Context, inv *Invite) error {
	if inv.ID == "" {
		inv.ID = uuid.NewString()
	}
	if inv.CreatedAt.IsZero() {
		inv.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO workspace_invites (id, workspace_id, code, email, role, max_uses, use_count, created_by, expires_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		inv.ID, inv.WorkspaceID, inv.Code, inv.Email, string(inv.Role),
		inv.MaxUses, inv.UseCount, inv.CreatedBy,
		inv.ExpiresAt.Format(time.RFC3339), inv.CreatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("insert invite: %w", err)
	}
	return nil
}

func (s *SQLiteAuthStore) GetInviteByCode(ctx context.Context, code string) (*Invite, error) {
	var inv Invite
	var role, expiresStr, createdStr string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, workspace_id, code, email, role, max_uses, use_count, created_by, expires_at, created_at
		 FROM workspace_invites WHERE code = ?`, code).
		Scan(&inv.ID, &inv.WorkspaceID, &inv.Code, &inv.Email, &role,
			&inv.MaxUses, &inv.UseCount, &inv.CreatedBy, &expiresStr, &createdStr)
	if err != nil {
		return nil, fmt.Errorf("get invite: %w", err)
	}
	inv.Role = Role(role)
	inv.ExpiresAt, _ = time.Parse(time.RFC3339, expiresStr)
	inv.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	return &inv, nil
}

func (s *SQLiteAuthStore) ListInvites(ctx context.Context, workspaceID string) ([]*Invite, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, workspace_id, code, email, role, max_uses, use_count, created_by, expires_at, created_at
		 FROM workspace_invites WHERE workspace_id = ?
		 ORDER BY created_at DESC`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list invites: %w", err)
	}
	defer rows.Close()

	result := make([]*Invite, 0)
	for rows.Next() {
		var inv Invite
		var role, expiresStr, createdStr string
		if err := rows.Scan(&inv.ID, &inv.WorkspaceID, &inv.Code, &inv.Email, &role,
			&inv.MaxUses, &inv.UseCount, &inv.CreatedBy, &expiresStr, &createdStr); err != nil {
			return nil, fmt.Errorf("scan invite: %w", err)
		}
		inv.Role = Role(role)
		inv.ExpiresAt, _ = time.Parse(time.RFC3339, expiresStr)
		inv.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		result = append(result, &inv)
	}
	return result, rows.Err()
}

func (s *SQLiteAuthStore) IncrementInviteUseCount(ctx context.Context, inviteID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE workspace_invites SET use_count = use_count + 1 WHERE id = ?`, inviteID)
	if err != nil {
		return fmt.Errorf("increment invite use count: %w", err)
	}
	return nil
}

func (s *SQLiteAuthStore) DeleteInvite(ctx context.Context, inviteID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM workspace_invites WHERE id = ?`, inviteID)
	if err != nil {
		return fmt.Errorf("delete invite: %w", err)
	}
	return nil
}
