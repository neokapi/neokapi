package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/core/id"
)

// PostgresAuthStore implements AuthStore using PostgreSQL.
type PostgresAuthStore struct {
	db *storage.PgDB
}

// NewAuthStore opens a PostgreSQL-backed AuthStore.
func NewAuthStore(connStr string) (*PostgresAuthStore, error) {
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

// NewAuthStoreFromDB wraps an existing PgDB for auth use.
func NewAuthStoreFromDB(db *storage.PgDB) (*PostgresAuthStore, error) {
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
		u.ID = id.New()
	}
	if u.CreatedAt.IsZero() {
		u.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO users (id, email, name, avatar_url, oidc_sub, onboarded_at, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		u.ID, u.Email, u.Name, u.AvatarURL, u.OIDCSub, u.OnboardedAt, u.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert user: %w", err)
	}
	return nil
}

func (s *PostgresAuthStore) GetUser(ctx context.Context, id string) (*platauth.User, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, email, name, avatar_url, oidc_sub, onboarded_at, created_at FROM users WHERE id = $1`, id)
	return scanUserPg(row)
}

func (s *PostgresAuthStore) GetUserByEmail(ctx context.Context, email string) (*platauth.User, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, email, name, avatar_url, oidc_sub, onboarded_at, created_at FROM users WHERE email = $1`, email)
	return scanUserPg(row)
}

func (s *PostgresAuthStore) GetUserByOIDCSub(ctx context.Context, sub string) (*platauth.User, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, email, name, avatar_url, oidc_sub, onboarded_at, created_at FROM users WHERE oidc_sub = $1`, sub)
	return scanUserPg(row)
}

func (s *PostgresAuthStore) SearchUsers(ctx context.Context, query string, limit int) ([]*platauth.User, error) {
	pattern := "%" + query + "%"
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, email, name, avatar_url, oidc_sub, onboarded_at, created_at FROM users
		 WHERE email ILIKE $1 OR name ILIKE $1
		 ORDER BY email ASC LIMIT $2`, pattern, limit)
	if err != nil {
		return nil, fmt.Errorf("search users: %w", err)
	}
	defer rows.Close()
	var users []*platauth.User
	for rows.Next() {
		u, err := scanUserPg(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *PostgresAuthStore) ListUsers(ctx context.Context, limit, offset int) ([]*platauth.User, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, email, name, avatar_url, oidc_sub, onboarded_at, created_at FROM users ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
		limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []*platauth.User
	for rows.Next() {
		u, err := scanUserPg(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *PostgresAuthStore) UpdateUser(ctx context.Context, u *platauth.User) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE users SET email=$1, name=$2, avatar_url=$3, oidc_sub=$4, onboarded_at=$5 WHERE id=$6`,
		u.Email, u.Name, u.AvatarURL, u.OIDCSub, u.OnboardedAt, u.ID)
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
	if w.DashboardVisibility == "" {
		w.DashboardVisibility = platauth.DashboardPrivate
	}
	termSrcJSON, _ := json.Marshal(w.PulseTermSources)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO workspaces (id, name, slug, description, logo_url, type, plan, stripe_customer_id, dashboard_visibility, pulse_access_key, pulse_term_sources, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		w.ID, w.Name, w.Slug, w.Description, w.LogoURL, string(w.Type),
		w.Plan, w.StripeCustomerID, string(w.DashboardVisibility), w.PulseAccessKey, string(termSrcJSON),
		w.CreatedAt, w.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert workspace: %w", err)
	}
	return nil
}

func (s *PostgresAuthStore) GetWorkspace(ctx context.Context, id string) (*platauth.Workspace, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, slug, description, logo_url, type, plan, stripe_customer_id, dashboard_visibility, pulse_access_key, pulse_term_sources, created_at, updated_at
		 FROM workspaces WHERE id = $1`, id)
	return scanWorkspacePg(row)
}

func (s *PostgresAuthStore) GetWorkspaceBySlug(ctx context.Context, slug string) (*platauth.Workspace, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, slug, description, logo_url, type, plan, stripe_customer_id, dashboard_visibility, pulse_access_key, pulse_term_sources, created_at, updated_at
		 FROM workspaces WHERE slug = $1`, slug)
	return scanWorkspacePg(row)
}

func (s *PostgresAuthStore) GetWorkspaceByAccessKey(ctx context.Context, key string) (*platauth.Workspace, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, slug, description, logo_url, type, plan, stripe_customer_id, dashboard_visibility, pulse_access_key, pulse_term_sources, created_at, updated_at
		 FROM workspaces WHERE pulse_access_key = $1 AND pulse_access_key != ''`, key)
	return scanWorkspacePg(row)
}

