package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/gokapi/gokapi/internal/storage"
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

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO workspaces (id, name, slug, description, logo_url, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		w.ID, w.Name, w.Slug, w.Description, w.LogoURL,
		w.CreatedAt.Format(time.RFC3339), w.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("insert workspace: %w", err)
	}
	return nil
}

func (s *SQLiteAuthStore) GetWorkspace(ctx context.Context, id string) (*Workspace, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, slug, description, logo_url, created_at, updated_at
		 FROM workspaces WHERE id = ?`, id)
	return scanWorkspace(row)
}

func (s *SQLiteAuthStore) GetWorkspaceBySlug(ctx context.Context, slug string) (*Workspace, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, slug, description, logo_url, created_at, updated_at
		 FROM workspaces WHERE slug = ?`, slug)
	return scanWorkspace(row)
}

func (s *SQLiteAuthStore) ListWorkspaces(ctx context.Context, userID string) ([]*Workspace, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT w.id, w.name, w.slug, w.description, w.logo_url, w.created_at, w.updated_at
		 FROM workspaces w
		 JOIN workspace_members wm ON w.id = wm.workspace_id
		 WHERE wm.user_id = ?
		 ORDER BY w.name`, userID)
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	defer rows.Close()

	var result []*Workspace
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

	var result []*Membership
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
	var createdStr, updatedStr string
	err := row.Scan(&w.ID, &w.Name, &w.Slug, &w.Description, &w.LogoURL, &createdStr, &updatedStr)
	if err != nil {
		return nil, fmt.Errorf("scan workspace: %w", err)
	}
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
