package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/gokapi/gokapi/bowrain/storage"
	platauth "github.com/gokapi/gokapi/platform/auth"
	"github.com/google/uuid"
)

// PostgresAuthStore implements AuthStore using PostgreSQL.
type PostgresAuthStore struct {
	db *storage.PgDB
}

// NewPostgresAuthStore opens a PostgreSQL-backed AuthStore.
func NewPostgresAuthStore(connStr string) (*PostgresAuthStore, error) {
	db, err := storage.OpenPostgres(connStr)
	if err != nil {
		return nil, fmt.Errorf("open auth database: %w", err)
	}
	if err := storage.MigratePostgresNS(db, "auth_schema_migrations", authMigrationsPg); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate auth schema: %w", err)
	}
	return &PostgresAuthStore{db: db}, nil
}

// NewPostgresAuthStoreFromDB wraps an existing PgDB for auth use.
func NewPostgresAuthStoreFromDB(db *storage.PgDB) (*PostgresAuthStore, error) {
	if err := storage.MigratePostgresNS(db, "auth_schema_migrations", authMigrationsPg); err != nil {
		return nil, fmt.Errorf("migrate auth schema: %w", err)
	}
	return &PostgresAuthStore{db: db}, nil
}

func (s *PostgresAuthStore) Close() error {
	return s.db.Close()
}

// ---------------------------------------------------------------------------
// Users
// ---------------------------------------------------------------------------

func (s *PostgresAuthStore) CreateUser(ctx context.Context, u *platauth.User) error {
	if u.ID == "" {
		u.ID = uuid.NewString()
	}
	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO users (id, email, name, avatar_url, oidc_sub, created_at) VALUES ($1, $2, $3, $4, $5, $6)`,
		u.ID, u.Email, u.Name, u.AvatarURL, u.OIDCSub, u.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert user: %w", err)
	}
	return nil
}

func (s *PostgresAuthStore) GetUser(ctx context.Context, id string) (*platauth.User, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, email, name, avatar_url, oidc_sub, created_at FROM users WHERE id = $1`, id)
	return scanUserPg(row)
}

func (s *PostgresAuthStore) GetUserByEmail(ctx context.Context, email string) (*platauth.User, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, email, name, avatar_url, oidc_sub, created_at FROM users WHERE email = $1`, email)
	return scanUserPg(row)
}

func (s *PostgresAuthStore) GetUserByOIDCSub(ctx context.Context, sub string) (*platauth.User, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, email, name, avatar_url, oidc_sub, created_at FROM users WHERE oidc_sub = $1`, sub)
	return scanUserPg(row)
}

func (s *PostgresAuthStore) UpdateUser(ctx context.Context, u *platauth.User) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE users SET email=$1, name=$2, avatar_url=$3, oidc_sub=$4 WHERE id=$5`,
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

func (s *PostgresAuthStore) CreateWorkspace(ctx context.Context, w *platauth.Workspace) error {
	if w.ID == "" {
		w.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	if w.CreatedAt.IsZero() {
		w.CreatedAt = now
	}
	w.UpdatedAt = now
	if w.Type == "" {
		w.Type = platauth.WorkspaceTypeTeam
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO workspaces (id, name, slug, description, logo_url, type, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		w.ID, w.Name, w.Slug, w.Description, w.LogoURL, string(w.Type),
		w.CreatedAt, w.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert workspace: %w", err)
	}
	return nil
}

func (s *PostgresAuthStore) GetWorkspace(ctx context.Context, id string) (*platauth.Workspace, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, slug, description, logo_url, type, created_at, updated_at
		 FROM workspaces WHERE id = $1`, id)
	return scanWorkspacePg(row)
}

func (s *PostgresAuthStore) GetWorkspaceBySlug(ctx context.Context, slug string) (*platauth.Workspace, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, slug, description, logo_url, type, created_at, updated_at
		 FROM workspaces WHERE slug = $1`, slug)
	return scanWorkspacePg(row)
}

func (s *PostgresAuthStore) ListWorkspaces(ctx context.Context, userID string) ([]*platauth.Workspace, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT w.id, w.name, w.slug, w.description, w.logo_url, w.type, w.created_at, w.updated_at, wm.role
		 FROM workspaces w
		 JOIN workspace_members wm ON w.id = wm.workspace_id
		 WHERE wm.user_id = $1
		 ORDER BY w.name`, userID)
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	defer rows.Close()

	result := make([]*platauth.Workspace, 0)
	for rows.Next() {
		w, err := scanWorkspaceWithRolePg(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, w)
	}
	return result, rows.Err()
}

func (s *PostgresAuthStore) UpdateWorkspace(ctx context.Context, w *platauth.Workspace) error {
	w.UpdatedAt = time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE workspaces SET name=$1, slug=$2, description=$3, logo_url=$4, updated_at=$5 WHERE id=$6`,
		w.Name, w.Slug, w.Description, w.LogoURL, w.UpdatedAt, w.ID)
	if err != nil {
		return fmt.Errorf("update workspace: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("workspace %s not found", w.ID)
	}
	return nil
}

func (s *PostgresAuthStore) DeleteWorkspace(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM workspaces WHERE id=$1`, id)
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

