package auth

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/core/id"
	platauth "github.com/neokapi/neokapi/platform/auth"
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
	{
		Version:     7,
		Description: "create refresh_tokens table",
		SQL: `
			CREATE TABLE refresh_tokens (
				id         TEXT PRIMARY KEY,
				user_id    TEXT NOT NULL,
				token_hash TEXT NOT NULL UNIQUE,
				expires_at TEXT NOT NULL,
				created_at TEXT NOT NULL,
				FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
			);
			CREATE INDEX idx_refresh_tokens_user ON refresh_tokens(user_id);
		`,
	},
	{
		Version:     8,
		Description: "add oidc_sub column to users",
		SQL: `
			ALTER TABLE users ADD COLUMN oidc_sub TEXT NOT NULL DEFAULT '';
			CREATE INDEX idx_users_oidc_sub ON users(oidc_sub);
		`,
	},
	{
		Version:     9,
		Description: "create api_tokens table",
		SQL: `
			CREATE TABLE api_tokens (
				id           TEXT PRIMARY KEY,
				user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
				name         TEXT NOT NULL,
				token_hash   TEXT UNIQUE NOT NULL,
				token_prefix TEXT NOT NULL,
				scopes       TEXT NOT NULL DEFAULT '["*"]',
				last_used_at TEXT,
				expires_at   TEXT,
				created_at   TEXT NOT NULL DEFAULT (datetime('now'))
			);
			CREATE INDEX idx_api_tokens_workspace ON api_tokens(workspace_id);
			CREATE INDEX idx_api_tokens_user ON api_tokens(user_id);
		`,
	},
	{
		Version:     10,
		Description: "add languages to workspaces and rename locale columns in unclaimed_projects",
		SQL: `
			ALTER TABLE workspaces ADD COLUMN languages TEXT NOT NULL DEFAULT '[]';
			ALTER TABLE unclaimed_projects RENAME COLUMN source_locale TO default_source_language;
			ALTER TABLE unclaimed_projects RENAME COLUMN target_locales TO target_languages;
		`,
	},
	{
		Version:     11,
		Description: "add plan and stripe_customer_id to workspaces",
		SQL: `
			ALTER TABLE workspaces ADD COLUMN plan TEXT NOT NULL DEFAULT 'free';
			ALTER TABLE workspaces ADD COLUMN stripe_customer_id TEXT;
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

func (s *SQLiteAuthStore) CreateUser(ctx context.Context, u *platauth.User) error {
	if u.ID == "" {
		u.ID = id.New()
	}
	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO users (id, email, name, avatar_url, oidc_sub, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		u.ID, u.Email, u.Name, u.AvatarURL, u.OIDCSub, u.CreatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("insert user: %w", err)
	}
	return nil
}

func (s *SQLiteAuthStore) GetUser(ctx context.Context, id string) (*platauth.User, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, email, name, avatar_url, oidc_sub, created_at FROM users WHERE id = ?`, id)
	return scanUser(row)
}

func (s *SQLiteAuthStore) GetUserByEmail(ctx context.Context, email string) (*platauth.User, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, email, name, avatar_url, oidc_sub, created_at FROM users WHERE email = ?`, email)
	return scanUser(row)
}

func (s *SQLiteAuthStore) GetUserByOIDCSub(ctx context.Context, sub string) (*platauth.User, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, email, name, avatar_url, oidc_sub, created_at FROM users WHERE oidc_sub = ?`, sub)
	return scanUser(row)
}

func (s *SQLiteAuthStore) UpdateUser(ctx context.Context, u *platauth.User) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE users SET email=?, name=?, avatar_url=?, oidc_sub=? WHERE id=?`,
		u.Email, u.Name, u.AvatarURL, u.OIDCSub, u.ID)
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

func (s *SQLiteAuthStore) CreateWorkspace(ctx context.Context, w *platauth.Workspace) error {
	if w.ID == "" {
		w.ID = id.New()
	}
	now := time.Now().UTC()
	if w.CreatedAt.IsZero() {
		w.CreatedAt = now
	}
	w.UpdatedAt = now
	if w.Type == "" {
		w.Type = platauth.WorkspaceTypeTeam
	}

	if w.Plan == "" {
		w.Plan = "free"
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO workspaces (id, name, slug, description, logo_url, type, plan, stripe_customer_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		w.ID, w.Name, w.Slug, w.Description, w.LogoURL, string(w.Type),
		w.Plan, w.StripeCustomerID,
		w.CreatedAt.Format(time.RFC3339), w.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("insert workspace: %w", err)
	}
	return nil
}

func (s *SQLiteAuthStore) GetWorkspace(ctx context.Context, id string) (*platauth.Workspace, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, slug, description, logo_url, type, plan, stripe_customer_id, created_at, updated_at
		 FROM workspaces WHERE id = ?`, id)
	return scanWorkspace(row)
}

