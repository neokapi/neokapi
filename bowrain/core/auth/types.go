// Package auth provides user authentication, workspace management,
// and authorization primitives for the neokapi platform.
package auth

import "time"

// User represents an authenticated user.
//
// OnboardedAt is set when the user has completed onboarding (chosen a handle
// and had their personal workspace created). Until then, the user has a
// session but no workspace; the web app routes them through /welcome.
type User struct {
	ID          string     `json:"id"`
	Email       string     `json:"email"`
	Name        string     `json:"name"`
	AvatarURL   string     `json:"avatar_url"`
	OIDCSub     string     `json:"oidc_sub,omitempty"`
	OnboardedAt *time.Time `json:"onboarded_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// EmailChangeRequest is a pending email-change request.
// The plaintext token is sent to the new email address; the hash is stored.
type EmailChangeRequest struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	NewEmail  string    `json:"new_email"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// SlugReservation records that a workspace slug is held during a rename grace
// period and cannot be reused for another workspace until ReservedUntil.
type SlugReservation struct {
	Slug          string    `json:"slug"`
	WorkspaceID   string    `json:"workspace_id"`
	ReservedUntil time.Time `json:"reserved_until"`
	CreatedAt     time.Time `json:"created_at"`
}

// WorkspaceType distinguishes personal from team workspaces.
type WorkspaceType string

const (
	WorkspaceTypePersonal WorkspaceType = "personal"
	WorkspaceTypeTeam     WorkspaceType = "team"
)

// DashboardVisibility controls public access to the Pulse dashboard.
type DashboardVisibility string

const (
	DashboardPrivate  DashboardVisibility = "private"  // workspace members only
	DashboardUnlisted DashboardVisibility = "unlisted" // accessible via direct URL, not indexed
	DashboardPublic   DashboardVisibility = "public"   // listed, indexed, discoverable
)

// ValidDashboardVisibility is the set of valid DashboardVisibility values.
var ValidDashboardVisibility = map[DashboardVisibility]bool{
	DashboardPrivate:  true,
	DashboardUnlisted: true,
	DashboardPublic:   true,
}

// PulseTermSources controls which terminology sources are exposed on the Pulse dashboard.
type PulseTermSources struct {
	Terminology     bool `json:"terminology"`      // standard glossary terms
	BrandVocabulary bool `json:"brand_vocabulary"` // product names, taglines, do/don't
}

// Workspace is the top-level organizational unit containing projects, members, and resources.
type Workspace struct {
	ID                  string              `json:"id"`
	Name                string              `json:"name"`
	Slug                string              `json:"slug"`
	Description         string              `json:"description"`
	LogoURL             string              `json:"logo_url"`
	Type                WorkspaceType       `json:"type"`
	Languages           []string            `json:"languages,omitempty"`
	Plan                string              `json:"plan"`
	StripeCustomerID    string              `json:"stripe_customer_id,omitempty"`
	DashboardVisibility DashboardVisibility `json:"dashboard_visibility"`
	PulseAccessKey      string              `json:"pulse_access_key,omitempty"` // opaque key for unlisted dashboard access
	PulseTermSources    PulseTermSources    `json:"pulse_term_sources"`
	BrandVoiceProfileID string              `json:"brand_voice_profile_id,omitempty"` // default brand voice profile for workspace
	Role                Role                `json:"role,omitempty"`                   // current user's role (populated by list/get with user context)
	CreatedAt           time.Time           `json:"created_at"`
	UpdatedAt           time.Time           `json:"updated_at"`
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

// RoleTemplate defines a named permission bundle within a workspace.
// Workspace admins can create, rename, and customize role templates.
type RoleTemplate struct {
	ID          string     `json:"id"`
	WorkspaceID string     `json:"workspace_id"`
	Name        string     `json:"name"`
	DisplayName string     `json:"display_name"`
	Description string     `json:"description"`
	Permissions Permission `json:"permissions"`
	IsBuiltin   bool       `json:"is_builtin"`
	Position    int        `json:"position"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// ProjectMembership ties a user to a project with a role template and optional language scope.
type ProjectMembership struct {
	ProjectID   string    `json:"project_id"`
	UserID      string    `json:"user_id"`
	RoleID      string    `json:"role_id"`
	WorkspaceID string    `json:"workspace_id"`
	Languages   []string  `json:"languages"`
	CreatedAt   time.Time `json:"created_at"`
}

// ResolvedPermission is the effective permissions for a user in a project context.
type ResolvedPermission struct {
	Permissions Permission `json:"permissions"`
	Languages   []string   `json:"languages"` // empty = all languages
}

// Group is a named set of users within a workspace that can be bound to project
// roles in bulk (teams).
type Group struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspace_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	MemberCount int       `json:"member_count,omitempty"`
}

// GroupRoleBinding binds a group to a role template on a project, optionally
// scoped to specific languages.
type GroupRoleBinding struct {
	ID          string    `json:"id"`
	GroupID     string    `json:"group_id"`
	WorkspaceID string    `json:"workspace_id"`
	ProjectID   string    `json:"project_id"`
	RoleID      string    `json:"role_id"`
	Languages   []string  `json:"languages"`
	CreatedAt   time.Time `json:"created_at"`
}

// DenySubjectType identifies what a deny rule applies to.
type DenySubjectType string

const (
	DenySubjectUser  DenySubjectType = "user"
	DenySubjectRole  DenySubjectType = "role"  // workspace role (owner/admin/member/viewer)
	DenySubjectGroup DenySubjectType = "group" // group ID
)

// DenyRule is a negative permission that always overrides grants. A rule with an
// empty ProjectID applies workspace-wide.
type DenyRule struct {
	ID          string          `json:"id"`
	WorkspaceID string          `json:"workspace_id"`
	SubjectType DenySubjectType `json:"subject_type"`
	SubjectID   string          `json:"subject_id"`
	ProjectID   string          `json:"project_id"` // empty = workspace-wide
	DeniedPerms Permission      `json:"denied_perms"`
	Reason      string          `json:"reason"`
	CreatedAt   time.Time       `json:"created_at"`
}

// SoDMode is the separation-of-duties enforcement level for a workspace.
type SoDMode string

const (
	SoDOff   SoDMode = "off"   // no separation enforced
	SoDWarn  SoDMode = "warn"  // record a warning but allow
	SoDBlock SoDMode = "block" // reject the action
)

// ValidSoDModes is the set of valid SoDMode values.
var ValidSoDModes = map[SoDMode]bool{SoDOff: true, SoDWarn: true, SoDBlock: true}

// DefaultPermissionsForRole returns the fallback project permissions when no explicit
// project membership exists, based on the user's workspace role.
func DefaultPermissionsForRole(wsRole Role) *ResolvedPermission {
	switch wsRole {
	case RoleOwner, RoleAdmin:
		return &ResolvedPermission{Permissions: PermAll}
	case RoleMember:
		return &ResolvedPermission{Permissions: PermViewContent | PermTranslate | PermManageFiles | PermRunFlows}
	default:
		return &ResolvedPermission{Permissions: PermViewContent}
	}
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

// AgentMode defines @bravo's interaction mode.
type AgentMode string

const (
	AgentModeAsk      AgentMode = "ask"      // read-only, advisory
	AgentModeCoworker AgentMode = "coworker" // full tool access
	AgentModeVoice    AgentMode = "voice"    // brand voice scoped
)

// ValidAgentModes is the set of valid AgentMode values.
var ValidAgentModes = map[AgentMode]bool{
	AgentModeAsk:      true,
	AgentModeCoworker: true,
	AgentModeVoice:    true,
}

// ModePermissionCeiling returns the maximum permissions allowed for a given agent mode.
func ModePermissionCeiling(mode AgentMode) Permission {
	switch mode {
	case AgentModeAsk:
		return PermViewContent
	case AgentModeCoworker:
		return PermAll
	case AgentModeVoice:
		return PermViewContent | PermManageBrand | PermReview
	default:
		return PermViewContent // safe default
	}
}

// SessionGrant represents a just-in-time, ephemeral permission scope for
// an @bravo conversation or MCP tool session.
type SessionGrant struct {
	SessionID   string     `json:"session_id"`  // conversation ID or MCP session ID
	UserID      string     `json:"user_id"`     // who granted
	Permissions Permission `json:"permissions"` // bitmask subset of user's permissions
	Languages   []string   `json:"languages"`   // language constraint (empty = all)
	ProjectIDs  []string   `json:"project_ids"` // project constraint (empty = all)
	Mode        AgentMode  `json:"mode"`        // current interaction mode
	ExpiresAt   time.Time  `json:"expires_at"`  // auto-expire
}
