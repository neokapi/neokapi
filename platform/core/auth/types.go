// Package auth provides user authentication, workspace management,
// and authorization primitives for the neokapi platform.
package auth

import "time"

// User represents an authenticated user.
type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	AvatarURL string    `json:"avatar_url"`
	OIDCSub   string    `json:"oidc_sub,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// WorkspaceType distinguishes personal from team workspaces.
type WorkspaceType string

const (
	WorkspaceTypePersonal WorkspaceType = "personal"
	WorkspaceTypeTeam     WorkspaceType = "team"
)

// Workspace is the top-level organizational unit containing projects, members, and resources.
type Workspace struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Slug        string        `json:"slug"`
	Description string        `json:"description"`
	LogoURL     string        `json:"logo_url"`
	Type        WorkspaceType `json:"type"`
	Languages   []string      `json:"languages,omitempty"`
	Role        Role          `json:"role,omitempty"` // current user's role (populated by list/get with user context)
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
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

// UnclaimedProject represents an anonymous project awaiting user claim.
type UnclaimedProject struct {
	ProjectID             string    `json:"project_id"`
	ClaimToken            string    `json:"-"` // hashed token stored in DB
	Name                  string    `json:"name"`
	DefaultSourceLanguage string    `json:"default_source_language"`
	TargetLanguages       string    `json:"target_languages"` // comma-separated
	CreatedAt             time.Time `json:"created_at"`
	ExpiresAt             time.Time `json:"expires_at"`
}

// APIToken represents a long-lived, revocable API token for CI/CD and programmatic access.
type APIToken struct {
	ID          string     `json:"id"`
	UserID      string     `json:"user_id"`
	WorkspaceID string     `json:"workspace_id"`
	Name        string     `json:"name"`
	TokenPrefix string     `json:"token_prefix"` // first 8 chars for display
	Scopes      string     `json:"scopes"`       // JSON array, default '["*"]'
	LastUsedAt  *time.Time `json:"last_used_at"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// Invite represents a workspace invitation.
type Invite struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	Code        string    `json:"code"`
	Email       string    `json:"email,omitempty"`
	Role        Role      `json:"role"`
	MaxUses     int       `json:"max_uses"`
	UseCount    int       `json:"use_count"`
	CreatedBy   string    `json:"created_by"`
	ExpiresAt   time.Time `json:"expires_at"`
	CreatedAt   time.Time `json:"created_at"`
}