func (s *PostgresAuthStore) AddMember(ctx context.Context, workspaceID, userID string, role platauth.Role) error {
	if !platauth.ValidRoles[role] {
		return fmt.Errorf("invalid role: %s", role)
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO workspace_members (workspace_id, user_id, role, joined_at) VALUES ($1, $2, $3, $4)`,
		workspaceID, userID, string(role), time.Now().UTC())
	if err != nil {
		return fmt.Errorf("add member: %w", err)
	}
	return nil
}

func (s *PostgresAuthStore) RemoveMember(ctx context.Context, workspaceID, userID string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM workspace_members WHERE workspace_id=$1 AND user_id=$2`,
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

func (s *PostgresAuthStore) UpdateRole(ctx context.Context, workspaceID, userID string, role platauth.Role) error {
	if !platauth.ValidRoles[role] {
		return fmt.Errorf("invalid role: %s", role)
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE workspace_members SET role=$1 WHERE workspace_id=$2 AND user_id=$3`,
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

func (s *PostgresAuthStore) ListMembers(ctx context.Context, workspaceID string) ([]*platauth.Membership, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT workspace_id, user_id, role, joined_at
		 FROM workspace_members WHERE workspace_id = $1
		 ORDER BY joined_at`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list members: %w", err)
	}
	defer rows.Close()

	result := make([]*platauth.Membership, 0)
	for rows.Next() {
		m, err := scanMembershipPg(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, m)
	}
	return result, rows.Err()
}

func (s *PostgresAuthStore) GetMembership(ctx context.Context, workspaceID, userID string) (*platauth.Membership, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT workspace_id, user_id, role, joined_at
		 FROM workspace_members WHERE workspace_id = $1 AND user_id = $2`,
		workspaceID, userID)
	return scanMembershipPg(row)
}

// ---------------------------------------------------------------------------
// Unclaimed Projects
// ---------------------------------------------------------------------------

func (s *PostgresAuthStore) CreateUnclaimedProject(ctx context.Context, projectID, claimTokenHash, name, sourceLoc, targetLocs string, expiresAt time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO unclaimed_projects (project_id, claim_token, name, source_locale, target_locales, expires_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		projectID, claimTokenHash, name, sourceLoc, targetLocs, expiresAt)
	if err != nil {
		return fmt.Errorf("insert unclaimed project: %w", err)
	}
	return nil
}

func (s *PostgresAuthStore) GetUnclaimedByToken(ctx context.Context, claimTokenHash string) (*platauth.UnclaimedProject, error) {
	var p platauth.UnclaimedProject
	err := s.db.QueryRowContext(ctx,
		`SELECT project_id, claim_token, name, source_locale, target_locales, created_at, expires_at
		 FROM unclaimed_projects WHERE claim_token = $1`, claimTokenHash).
		Scan(&p.ProjectID, &p.ClaimToken, &p.Name, &p.SourceLocale, &p.TargetLocales, &p.CreatedAt, &p.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("get unclaimed project: %w", err)
	}
	return &p, nil
}

func (s *PostgresAuthStore) DeleteUnclaimed(ctx context.Context, projectID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM unclaimed_projects WHERE project_id = $1`, projectID)
	if err != nil {
		return fmt.Errorf("delete unclaimed project: %w", err)
	}
	return nil
}

func (s *PostgresAuthStore) PurgeExpiredUnclaimed(ctx context.Context) (int, error) {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM unclaimed_projects WHERE expires_at < NOW()`)
	if err != nil {
		return 0, fmt.Errorf("purge expired unclaimed: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// ---------------------------------------------------------------------------
// Invitations
// ---------------------------------------------------------------------------

func (s *PostgresAuthStore) CreateInvite(ctx context.Context, inv *platauth.Invite) error {
	if inv.ID == "" {
		inv.ID = uuid.NewString()
	}
	if inv.CreatedAt.IsZero() {
		inv.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO workspace_invites (id, workspace_id, code, email, role, max_uses, use_count, created_by, expires_at, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		inv.ID, inv.WorkspaceID, inv.Code, inv.Email, string(inv.Role),
		inv.MaxUses, inv.UseCount, inv.CreatedBy,
		inv.ExpiresAt, inv.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert invite: %w", err)
	}
	return nil
}

func (s *PostgresAuthStore) GetInviteByCode(ctx context.Context, code string) (*platauth.Invite, error) {
	var inv platauth.Invite
	var role string
	err := s.db.QueryRowContext(ctx,
		`SELECT id, workspace_id, code, email, role, max_uses, use_count, created_by, expires_at, created_at
		 FROM workspace_invites WHERE code = $1`, code).
		Scan(&inv.ID, &inv.WorkspaceID, &inv.Code, &inv.Email, &role,
			&inv.MaxUses, &inv.UseCount, &inv.CreatedBy, &inv.ExpiresAt, &inv.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get invite: %w", err)
	}
	inv.Role = platauth.Role(role)
	return &inv, nil
}

func (s *PostgresAuthStore) ListInvites(ctx context.Context, workspaceID string) ([]*platauth.Invite, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, workspace_id, code, email, role, max_uses, use_count, created_by, expires_at, created_at
		 FROM workspace_invites WHERE workspace_id = $1
		 ORDER BY created_at DESC`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list invites: %w", err)
	}
	defer rows.Close()

	result := make([]*platauth.Invite, 0)
	for rows.Next() {
		var inv platauth.Invite
		var role string
		if err := rows.Scan(&inv.ID, &inv.WorkspaceID, &inv.Code, &inv.Email, &role,
			&inv.MaxUses, &inv.UseCount, &inv.CreatedBy, &inv.ExpiresAt, &inv.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan invite: %w", err)
		}
		inv.Role = platauth.Role(role)
		result = append(result, &inv)
	}
	return result, rows.Err()
}

