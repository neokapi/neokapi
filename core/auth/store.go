package auth

import "context"

// AuthStore persists users, workspaces, and membership data.
type AuthStore interface {
	// Users
	CreateUser(ctx context.Context, u *User) error
	GetUser(ctx context.Context, id string) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	UpdateUser(ctx context.Context, u *User) error

	// Workspaces
	CreateWorkspace(ctx context.Context, w *Workspace) error
	GetWorkspace(ctx context.Context, id string) (*Workspace, error)
	GetWorkspaceBySlug(ctx context.Context, slug string) (*Workspace, error)
	ListWorkspaces(ctx context.Context, userID string) ([]*Workspace, error)
	UpdateWorkspace(ctx context.Context, w *Workspace) error
	DeleteWorkspace(ctx context.Context, id string) error

	// Membership
	AddMember(ctx context.Context, workspaceID, userID string, role Role) error
	RemoveMember(ctx context.Context, workspaceID, userID string) error
	UpdateRole(ctx context.Context, workspaceID, userID string, role Role) error
	ListMembers(ctx context.Context, workspaceID string) ([]*Membership, error)
	GetMembership(ctx context.Context, workspaceID, userID string) (*Membership, error)

	// Lifecycle
	Close() error
}