func (s *PostgresAuthStore) ListWorkspaces(ctx context.Context, userID string) ([]*platauth.Workspace, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT w.id, w.name, w.slug, w.description, w.logo_url, w.type, w.plan, w.stripe_customer_id, w.dashboard_visibility, w.pulse_access_key, w.pulse_term_sources, w.created_at, w.updated_at, wm.role
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

func (s *PostgresAuthStore) ListPublicWorkspaces(ctx context.Context) ([]*platauth.Workspace, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, slug, description, logo_url, type, plan, stripe_customer_id, dashboard_visibility, pulse_access_key, pulse_term_sources, created_at, updated_at
		 FROM workspaces
		 WHERE dashboard_visibility = 'public'
		 ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("list public workspaces: %w", err)
	}
	defer rows.Close()

	result := make([]*platauth.Workspace, 0)
	for rows.Next() {
		w, err := scanWorkspacePg(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, w)
	}
	return result, rows.Err()
}

func (s *PostgresAuthStore) UpdateWorkspace(ctx context.Context, w *platauth.Workspace) error {
	w.UpdatedAt = time.Now().UTC()
	termSrcJSON, _ := json.Marshal(w.PulseTermSources)
	res, err := s.db.ExecContext(ctx,
		`UPDATE workspaces SET name=$1, slug=$2, description=$3, logo_url=$4, plan=$5, stripe_customer_id=$6, dashboard_visibility=$7, pulse_access_key=$8, pulse_term_sources=$9, updated_at=$10 WHERE id=$11`,
		w.Name, w.Slug, w.Description, w.LogoURL, w.Plan, w.StripeCustomerID,
		string(w.DashboardVisibility), w.PulseAccessKey, string(termSrcJSON), w.UpdatedAt, w.ID)
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
		return errors.New("membership not found")
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
		return errors.New("membership not found")
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
		`INSERT INTO unclaimed_projects (project_id, claim_token, name, default_source_language, target_languages, expires_at)
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
		`SELECT project_id, claim_token, name, default_source_language, target_languages, created_at, expires_at
		 FROM unclaimed_projects WHERE claim_token = $1`, claimTokenHash).
		Scan(&p.ProjectID, &p.ClaimToken, &p.Name, &p.DefaultSourceLanguage, &p.TargetLanguages, &p.CreatedAt, &p.ExpiresAt)
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
		inv.ID = id.New()
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
	id := id.New()
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
		if _, err := s.db.ExecContext(ctx, `DELETE FROM refresh_tokens WHERE id = $1`, id); err != nil {
			slog.Warn("failed to delete expired refresh token", "id", id, "error", err)
		}
		return "", errors.New("refresh token expired")
	}

	// Single-use: delete after successful validation (token rotation).
	if _, err := s.db.ExecContext(ctx, `DELETE FROM refresh_tokens WHERE id = $1`, id); err != nil {
		slog.Warn("failed to delete consumed refresh token", "id", id, "error", err)
	}
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
// API Tokens
// ---------------------------------------------------------------------------

func (s *PostgresAuthStore) CreateAPIToken(ctx context.Context, token *platauth.APIToken, tokenHash string) error {
	if token.ID == "" {
		token.ID = id.New()
	}
	if token.CreatedAt.IsZero() {
		token.CreatedAt = time.Now().UTC()
	}
	if token.Scopes == "" {
		token.Scopes = `["*"]`
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO api_tokens (id, user_id, workspace_id, name, token_hash, token_prefix, scopes, expires_at, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		token.ID, token.UserID, token.WorkspaceID, token.Name, tokenHash,
		token.TokenPrefix, token.Scopes, token.ExpiresAt, token.CreatedAt)
	if err != nil {
		return fmt.Errorf("insert api token: %w", err)
	}
	return nil
}

func (s *PostgresAuthStore) GetAPITokenByHash(ctx context.Context, tokenHash string) (*platauth.APIToken, error) {
	var tok platauth.APIToken
	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, workspace_id, name, token_prefix, scopes, last_used_at, expires_at, created_at
		 FROM api_tokens WHERE token_hash = $1`, tokenHash).
		Scan(&tok.ID, &tok.UserID, &tok.WorkspaceID, &tok.Name, &tok.TokenPrefix,
			&tok.Scopes, &tok.LastUsedAt, &tok.ExpiresAt, &tok.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get api token: %w", err)
	}
	return &tok, nil
}