func (s *PostgresAuthStore) IncrementInviteUseCount(ctx context.Context, inviteID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE workspace_invites SET use_count = use_count + 1 WHERE id = $1`, inviteID)
	if err != nil {
		return fmt.Errorf("increment invite use count: %w", err)
	}
	return nil
}

func (s *PostgresAuthStore) DeleteInvite(ctx context.Context, inviteID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM workspace_invites WHERE id = $1`, inviteID)
	if err != nil {
		return fmt.Errorf("delete invite: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Refresh Tokens
// ---------------------------------------------------------------------------

func (s *PostgresAuthStore) StoreRefreshToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) (string, error) {
	id := uuid.NewString()
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO refresh_tokens (id, user_id, token_hash, expires_at, created_at) VALUES ($1, $2, $3, $4, $5)`,
		id, userID, tokenHash, expiresAt, now)
	if err != nil {
		return "", fmt.Errorf("insert refresh token: %w", err)
	}
	return id, nil
}

func (s *PostgresAuthStore) ValidateRefreshTokenByHash(ctx context.Context, tokenHash string) (string, error) {
	var id, userID string
	var expiresAt time.Time
	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, expires_at FROM refresh_tokens WHERE token_hash = $1`, tokenHash).
		Scan(&id, &userID, &expiresAt)
	if err != nil {
		return "", fmt.Errorf("refresh token not found: %w", err)
	}

	if time.Now().After(expiresAt) {
		_, _ = s.db.ExecContext(ctx, `DELETE FROM refresh_tokens WHERE id = $1`, id)
		return "", fmt.Errorf("refresh token expired")
	}

	// Single-use: delete after successful validation (token rotation).
	_, _ = s.db.ExecContext(ctx, `DELETE FROM refresh_tokens WHERE id = $1`, id)
	return userID, nil
}

func (s *PostgresAuthStore) RevokeRefreshToken(ctx context.Context, tokenID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM refresh_tokens WHERE id = $1`, tokenID)
	if err != nil {
		return fmt.Errorf("revoke refresh token: %w", err)
	}
	return nil
}

func (s *PostgresAuthStore) RevokeUserRefreshTokens(ctx context.Context, userID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM refresh_tokens WHERE user_id = $1`, userID)
	if err != nil {
		return fmt.Errorf("revoke user refresh tokens: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Scan helpers (PostgreSQL — uses time.Time directly for TIMESTAMPTZ)
// ---------------------------------------------------------------------------

func scanUserPg(row scanner) (*platauth.User, error) {
	var u platauth.User
	err := row.Scan(&u.ID, &u.Email, &u.Name, &u.AvatarURL, &u.OIDCSub, &u.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan user: %w", err)
	}
	return &u, nil
}

func scanWorkspacePg(row scanner) (*platauth.Workspace, error) {
	var w platauth.Workspace
	var wsType string
	err := row.Scan(&w.ID, &w.Name, &w.Slug, &w.Description, &w.LogoURL, &wsType, &w.CreatedAt, &w.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan workspace: %w", err)
	}
	w.Type = platauth.WorkspaceType(wsType)
	return &w, nil
}

func scanWorkspaceWithRolePg(row scanner) (*platauth.Workspace, error) {
	var w platauth.Workspace
	var wsType, role string
	err := row.Scan(&w.ID, &w.Name, &w.Slug, &w.Description, &w.LogoURL, &wsType, &w.CreatedAt, &w.UpdatedAt, &role)
	if err != nil {
		return nil, fmt.Errorf("scan workspace with role: %w", err)
	}
	w.Type = platauth.WorkspaceType(wsType)
	w.Role = platauth.Role(role)
	return &w, nil
}

func scanMembershipPg(row scanner) (*platauth.Membership, error) {
	var m platauth.Membership
	var role string
	err := row.Scan(&m.WorkspaceID, &m.UserID, &role, &m.JoinedAt)
	if err != nil {
		return nil, fmt.Errorf("scan membership: %w", err)
	}
	m.Role = platauth.Role(role)
	return &m, nil
}
