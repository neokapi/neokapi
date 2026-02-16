package auth

import (
	"context"
	"time"
)

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

	// Unclaimed projects
	CreateUnclaimedProject(ctx context.Context, projectID, claimTokenHash, name, sourceLoc, targetLocs string, expiresAt time.Time) error
	GetUnclaimedByToken(ctx context.Context, claimTokenHash string) (*UnclaimedProject, error)
	DeleteUnclaimed(ctx context.Context, projectID string) error
	PurgeExpiredUnclaimed(ctx context.Context) (int, error)

	// Invitations
	CreateInvite(ctx context.Context, inv *Invite) error
	GetInviteByCode(ctx context.Context, code string) (*Invite, error)
	ListInvites(ctx context.Context, workspaceID string) ([]*Invite, error)
	IncrementInviteUseCount(ctx context.Context, inviteID string) error
	DeleteInvite(ctx context.Context, inviteID string) error

	// Lifecycle
	Close() error
}
