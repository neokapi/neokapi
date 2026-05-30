package auth

import (
	"context"
	"time"

	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
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

	// Groups (teams)
	CreateGroup(ctx context.Context, g *platauth.Group) error
	ListGroups(ctx context.Context, workspaceID string) ([]*platauth.Group, error)
	DeleteGroup(ctx context.Context, workspaceID, groupID string) error
	AddGroupMember(ctx context.Context, groupID, userID string) error
	RemoveGroupMember(ctx context.Context, groupID, userID string) error
	ListGroupMembers(ctx context.Context, groupID string) ([]string, error)
	AddGroupRoleBinding(ctx context.Context, b *platauth.GroupRoleBinding) error
	ListGroupRoleBindings(ctx context.Context, groupID string) ([]*platauth.GroupRoleBinding, error)
	RemoveGroupRoleBinding(ctx context.Context, bindingID string) error

	// Deny rules (negative permissions)
	CreateDenyRule(ctx context.Context, r *platauth.DenyRule) error
	ListDenyRules(ctx context.Context, workspaceID string) ([]*platauth.DenyRule, error)
	DeleteDenyRule(ctx context.Context, workspaceID, ruleID string) error
	// ResolveDenies returns the union of permissions denied to a user for a
	// project, considering user-, role-, and group-subject rules.
	ResolveDenies(ctx context.Context, workspaceID, projectID, userID string, wsRole platauth.Role) (platauth.Permission, error)

	// Workspace role overrides (tune the workspace-role permission fallback)
	GetWorkspaceRoleOverride(ctx context.Context, workspaceID string, role platauth.Role) (platauth.Permission, bool, error)
	SetWorkspaceRoleOverride(ctx context.Context, workspaceID string, role platauth.Role, perms platauth.Permission) error
	ListWorkspaceRoleOverrides(ctx context.Context, workspaceID string) (map[platauth.Role]platauth.Permission, error)

	// Separation-of-duties policy
	GetSoDMode(ctx context.Context, workspaceID string) (platauth.SoDMode, error)
	SetSoDMode(ctx context.Context, workspaceID string, mode platauth.SoDMode) error

	// Workspace slug reservations (rename grace period).
	// ReserveSlug records that `slug` was previously held by `workspaceID` and
	// must not be reused until `until`. IsSlugReserved returns the workspace
	// that previously held the slug, if any reservation is still active.
	ReserveSlug(ctx context.Context, workspaceID, slug string, until time.Time) error
	IsSlugReserved(ctx context.Context, slug string) (workspaceID string, reservedUntil time.Time, ok bool, err error)
	// ListTakenSlugs returns the subset of `candidates` currently unavailable
	// because they're either active workspace slugs or live rename
	// reservations. Used by suggestion logic to test many candidates in one
	// round trip.
	ListTakenSlugs(ctx context.Context, candidates []string) (map[string]bool, error)
	ListSlugReservations(ctx context.Context) ([]*platauth.SlugReservation, error)
	ReleaseSlugReservation(ctx context.Context, slug string) error
	PurgeExpiredSlugReservations(ctx context.Context) (int, error)

	// Email change requests.
	CreateEmailChangeRequest(ctx context.Context, req *platauth.EmailChangeRequest, tokenHash string) error
	GetEmailChangeRequestByToken(ctx context.Context, tokenHash string) (*platauth.EmailChangeRequest, error)
	DeleteEmailChangeRequestsForUser(ctx context.Context, userID string) error
	PurgeExpiredEmailChangeRequests(ctx context.Context) (int, error)

	// Lifecycle
	Close() error
}
