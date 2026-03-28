package auth

import (
	"context"
	"time"

	platauth "github.com/neokapi/neokapi/platform/auth"
)

// AuthStore persists users, workspaces, and membership data.
type AuthStore interface {
	// Users
	CreateUser(ctx context.Context, u *platauth.User) error
	GetUser(ctx context.Context, id string) (*platauth.User, error)
	GetUserByEmail(ctx context.Context, email string) (*platauth.User, error)
	GetUserByOIDCSub(ctx context.Context, sub string) (*platauth.User, error)
	UpdateUser(ctx context.Context, u *platauth.User) error
	ListUsers(ctx context.Context, limit, offset int) ([]*platauth.User, error)
	SearchUsers(ctx context.Context, query string, limit int) ([]*platauth.User, error)

	// Workspaces
	CreateWorkspace(ctx context.Context, w *platauth.Workspace) error
	GetWorkspace(ctx context.Context, id string) (*platauth.Workspace, error)
	GetWorkspaceBySlug(ctx context.Context, slug string) (*platauth.Workspace, error)
	GetWorkspaceByAccessKey(ctx context.Context, key string) (*platauth.Workspace, error)
	ListWorkspaces(ctx context.Context, userID string) ([]*platauth.Workspace, error)
	ListPublicWorkspaces(ctx context.Context) ([]*platauth.Workspace, error)
	UpdateWorkspace(ctx context.Context, w *platauth.Workspace) error
	DeleteWorkspace(ctx context.Context, id string) error

	// Membership
	AddMember(ctx context.Context, workspaceID, userID string, role platauth.Role) error
	RemoveMember(ctx context.Context, workspaceID, userID string) error
	UpdateRole(ctx context.Context, workspaceID, userID string, role platauth.Role) error
	ListMembers(ctx context.Context, workspaceID string) ([]*platauth.Membership, error)
	GetMembership(ctx context.Context, workspaceID, userID string) (*platauth.Membership, error)

	// Unclaimed projects
	CreateUnclaimedProject(ctx context.Context, projectID, claimTokenHash, name, sourceLoc, targetLocs string, expiresAt time.Time) error
	GetUnclaimedByToken(ctx context.Context, claimTokenHash string) (*platauth.UnclaimedProject, error)
	DeleteUnclaimed(ctx context.Context, projectID string) error
	PurgeExpiredUnclaimed(ctx context.Context) (int, error)

	// Invitations
	CreateInvite(ctx context.Context, inv *platauth.Invite) error
	GetInviteByCode(ctx context.Context, code string) (*platauth.Invite, error)
	ListInvites(ctx context.Context, workspaceID string) ([]*platauth.Invite, error)
	IncrementInviteUseCount(ctx context.Context, inviteID string) error
	DeleteInvite(ctx context.Context, inviteID string) error

	// API tokens
	CreateAPIToken(ctx context.Context, token *platauth.APIToken, tokenHash string) error
	GetAPITokenByHash(ctx context.Context, tokenHash string) (*platauth.APIToken, error)
	ListAPITokens(ctx context.Context, workspaceID string) ([]*platauth.APIToken, error)
	DeleteAPIToken(ctx context.Context, id string) error
	UpdateAPITokenLastUsed(ctx context.Context, id string) error

	// Refresh tokens
	StoreRefreshToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) (string, error)
	ValidateRefreshTokenByHash(ctx context.Context, tokenHash string) (userID string, err error)
	RevokeRefreshToken(ctx context.Context, tokenID string) error
	RevokeUserRefreshTokens(ctx context.Context, userID string) error

	// Role templates
	CreateRoleTemplate(ctx context.Context, rt *platauth.RoleTemplate) error
	GetRoleTemplate(ctx context.Context, workspaceID, roleID string) (*platauth.RoleTemplate, error)
	ListRoleTemplates(ctx context.Context, workspaceID string) ([]*platauth.RoleTemplate, error)
	UpdateRoleTemplate(ctx context.Context, rt *platauth.RoleTemplate) error
	DeleteRoleTemplate(ctx context.Context, workspaceID, roleID string) error
	SeedDefaultRoleTemplates(ctx context.Context, workspaceID string) error

	// Project membership
	AddProjectMember(ctx context.Context, pm *platauth.ProjectMembership) error
	GetProjectMembership(ctx context.Context, projectID, userID string) (*platauth.ProjectMembership, error)
	ListProjectMembers(ctx context.Context, projectID string) ([]*platauth.ProjectMembership, error)
	UpdateProjectMember(ctx context.Context, pm *platauth.ProjectMembership) error
	RemoveProjectMember(ctx context.Context, projectID, userID string) error
	ResolveProjectPermissions(ctx context.Context, projectID, userID string) (*platauth.ResolvedPermission, error)

	// Lifecycle
	Close() error
}