func (s *PostgresAuthStore) ListAPITokens(ctx context.Context, workspaceID string) ([]*platauth.APIToken, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, workspace_id, name, token_prefix, scopes, last_used_at, expires_at, created_at
		 FROM api_tokens WHERE workspace_id = $1
		 ORDER BY created_at DESC`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list api tokens: %w", err)
	}
	defer rows.Close()

	result := make([]*platauth.APIToken, 0)
	for rows.Next() {
		var tok platauth.APIToken
		if err := rows.Scan(&tok.ID, &tok.UserID, &tok.WorkspaceID, &tok.Name, &tok.TokenPrefix,
			&tok.Scopes, &tok.LastUsedAt, &tok.ExpiresAt, &tok.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan api token: %w", err)
		}
		result = append(result, &tok)
	}
	return result, rows.Err()
}

func (s *PostgresAuthStore) DeleteAPIToken(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM api_tokens WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete api token: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("api token %s not found", id)
	}
	return nil
}

func (s *PostgresAuthStore) UpdateAPITokenLastUsed(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE api_tokens SET last_used_at = NOW() WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("update api token last used: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Role Templates
// ---------------------------------------------------------------------------

func (s *PostgresAuthStore) CreateRoleTemplate(ctx context.Context, rt *platauth.RoleTemplate) error {
	if rt.ID == "" {
		rt.ID = id.New()
	}
	now := time.Now().UTC()
	if rt.CreatedAt.IsZero() {
		rt.CreatedAt = now
	}
	rt.UpdatedAt = now
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO role_templates (id, workspace_id, name, display_name, description, permissions, is_builtin, position, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		rt.ID, rt.WorkspaceID, rt.Name, rt.DisplayName, rt.Description,
		int64(rt.Permissions), rt.IsBuiltin, rt.Position, rt.CreatedAt, rt.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert role template: %w", err)
	}
	return nil
}

func (s *PostgresAuthStore) GetRoleTemplate(ctx context.Context, workspaceID, roleID string) (*platauth.RoleTemplate, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, workspace_id, name, display_name, description, permissions, is_builtin, position, created_at, updated_at
		 FROM role_templates WHERE workspace_id = $1 AND id = $2`, workspaceID, roleID)
	return scanRoleTemplatePg(row)
}

func (s *PostgresAuthStore) ListRoleTemplates(ctx context.Context, workspaceID string) ([]*platauth.RoleTemplate, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, workspace_id, name, display_name, description, permissions, is_builtin, position, created_at, updated_at
		 FROM role_templates WHERE workspace_id = $1
		 ORDER BY position, name`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list role templates: %w", err)
	}
	defer rows.Close()

	result := make([]*platauth.RoleTemplate, 0)
	for rows.Next() {
		rt, err := scanRoleTemplatePg(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, rt)
	}
	return result, rows.Err()
}

func (s *PostgresAuthStore) UpdateRoleTemplate(ctx context.Context, rt *platauth.RoleTemplate) error {
	rt.UpdatedAt = time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE role_templates SET name=$1, display_name=$2, description=$3, permissions=$4, position=$5, updated_at=$6
		 WHERE workspace_id=$7 AND id=$8`,
		rt.Name, rt.DisplayName, rt.Description, int64(rt.Permissions), rt.Position, rt.UpdatedAt,
		rt.WorkspaceID, rt.ID)
	if err != nil {
		return fmt.Errorf("update role template: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("role template not found")
	}
	return nil
}

