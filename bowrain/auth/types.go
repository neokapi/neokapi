// Package auth provides server-side authentication (OIDC, SQLite store).
// Domain types (User, Workspace, Role, etc.) are defined in platform/auth
// and re-exported here via type aliases so that existing bowrain code
// continues to compile without widespread import changes.
package auth

import platauth "github.com/gokapi/gokapi/platform/auth"

// Type aliases — canonical definitions live in platform/auth.
type (
	User             = platauth.User
	Workspace        = platauth.Workspace
	WorkspaceType    = platauth.WorkspaceType
	Membership       = platauth.Membership
	Role             = platauth.Role
	Invite           = platauth.Invite
	UnclaimedProject = platauth.UnclaimedProject

	// JWT and device flow types.
	Claims             = platauth.Claims
	DeviceAuthResponse = platauth.DeviceAuthResponse
	TokenResponse      = platauth.TokenResponse
	DeviceFlowClient   = platauth.DeviceFlowClient
)

// Re-export constants.
const (
	RoleOwner  = platauth.RoleOwner
	RoleAdmin  = platauth.RoleAdmin
	RoleMember = platauth.RoleMember

	WorkspaceTypeTeam     = platauth.WorkspaceTypeTeam
	WorkspaceTypePersonal = platauth.WorkspaceTypePersonal
)

// Re-export variables.
var (
	ValidRoles    = platauth.ValidRoles
	GenerateToken = platauth.GenerateToken
	ValidateToken = platauth.ValidateToken
)
