// Package auth provides user authentication, workspace management,
// and authorization primitives for the gokapi platform.
package auth

import "time"

// User represents an authenticated user.
type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	AvatarURL string    `json:"avatar_url"`
	CreatedAt time.Time `json:"created_at"`
}

// Workspace is the top-level organizational unit containing projects, members, and resources.
type Workspace struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description string    `json:"description"`
	LogoURL     string    `json:"logo_url"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
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
	UserID      string    `json:"user_id"`
	WorkspaceID string    `json:"workspace_id"`
	Role        Role      `json:"role"`
	JoinedAt    time.Time `json:"joined_at"`
}