func (s *PostgresAuthStore) DeleteRoleTemplate(ctx context.Context, workspaceID, roleID string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM role_templates WHERE workspace_id=$1 AND id=$2 AND is_builtin = FALSE`,
		workspaceID, roleID)
	if err != nil {
		return fmt.Errorf("delete role template: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("role template not found or is builtin")
	}
	return nil
}

func (s *PostgresAuthStore) SeedDefaultRoleTemplates(ctx context.Context, workspaceID string) error {
	for _, def := range platauth.DefaultRoleTemplates {
		rt := def // copy
		rt.ID = id.New()
		rt.WorkspaceID = workspaceID
		if err := s.CreateRoleTemplate(ctx, &rt); err != nil {
			return fmt.Errorf("seed role template %s: %w", def.Name, err)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Project Membership
// ---------------------------------------------------------------------------

func (s *PostgresAuthStore) AddProjectMember(ctx context.Context, pm *platauth.ProjectMembership) error {
	if pm.CreatedAt.IsZero() {
		pm.CreatedAt = time.Now().UTC()
	}
	langs := "[]"
	if len(pm.Languages) > 0 {
		langs = marshalLanguages(pm.Languages)
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO project_members (project_id, user_id, role_id, workspace_id, languages, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		pm.ProjectID, pm.UserID, pm.RoleID, pm.WorkspaceID, langs, pm.CreatedAt)
	if err != nil {
		return fmt.Errorf("add project member: %w", err)
	}
	return nil
}

func (s *PostgresAuthStore) GetProjectMembership(ctx context.Context, projectID, userID string) (*platauth.ProjectMembership, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT project_id, user_id, role_id, workspace_id, languages, created_at
		 FROM project_members WHERE project_id = $1 AND user_id = $2`, projectID, userID)
	return scanProjectMemberPg(row)
}

func (s *PostgresAuthStore) ListProjectMembers(ctx context.Context, projectID string) ([]*platauth.ProjectMembership, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT project_id, user_id, role_id, workspace_id, languages, created_at
		 FROM project_members WHERE project_id = $1
		 ORDER BY created_at`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list project members: %w", err)
	}
	defer rows.Close()

	result := make([]*platauth.ProjectMembership, 0)
	for rows.Next() {
		pm, err := scanProjectMemberPg(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, pm)
	}
	return result, rows.Err()
}

func (s *PostgresAuthStore) UpdateProjectMember(ctx context.Context, pm *platauth.ProjectMembership) error {
	langs := "[]"
	if len(pm.Languages) > 0 {
		langs = marshalLanguages(pm.Languages)
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE project_members SET role_id=$1, languages=$2 WHERE project_id=$3 AND user_id=$4`,
		pm.RoleID, langs, pm.ProjectID, pm.UserID)
	if err != nil {
		return fmt.Errorf("update project member: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("project membership not found")
	}
	return nil
}

