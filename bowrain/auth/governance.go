package auth

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/core/id"
)

// ---------------------------------------------------------------------------
// Groups (teams)
// ---------------------------------------------------------------------------

// CreateGroup inserts a group. ID is generated if empty.
func (s *PostgresAuthStore) CreateGroup(ctx context.Context, g *platauth.Group) error {
	if g.ID == "" {
		g.ID = id.New()
	}
	if g.CreatedAt.IsZero() {
		g.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO groups (id, workspace_id, name, description, created_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		g.ID, g.WorkspaceID, g.Name, g.Description, g.CreatedAt)
	return err
}

// ListGroups returns all groups in a workspace with their member counts.
func (s *PostgresAuthStore) ListGroups(ctx context.Context, workspaceID string) ([]*platauth.Group, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT g.id, g.workspace_id, g.name, g.description, g.created_at,
		        (SELECT COUNT(*) FROM group_members gm WHERE gm.group_id = g.id)
		 FROM groups g WHERE g.workspace_id = $1 ORDER BY g.name`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*platauth.Group
	for rows.Next() {
		var g platauth.Group
		if err := rows.Scan(&g.ID, &g.WorkspaceID, &g.Name, &g.Description, &g.CreatedAt, &g.MemberCount); err != nil {
			return nil, err
		}
		out = append(out, &g)
	}
	return out, rows.Err()
}

// DeleteGroup removes a group (cascades members + bindings).
func (s *PostgresAuthStore) DeleteGroup(ctx context.Context, workspaceID, groupID string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM groups WHERE id = $1 AND workspace_id = $2`, groupID, workspaceID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("group %s not found", groupID)
	}
	return nil
}

// AddGroupMember adds a user to a group (idempotent).
func (s *PostgresAuthStore) AddGroupMember(ctx context.Context, groupID, userID string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO group_members (group_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		groupID, userID)
	return err
}

// RemoveGroupMember removes a user from a group.
func (s *PostgresAuthStore) RemoveGroupMember(ctx context.Context, groupID, userID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM group_members WHERE group_id = $1 AND user_id = $2`, groupID, userID)
	return err
}

// ListGroupMembers returns the user IDs in a group.
func (s *PostgresAuthStore) ListGroupMembers(ctx context.Context, groupID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT user_id FROM group_members WHERE group_id = $1`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			return nil, err
		}
		ids = append(ids, uid)
	}
	return ids, rows.Err()
}

// AddGroupRoleBinding binds a group to a project role.
func (s *PostgresAuthStore) AddGroupRoleBinding(ctx context.Context, b *platauth.GroupRoleBinding) error {
	if b.ID == "" {
		b.ID = id.New()
	}
	if b.CreatedAt.IsZero() {
		b.CreatedAt = time.Now().UTC()
	}
	langs := "[]"
	if len(b.Languages) > 0 {
		langs = marshalLanguages(b.Languages)
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO group_role_bindings (id, group_id, workspace_id, project_id, role_id, languages, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		b.ID, b.GroupID, b.WorkspaceID, b.ProjectID, b.RoleID, langs, b.CreatedAt)
	return err
}

// ListGroupRoleBindings returns the bindings for a group.
func (s *PostgresAuthStore) ListGroupRoleBindings(ctx context.Context, groupID string) ([]*platauth.GroupRoleBinding, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, group_id, workspace_id, project_id, role_id, languages, created_at
		 FROM group_role_bindings WHERE group_id = $1 ORDER BY created_at`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*platauth.GroupRoleBinding
	for rows.Next() {
		var b platauth.GroupRoleBinding
		var langs string
		if err := rows.Scan(&b.ID, &b.GroupID, &b.WorkspaceID, &b.ProjectID, &b.RoleID, &langs, &b.CreatedAt); err != nil {
			return nil, err
		}
		b.Languages = unmarshalLanguages(langs)
		out = append(out, &b)
	}
	return out, rows.Err()
}

// RemoveGroupRoleBinding deletes a binding.
func (s *PostgresAuthStore) RemoveGroupRoleBinding(ctx context.Context, bindingID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM group_role_bindings WHERE id = $1`, bindingID)
	return err
}

// ---------------------------------------------------------------------------
// Deny rules
// ---------------------------------------------------------------------------

// CreateDenyRule inserts a deny rule.
func (s *PostgresAuthStore) CreateDenyRule(ctx context.Context, r *platauth.DenyRule) error {
	if r.ID == "" {
		r.ID = id.New()
	}
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now().UTC()
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO deny_rules (id, workspace_id, subject_type, subject_id, project_id, denied_perms, reason, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		r.ID, r.WorkspaceID, string(r.SubjectType), r.SubjectID, r.ProjectID, int64(r.DeniedPerms), r.Reason, r.CreatedAt)
	return err
}

