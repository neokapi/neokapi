// Package auth provides user authentication, workspace management,
// and authorization primitives for the gokapi platform.
package auth

import "time"

// User represents an authenticated user.
type User struct {
	ID        string
	Email     string
	Name      string
	AvatarURL string
	CreatedAt time.Time
}

// Workspace is the top-level organizational unit containing projects, members, and resources.
type Workspace struct {
	ID          string
	Name        string
	Slug        string // URL-friendly identifier
	Description string
	LogoURL     string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Role defines a member's permission level within a workspace.
type Role string

const (
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
	RoleMember Role = "member"
	RoleViewer Role = "viewer"
)

// ValidRoles is the set of valid Role values.
var ValidRoles = map[Role]bool{
	RoleOwner:  true,
	RoleAdmin:  true,
	RoleMember: true,
	RoleViewer: true,
}

// Membership ties a user to a workspace with a specific role.
type Membership struct {
	UserID      string
	WorkspaceID string
	Role        Role
	JoinedAt    time.Time
}