func (s *PostgresAuthStore) RemoveProjectMember(ctx context.Context, projectID, userID string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM project_members WHERE project_id=$1 AND user_id=$2`,
		projectID, userID)
	if err != nil {
		return fmt.Errorf("remove project member: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("project membership not found")
	}
	return nil
}

func (s *PostgresAuthStore) ResolveProjectPermissions(ctx context.Context, projectID, userID string) (*platauth.ResolvedPermission, error) {
	// Union direct project membership with any group-role bindings the user has
	// on this project. A user with only a group binding still resolves here.
	rows, err := s.db.QueryContext(ctx,
		`SELECT rt.permissions, pm.languages
		 FROM project_members pm
		 JOIN role_templates rt ON rt.workspace_id = pm.workspace_id AND rt.id = pm.role_id
		 WHERE pm.project_id = $1 AND pm.user_id = $2
		 UNION ALL
		 SELECT rt.permissions, grb.languages
		 FROM group_role_bindings grb
		 JOIN group_members gm ON gm.group_id = grb.group_id
		 JOIN role_templates rt ON rt.workspace_id = grb.workspace_id AND rt.id = grb.role_id
		 WHERE grb.project_id = $1 AND gm.user_id = $2`, projectID, userID)
	if err != nil {
		return nil, fmt.Errorf("resolve project permissions: %w", err)
	}
	defer rows.Close()

	var perms platauth.Permission
	var languages []string
	seen := map[string]bool{}
	allLanguages := false
	any := false
	for rows.Next() {
		var p int64
		var langsStr string
		if err := rows.Scan(&p, &langsStr); err != nil {
			return nil, fmt.Errorf("scan project permissions: %w", err)
		}
		any = true
		perms |= platauth.Permission(p)
		langs := unmarshalLanguages(langsStr)
		if len(langs) == 0 {
			allLanguages = true // an unconstrained source grants all languages
			continue
		}
		for _, l := range langs {
			if !seen[l] {
				seen[l] = true
				languages = append(languages, l)
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate project permissions: %w", err)
	}
	if !any {
		return nil, fmt.Errorf("resolve project permissions: %w", sql.ErrNoRows)
	}
	if allLanguages {
		languages = nil
	}
	return &platauth.ResolvedPermission{Permissions: perms, Languages: languages}, nil
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

type scanner interface {
	Scan(dest ...any) error
}

// parsePulseTermSources unmarshals JSON into PulseTermSources with defaults.
func parsePulseTermSources(raw string, dst *platauth.PulseTermSources) {
	dst.Terminology = true
	dst.BrandVocabulary = false
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), dst)
	}
}

// ---------------------------------------------------------------------------
// Scan helpers (PostgreSQL — uses time.Time directly for TIMESTAMPTZ)
// ---------------------------------------------------------------------------

func scanUserPg(row scanner) (*platauth.User, error) {
	var u platauth.User
	err := row.Scan(&u.ID, &u.Email, &u.Name, &u.AvatarURL, &u.OIDCSub, &u.OnboardedAt, &u.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan user: %w", err)
	}
	return &u, nil
}

func scanWorkspacePg(row scanner) (*platauth.Workspace, error) {
	var w platauth.Workspace
	var wsType string
	var stripeCustomerID *string
	var dashVis, termSrc string
	err := row.Scan(&w.ID, &w.Name, &w.Slug, &w.Description, &w.LogoURL, &wsType, &w.Plan, &stripeCustomerID, &dashVis, &w.PulseAccessKey, &termSrc, &w.CreatedAt, &w.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan workspace: %w", err)
	}
	w.Type = platauth.WorkspaceType(wsType)
	w.DashboardVisibility = platauth.DashboardVisibility(dashVis)
	if w.DashboardVisibility == "" {
		w.DashboardVisibility = platauth.DashboardPrivate
	}
	parsePulseTermSources(termSrc, &w.PulseTermSources)
	if stripeCustomerID != nil {
		w.StripeCustomerID = *stripeCustomerID
	}
	return &w, nil
}

func scanWorkspaceWithRolePg(row scanner) (*platauth.Workspace, error) {
	var w platauth.Workspace
	var wsType, role string
	var stripeCustomerID *string
	var dashVis, termSrc string
	err := row.Scan(&w.ID, &w.Name, &w.Slug, &w.Description, &w.LogoURL, &wsType, &w.Plan, &stripeCustomerID, &dashVis, &w.PulseAccessKey, &termSrc, &w.CreatedAt, &w.UpdatedAt, &role)
	if err != nil {
		return nil, fmt.Errorf("scan workspace with role: %w", err)
	}
	w.Type = platauth.WorkspaceType(wsType)
	w.DashboardVisibility = platauth.DashboardVisibility(dashVis)
	if w.DashboardVisibility == "" {
		w.DashboardVisibility = platauth.DashboardPrivate
	}
	parsePulseTermSources(termSrc, &w.PulseTermSources)
	w.Role = platauth.Role(role)
	if stripeCustomerID != nil {
		w.StripeCustomerID = *stripeCustomerID
	}
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

func scanRoleTemplatePg(row scanner) (*platauth.RoleTemplate, error) {
	var rt platauth.RoleTemplate
	var perms int64
	err := row.Scan(&rt.ID, &rt.WorkspaceID, &rt.Name, &rt.DisplayName, &rt.Description,
		&perms, &rt.IsBuiltin, &rt.Position, &rt.CreatedAt, &rt.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan role template: %w", err)
	}
	rt.Permissions = platauth.Permission(perms)
	return &rt, nil
}

func scanProjectMemberPg(row scanner) (*platauth.ProjectMembership, error) {
	var pm platauth.ProjectMembership
	var langsStr string
	err := row.Scan(&pm.ProjectID, &pm.UserID, &pm.RoleID, &pm.WorkspaceID, &langsStr, &pm.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("scan project member: %w", err)
	}
	pm.Languages = unmarshalLanguages(langsStr)
	return &pm, nil
}