// ListDenyRules returns all deny rules for a workspace.
func (s *PostgresAuthStore) ListDenyRules(ctx context.Context, workspaceID string) ([]*platauth.DenyRule, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, workspace_id, subject_type, subject_id, project_id, denied_perms, reason, created_at
		 FROM deny_rules WHERE workspace_id = $1 ORDER BY created_at DESC`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*platauth.DenyRule
	for rows.Next() {
		var r platauth.DenyRule
		var subjType string
		var perms int64
		if err := rows.Scan(&r.ID, &r.WorkspaceID, &subjType, &r.SubjectID, &r.ProjectID, &perms, &r.Reason, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.SubjectType = platauth.DenySubjectType(subjType)
		r.DeniedPerms = platauth.Permission(perms)
		out = append(out, &r)
	}
	return out, rows.Err()
}

// DeleteDenyRule removes a deny rule.
func (s *PostgresAuthStore) DeleteDenyRule(ctx context.Context, workspaceID, ruleID string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM deny_rules WHERE id = $1 AND workspace_id = $2`, ruleID, workspaceID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("deny rule %s not found", ruleID)
	}
	return nil
}

// ResolveDenies returns the union of all permissions denied to a user for a
// project, considering user-, role-, and group-subject rules (workspace-wide
// rules have an empty project_id).
func (s *PostgresAuthStore) ResolveDenies(ctx context.Context, workspaceID, projectID, userID string, wsRole platauth.Role) (platauth.Permission, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT denied_perms FROM deny_rules
		 WHERE workspace_id = $1 AND (project_id = '' OR project_id = $2)
		   AND (
		     (subject_type = 'user' AND subject_id = $3)
		     OR (subject_type = 'role' AND subject_id = $4)
		     OR (subject_type = 'group' AND subject_id IN (SELECT group_id FROM group_members WHERE user_id = $3))
		   )`,
		workspaceID, projectID, userID, string(wsRole))
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var denied platauth.Permission
	for rows.Next() {
		var p int64
		if err := rows.Scan(&p); err != nil {
			return 0, err
		}
		denied |= platauth.Permission(p)
	}
	return denied, rows.Err()
}

// ---------------------------------------------------------------------------
// Workspace role overrides
// ---------------------------------------------------------------------------

// GetWorkspaceRoleOverride returns the override permissions for a workspace role,
// if one is configured.
func (s *PostgresAuthStore) GetWorkspaceRoleOverride(ctx context.Context, workspaceID string, role platauth.Role) (platauth.Permission, bool, error) {
	var perms int64
	err := s.db.QueryRowContext(ctx,
		`SELECT permissions FROM workspace_role_overrides WHERE workspace_id = $1 AND role = $2`,
		workspaceID, string(role)).Scan(&perms)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return platauth.Permission(perms), true, nil
}

// SetWorkspaceRoleOverride upserts an override for a workspace role.
func (s *PostgresAuthStore) SetWorkspaceRoleOverride(ctx context.Context, workspaceID string, role platauth.Role, perms platauth.Permission) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO workspace_role_overrides (workspace_id, role, permissions)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (workspace_id, role) DO UPDATE SET permissions = EXCLUDED.permissions`,
		workspaceID, string(role), int64(perms))
	return err
}

// ListWorkspaceRoleOverrides returns all configured role overrides for a workspace.
func (s *PostgresAuthStore) ListWorkspaceRoleOverrides(ctx context.Context, workspaceID string) (map[platauth.Role]platauth.Permission, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT role, permissions FROM workspace_role_overrides WHERE workspace_id = $1`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[platauth.Role]platauth.Permission{}
	for rows.Next() {
		var role string
		var perms int64
		if err := rows.Scan(&role, &perms); err != nil {
			return nil, err
		}
		out[platauth.Role(role)] = platauth.Permission(perms)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Separation-of-duties policy
// ---------------------------------------------------------------------------

// GetSoDMode returns the separation-of-duties mode for a workspace, defaulting
// to "warn" when unset.
func (s *PostgresAuthStore) GetSoDMode(ctx context.Context, workspaceID string) (platauth.SoDMode, error) {
	var mode string
	err := s.db.QueryRowContext(ctx,
		`SELECT mode FROM sod_settings WHERE workspace_id = $1`, workspaceID).Scan(&mode)
	if err == sql.ErrNoRows {
		return platauth.SoDWarn, nil
	}
	if err != nil {
		return platauth.SoDWarn, err
	}
	return platauth.SoDMode(mode), nil
}

// SetSoDMode upserts the separation-of-duties mode for a workspace.
func (s *PostgresAuthStore) SetSoDMode(ctx context.Context, workspaceID string, mode platauth.SoDMode) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sod_settings (workspace_id, mode, updated_at)
		 VALUES ($1, $2, NOW())
		 ON CONFLICT (workspace_id) DO UPDATE SET mode = EXCLUDED.mode, updated_at = NOW()`,
		workspaceID, string(mode))
	return err
}