func (s *SQLiteAuthStore) GetWorkspaceBySlug(ctx context.Context, slug string) (*platauth.Workspace, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, slug, description, logo_url, type, plan, stripe_customer_id, created_at, updated_at
		 FROM workspaces WHERE slug = ?`, slug)
	return scanWorkspace(row)
}

func (s *SQLiteAuthStore) ListWorkspaces(ctx context.Context, userID string) ([]*platauth.Workspace, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT w.id, w.name, w.slug, w.description, w.logo_url, w.type, w.plan, w.stripe_customer_id, w.created_at, w.updated_at, wm.role
		 FROM workspaces w
		 JOIN workspace_members wm ON w.id = wm.workspace_id
		 WHERE wm.user_id = ?
		 ORDER BY w.name`, userID)
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	defer rows.Close()

	result := make([]*platauth.Workspace, 0)
	for rows.Next() {
		w, err := scanWorkspaceWithRole(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, w)
	}
	return result, rows.Err()
}

func (s *SQLiteAuthStore) UpdateWorkspace(ctx context.Context, w *platauth.Workspace) error {
	w.UpdatedAt = time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE workspaces SET name=?, slug=?, description=?, logo_url=?, plan=?, stripe_customer_id=?, updated_at=? WHERE id=?`,
		w.Name, w.Slug, w.Description, w.LogoURL, w.Plan, w.StripeCustomerID,
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

func (s *SQLiteAuthStore) AddMember(ctx context.Context, workspaceID, userID string, role platauth.Role) error {
	if !platauth.ValidRoles[role] {
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

func (s *SQLiteAuthStore) UpdateRole(ctx context.Context, workspaceID, userID string, role platauth.Role) error {
	if !platauth.ValidRoles[role] {
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

func (s *SQLiteAuthStore) ListMembers(ctx context.Context, workspaceID string) ([]*platauth.Membership, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT workspace_id, user_id, role, joined_at
		 FROM workspace_members WHERE workspace_id = ?
		 ORDER BY joined_at`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	defer rows.Close()

	result := make([]*platauth.Membership, 0)
	for rows.Next() {
		m, err := scanMembershipRow(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

func (s *SQLiteAuthStore) GetMembership(ctx context.Context, workspaceID, userID string) (*platauth.Membership, error) {
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

func scanUser(row scanner) (*platauth.User, error) {
	var u platauth.User
	var createdStr string
	err := row.Scan(&u.ID, &u.Email, &u.Name, &u.AvatarURL, &u.OIDCSub, &createdStr)
	if err != nil {
		return nil, fmt.Errorf("scan user: %w", err)
	}
	u.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	return &u, nil
}

func scanWorkspace(row scanner) (*platauth.Workspace, error) {
	var w platauth.Workspace
	var wsType, createdStr, updatedStr string
	var stripeCustomerID *string
	err := row.Scan(&w.ID, &w.Name, &w.Slug, &w.Description, &w.LogoURL, &wsType, &w.Plan, &stripeCustomerID, &createdStr, &updatedStr)
	if err != nil {
		return nil, fmt.Errorf("scan workspace: %w", err)
	}
	w.Type = platauth.WorkspaceType(wsType)
	w.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	w.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
	if stripeCustomerID != nil {
		w.StripeCustomerID = *stripeCustomerID
	}
	return &w, nil
}

// scanWorkspaceWithRole scans workspace columns plus wm.role.
func scanWorkspaceWithRole(row scanner) (*platauth.Workspace, error) {
	var w platauth.Workspace
	var wsType, createdStr, updatedStr, role string
	var stripeCustomerID *string
	err := row.Scan(&w.ID, &w.Name, &w.Slug, &w.Description, &w.LogoURL, &wsType, &w.Plan, &stripeCustomerID, &createdStr, &updatedStr, &role)
	if err != nil {
		return nil, fmt.Errorf("scan workspace with role: %w", err)
	}
	w.Type = platauth.WorkspaceType(wsType)
	w.Role = platauth.Role(role)
	w.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	w.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
	if stripeCustomerID != nil {
		w.StripeCustomerID = *stripeCustomerID
	}
	return &w, nil
}

func scanMembership(row scanner) (*platauth.Membership, error) {
	var m platauth.Membership
	var role, joinedStr string
	err := row.Scan(&m.WorkspaceID, &m.UserID, &role, &joinedStr)
	if err != nil {
		return nil, fmt.Errorf("scan membership: %w", err)
	}
	m.Role = platauth.Role(role)
	m.JoinedAt, _ = time.Parse(time.RFC3339, joinedStr)
	return &m, nil
}

func scanMembershipRow(rows scanner) (*platauth.Membership, error) {
	return scanMembership(rows)
}

// ---------------------------------------------------------------------------
// Unclaimed Projects
// ---------------------------------------------------------------------------

func (s *SQLiteAuthStore) CreateUnclaimedProject(ctx context.Context, projectID, claimTokenHash, name, sourceLoc, targetLocs string, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO unclaimed_projects (project_id, claim_token, name, default_source_language, target_languages, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		projectID, claimTokenHash, name, sourceLoc, targetLocs, expiresAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("insert unclaimed project: %w", err)
	}
	return nil
}

func (s *SQLiteAuthStore) GetUnclaimedByToken(ctx context.Context, claimTokenHash string) (*platauth.UnclaimedProject, error) {
	var p platauth.UnclaimedProject
	var createdStr, expiresStr string
	err := s.db.QueryRowContext(ctx,
		`SELECT project_id, claim_token, name, default_source_language, target_languages, created_at, expires_at
		 FROM unclaimed_projects WHERE claim_token = ?`, claimTokenHash).
		Scan(&p.ProjectID, &p.ClaimToken, &p.Name, &p.DefaultSourceLanguage, &p.TargetLanguages, &createdStr, &expiresStr)
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

func (s *SQLiteAuthStore) CreateInvite(ctx context.Context, inv *platauth.Invite) error {
	if inv.ID == "" {
		inv.ID = id.New()
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

func (s *SQLiteAuthStore) GetInviteByCode(ctx context.Context, code string) (*platauth.Invite, error) {
	var inv platauth.Invite
	var role, expiresStr, createdStr string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, workspace_id, code, email, role, max_uses, use_count, created_by, expires_at, created_at
		 FROM workspace_invites WHERE code = ?`, code).
		Scan(&inv.ID, &inv.WorkspaceID, &inv.Code, &inv.Email, &role,
			&inv.MaxUses, &inv.UseCount, &inv.CreatedBy, &expiresStr, &createdStr)
	if err != nil {
		return nil, fmt.Errorf("get invite: %w", err)
	}
	inv.Role = platauth.Role(role)
	inv.ExpiresAt, _ = time.Parse(time.RFC3339, expiresStr)
	inv.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	return &inv, nil
}

func (s *SQLiteAuthStore) ListInvites(ctx context.Context, workspaceID string) ([]*platauth.Invite, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, workspace_id, code, email, role, max_uses, use_count, created_by, expires_at, created_at
		 FROM workspace_invites WHERE workspace_id = ?
		 ORDER BY created_at DESC`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list invites: %w", err)
	}
	defer rows.Close()

	result := make([]*platauth.Invite, 0)
	for rows.Next() {
		var inv platauth.Invite
		var role, expiresStr, createdStr string
		if err := rows.Scan(&inv.ID, &inv.WorkspaceID, &inv.Code, &inv.Email, &role,
			&inv.MaxUses, &inv.UseCount, &inv.CreatedBy, &expiresStr, &createdStr); err != nil {
			return nil, fmt.Errorf("scan invite: %w", err)
		}
		inv.Role = platauth.Role(role)
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

// ---------------------------------------------------------------------------
// Refresh Tokens
// ---------------------------------------------------------------------------

func (s *SQLiteAuthStore) StoreRefreshToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) (string, error) {
	id := id.New()
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, created_at) VALUES (?, ?, ?, ?, ?)`,
		id, userID, tokenHash, expiresAt.Format(time.RFC3339), now.Format(time.RFC3339))
	if err != nil {
		return "", fmt.Errorf("insert refresh token: %w", err)
	}
	return id, nil
}

func (s *SQLiteAuthStore) ValidateRefreshTokenByHash(ctx context.Context, tokenHash string) (string, error) {
	var id, userID, expiresStr string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, expires_at FROM refresh_tokens WHERE token_hash = ?`, tokenHash).
		Scan(&id, &userID, &expiresStr)
	if err != nil {
		return "", fmt.Errorf("refresh token not found: %w", err)
	}

	expiresAt, _ := time.Parse(time.RFC3339, expiresStr)
	if time.Now().After(expiresAt) {
		// Expired — delete and reject.
		if _, err := s.db.ExecContext(ctx, `DELETE FROM refresh_tokens WHERE id = ?`, id); err != nil {
			log.Printf("WARNING: failed to delete expired refresh token %s: %v", id, err)
		}
		return "", fmt.Errorf("refresh token expired")
	}

	// Single-use: delete after successful validation (token rotation).
	if _, err := s.db.ExecContext(ctx, `DELETE FROM refresh_tokens WHERE id = ?`, id); err != nil {
		log.Printf("WARNING: failed to delete consumed refresh token %s: %v", id, err)
	}
	return userID, nil
}

func (s *SQLiteAuthStore) RevokeRefreshToken(ctx context.Context, tokenID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM refresh_tokens WHERE id = ?`, tokenID)
	if err != nil {
		return fmt.Errorf("revoke refresh token: %w", err)
	}
	return nil
}

func (s *SQLiteAuthStore) RevokeUserRefreshTokens(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM refresh_tokens WHERE user_id = ?`, userID)
	if err != nil {
		return fmt.Errorf("revoke user refresh tokens: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// API Tokens
// ---------------------------------------------------------------------------

func (s *SQLiteAuthStore) CreateAPIToken(ctx context.Context, token *platauth.APIToken, tokenHash string) error {
	if token.ID == "" {
		token.ID = id.New()
	}
	if token.CreatedAt.IsZero() {
		token.CreatedAt = time.Now().UTC()
	}
	if token.Scopes == "" {
		token.Scopes = `["*"]`
	}

	var expiresStr *string
	if token.ExpiresAt != nil {
		s := token.ExpiresAt.Format(time.RFC3339)
		expiresStr = &s
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO api_tokens (id, user_id, workspace_id, name, token_hash, token_prefix, scopes, expires_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		token.ID, token.UserID, token.WorkspaceID, token.Name, tokenHash,
		token.TokenPrefix, token.Scopes, expiresStr, token.CreatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("insert api token: %w", err)
	}
	return nil
}

func (s *SQLiteAuthStore) GetAPITokenByHash(ctx context.Context, tokenHash string) (*platauth.APIToken, error) {
	var tok platauth.APIToken
	var lastUsedStr, expiresStr, createdStr *string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, workspace_id, name, token_prefix, scopes, last_used_at, expires_at, created_at
		 FROM api_tokens WHERE token_hash = ?`, tokenHash).
		Scan(&tok.ID, &tok.UserID, &tok.WorkspaceID, &tok.Name, &tok.TokenPrefix,
			&tok.Scopes, &lastUsedStr, &expiresStr, &createdStr)
	if err != nil {
		return nil, fmt.Errorf("get api token: %w", err)
	}
	if lastUsedStr != nil {
		t, _ := time.Parse(time.RFC3339, *lastUsedStr)
		tok.LastUsedAt = &t
	}
	if expiresStr != nil {
		t, _ := time.Parse(time.RFC3339, *expiresStr)
		tok.ExpiresAt = &t
	}
	if createdStr != nil {
		tok.CreatedAt, _ = time.Parse(time.RFC3339, *createdStr)
	}
	return &tok, nil
}

func (s *SQLiteAuthStore) ListAPITokens(ctx context.Context, workspaceID string) ([]*platauth.APIToken, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, workspace_id, name, token_prefix, scopes, last_used_at, expires_at, created_at
		 FROM api_tokens WHERE workspace_id = ?
		 ORDER BY created_at DESC`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list api tokens: %w", err)
	}
	defer rows.Close()

	result := make([]*platauth.APIToken, 0)
	for rows.Next() {
		var tok platauth.APIToken
		var lastUsedStr, expiresStr, createdStr *string
		if err := rows.Scan(&tok.ID, &tok.UserID, &tok.WorkspaceID, &tok.Name, &tok.TokenPrefix,
			&tok.Scopes, &lastUsedStr, &expiresStr, &createdStr); err != nil {
			return nil, fmt.Errorf("scan api token: %w", err)
		}
		if lastUsedStr != nil {
			t, _ := time.Parse(time.RFC3339, *lastUsedStr)
			tok.LastUsedAt = &t
		}
		if expiresStr != nil {
			t, _ := time.Parse(time.RFC3339, *expiresStr)
			tok.ExpiresAt = &t
		}
		if createdStr != nil {
			tok.CreatedAt, _ = time.Parse(time.RFC3339, *createdStr)
		}
		result = append(result, &tok)
	}
	return result, rows.Err()
}

func (s *SQLiteAuthStore) DeleteAPIToken(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM api_tokens WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete api token: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("api token %s not found", id)
	}
	return nil
}

func (s *SQLiteAuthStore) UpdateAPITokenLastUsed(ctx context.Context, id string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx,
		`UPDATE api_tokens SET last_used_at = ? WHERE id = ?`, now, id)
	if err != nil {
		return fmt.Errorf("update api token last used: %w", err)
	}
	return nil
}
