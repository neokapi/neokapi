package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/neokapi/neokapi/bowrain/analytics"
	"github.com/neokapi/neokapi/bowrain/auth"
	platauth "github.com/neokapi/neokapi/bowrain/core/auth"
	"github.com/neokapi/neokapi/core/id"
)

// AuthService provides authentication and workspace business logic.
type AuthService struct {
	store     auth.AuthStore
	jwtSecret string

	// tracker captures workspace/member/project lifecycle analytics events.
	// Optional; nil disables capture (see Services.SetEventTracker).
	tracker EventTracker
}

// NewAuthService creates a new AuthService.
func NewAuthService(store auth.AuthStore, jwtSecret string) *AuthService {
	return &AuthService{store: store, jwtSecret: jwtSecret}
}

// GetOrCreateUser finds a user by email, or creates one if not found.
// Used during OIDC login to upsert the user record.
//
// On first creation, the user record is created but no personal workspace
// is provisioned — the web app routes new users through /welcome where
// they pick a handle, after which CompleteOnboarding creates the workspace.
// If oidcSub is provided and the existing user lacks one, it is backfilled.
// locale is the sign-in request's preferred locale (BCP-47 primary subtag,
// e.g. from Accept-Language); it is stored on first creation and backfilled
// onto existing users that have no locale yet. Empty means unknown.
func (s *AuthService) GetOrCreateUser(ctx context.Context, email, name, avatarURL, oidcSub, locale string) (*platauth.User, error) {
	if email == "" {
		return nil, errors.New("email is required")
	}
	u, err := s.store.GetUserByEmail(ctx, email)
	if err == nil {
		// Backfill OIDC subject and locale if not yet stored.
		changed := false
		if oidcSub != "" && u.OIDCSub == "" {
			u.OIDCSub = oidcSub
			changed = true
		}
		if locale != "" && u.Locale == "" {
			u.Locale = locale
			changed = true
		}
		if changed {
			_ = s.store.UpdateUser(ctx, u)
		}
		return u, nil
	}
	u = &platauth.User{
		Email:     email,
		Name:      name,
		AvatarURL: avatarURL,
		OIDCSub:   oidcSub,
		Locale:    locale,
	}
	if err := s.store.CreateUser(ctx, u); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	return u, nil
}

// NeedsOnboarding reports whether the user has not yet completed onboarding
// (chosen a handle and had their personal workspace created).
//
// Existing users created before the onboarding flow existed have OnboardedAt
// nil but already have a personal workspace; they are lazily marked as
// onboarded so they bypass /welcome.
func (s *AuthService) NeedsOnboarding(ctx context.Context, userID string) (bool, error) {
	u, err := s.store.GetUser(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("get user: %w", err)
	}
	if u.OnboardedAt != nil {
		return false, nil
	}
	workspaces, err := s.store.ListWorkspaces(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("list workspaces: %w", err)
	}
	for _, w := range workspaces {
		if w.Type == platauth.WorkspaceTypePersonal {
			now := time.Now().UTC()
			u.OnboardedAt = &now
			_ = s.store.UpdateUser(ctx, u)
			return false, nil
		}
	}
	return true, nil
}

// SuggestSlug derives a candidate handle from an email address. The result
// may collide with an existing workspace or reserved slug; callers should
// resolve to a unique value via FindAvailableSlug or surface validation
// errors to the user.
func SuggestSlug(email string) string {
	return platauth.SuggestSlug(email)
}

// findAvailableSlugMaxAttempts caps how many `base`, `base-2`, … `base-N`
// candidates we evaluate before giving up. The whole batch is checked in one
// query, so the cap is about UX (avoid suggesting "asgeirf-87"), not cost.
const findAvailableSlugMaxAttempts = 100

// FindAvailableSlug returns the first slug derived from `base` that is not
// taken and not reserved, by appending -2, -3, etc. as needed.
//
// All candidates are tested in a single round trip via ListTakenSlugs, so
// this stays at one DB query no matter how crowded the chosen base is.
func (s *AuthService) FindAvailableSlug(ctx context.Context, base string) (string, error) {
	if base == "" {
		return "", errors.New("base slug is required")
	}
	candidates := make([]string, 0, findAvailableSlugMaxAttempts)
	candidates = append(candidates, base)
	for i := 2; i <= findAvailableSlugMaxAttempts; i++ {
		candidates = append(candidates, fmt.Sprintf("%s-%d", base, i))
	}
	taken, err := s.store.ListTakenSlugs(ctx, candidates)
	if err != nil {
		return "", err
	}
	for _, c := range candidates {
		if !taken[c] {
			return c, nil
		}
	}
	return "", fmt.Errorf("could not find unique slug for %s", base)
}

// IsSlugAvailable reports whether the given slug is well-formed and not in
// use (either as an active workspace slug or as a reserved-on-rename slug).
// The returned reason is a short tag suitable for client display
// ("invalid", "reserved", "taken") when available is false.
func (s *AuthService) IsSlugAvailable(ctx context.Context, slug string) (bool, string, error) {
	if err := platauth.ValidateWorkspaceSlug(slug); err != nil {
		return false, "invalid", nil
	}
	if _, err := s.store.GetWorkspaceBySlug(ctx, slug); err == nil {
		return false, "taken", nil
	}
	if _, _, reserved, err := s.store.IsSlugReserved(ctx, slug); err != nil {
		return false, "", fmt.Errorf("check reserved: %w", err)
	} else if reserved {
		return false, "reserved", nil
	}
	return true, "", nil
}

// CompleteOnboarding marks the user as onboarded and creates their personal
// workspace with the chosen slug. If the user is already onboarded with a
// personal workspace, it is returned without modification (idempotent).
func (s *AuthService) CompleteOnboarding(ctx context.Context, userID, slug, displayName string) (*platauth.Workspace, error) {
	if userID == "" {
		return nil, errors.New("user ID is required")
	}
	if slug == "" {
		return nil, errors.New("slug is required")
	}
	u, err := s.store.GetUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	// Idempotent: if user already has a personal workspace, return it.
	if existing, err := s.findPersonalWorkspace(ctx, userID); err == nil && existing != nil {
		if u.OnboardedAt == nil {
			now := time.Now().UTC()
			u.OnboardedAt = &now
			_ = s.store.UpdateUser(ctx, u)
		}
		return existing, nil
	}
	if err := platauth.ValidateWorkspaceSlug(slug); err != nil {
		return nil, err
	}
	taken, err := s.isSlugTaken(ctx, slug)
	if err != nil {
		return nil, err
	}
	if taken {
		return nil, fmt.Errorf("slug %q is not available", slug)
	}
	if displayName == "" {
		displayName = u.Name
	}
	if displayName == "" {
		displayName = slug
	}
	w := &platauth.Workspace{
		Name: displayName,
		Slug: slug,
		Type: platauth.WorkspaceTypePersonal,
	}
	if err := s.CreateWorkspaceWithOwner(ctx, w, userID); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	u.OnboardedAt = &now
	if err := s.store.UpdateUser(ctx, u); err != nil {
		return w, fmt.Errorf("mark onboarded: %w", err)
	}
	return w, nil
}

// isSlugTaken reports whether `slug` is in use as an active workspace slug
// or held by an active rename reservation.
func (s *AuthService) isSlugTaken(ctx context.Context, slug string) (bool, error) {
	if _, err := s.store.GetWorkspaceBySlug(ctx, slug); err == nil {
		return true, nil
	}
	_, _, reserved, err := s.store.IsSlugReserved(ctx, slug)
	if err != nil {
		return false, fmt.Errorf("check reserved: %w", err)
	}
	return reserved, nil
}

// findPersonalWorkspace returns the user's personal workspace if any.
func (s *AuthService) findPersonalWorkspace(ctx context.Context, userID string) (*platauth.Workspace, error) {
	workspaces, err := s.store.ListWorkspaces(ctx, userID)
	if err != nil {
		return nil, err
	}
	for _, w := range workspaces {
		if w.Type == platauth.WorkspaceTypePersonal {
			return w, nil
		}
	}
	return nil, nil
}

// GenerateToken creates a JWT for the given user.
func (s *AuthService) GenerateToken(user *platauth.User, expiry time.Duration) (string, error) {
	return platauth.GenerateToken(user, s.jwtSecret, expiry)
}

// CreateWorkspaceWithOwner creates a workspace and adds the user as owner.
func (s *AuthService) CreateWorkspaceWithOwner(ctx context.Context, w *platauth.Workspace, ownerID string) error {
	if w == nil {
		return errors.New("workspace is required")
	}
	if w.Name == "" {
		return errors.New("workspace name is required")
	}
	if w.Slug == "" {
		return errors.New("workspace slug is required")
	}
	if ownerID == "" {
		return errors.New("owner ID is required")
	}
	if err := s.store.CreateWorkspace(ctx, w); err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}
	if err := s.store.AddMember(ctx, w.ID, ownerID, platauth.RoleOwner); err != nil {
		return fmt.Errorf("add owner: %w", err)
	}
	// Seed default role templates for the new workspace.
	seedErr := s.store.SeedDefaultRoleTemplates(ctx, w.ID)

	props := analytics.Props(w.ID, "")
	props["workspace_type"] = string(w.Type)
	track(s.tracker, ownerID, analytics.EventWorkspaceCreated, props)

	if seedErr != nil {
		// Log but don't fail — workspace and owner were created successfully.
		return nil
	}
	return nil
}

// Store returns the underlying AuthStore.
func (s *AuthService) Store() auth.AuthStore {
	return s.store
}

// CreateAnonymousProject creates an unclaimed project with a claim token.
// Returns the plaintext claim token (caller must persist it).
func (s *AuthService) CreateAnonymousProject(ctx context.Context, name, sourceLoc string, targetLocs []string) (projectID, claimToken string, err error) {
	if name == "" {
		return "", "", errors.New("project name is required")
	}
	if sourceLoc == "" {
		return "", "", errors.New("source locale is required")
	}
	projectID = id.New()

	// Generate claim token: clm_ + 32 random hex bytes.
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", "", fmt.Errorf("generate claim token: %w", err)
	}
	claimToken = "clm_" + hex.EncodeToString(tokenBytes)

	// Hash the token for storage.
	hash := sha256.Sum256([]byte(claimToken))
	tokenHash := hex.EncodeToString(hash[:])

	targets := strings.Join(targetLocs, ",")
	expiresAt := time.Now().Add(7 * 24 * time.Hour) // 7 days

	if err := s.store.CreateUnclaimedProject(ctx, projectID, tokenHash, name, sourceLoc, targets, expiresAt); err != nil {
		return "", "", fmt.Errorf("create unclaimed project: %w", err)
	}

	return projectID, claimToken, nil
}

// ClaimProject moves an unclaimed project into the user's personal workspace.
func (s *AuthService) ClaimProject(ctx context.Context, userID, claimToken string) (projectID, workspaceSlug string, err error) {
	hash := sha256.Sum256([]byte(claimToken))
	tokenHash := hex.EncodeToString(hash[:])

	unclaimed, err := s.store.GetUnclaimedByToken(ctx, tokenHash)
	if err != nil {
		return "", "", errors.New("invalid claim token")
	}

	if time.Now().After(unclaimed.ExpiresAt) {
		return "", "", errors.New("claim token expired")
	}

	// Find user's personal workspace.
	workspaces, err := s.store.ListWorkspaces(ctx, userID)
	if err != nil {
		return "", "", fmt.Errorf("list workspaces: %w", err)
	}

	var personalWS *platauth.Workspace
	for _, ws := range workspaces {
		if ws.Type == platauth.WorkspaceTypePersonal {
			personalWS = ws
			break
		}
	}
	if personalWS == nil {
		return "", "", errors.New("no personal workspace found")
	}

	// Delete unclaimed record (project will be associated by the caller).
	if err := s.store.DeleteUnclaimed(ctx, unclaimed.ProjectID); err != nil {
		return "", "", fmt.Errorf("delete unclaimed: %w", err)
	}

	track(s.tracker, userID, analytics.EventProjectClaimed,
		analytics.Props(personalWS.ID, unclaimed.ProjectID))

	return unclaimed.ProjectID, personalWS.Slug, nil
}

// CreateInvite creates a workspace invitation.
func (s *AuthService) CreateInvite(ctx context.Context, workspaceID, createdBy string, role platauth.Role, email string, maxUses int, ttl time.Duration) (*platauth.Invite, error) {
	if workspaceID == "" {
		return nil, errors.New("workspace ID is required")
	}
	if createdBy == "" {
		return nil, errors.New("creator ID is required")
	}
	codeBytes := make([]byte, 16)
	if _, err := rand.Read(codeBytes); err != nil {
		return nil, fmt.Errorf("generate invite code: %w", err)
	}

	inv := &platauth.Invite{
		WorkspaceID: workspaceID,
		Code:        hex.EncodeToString(codeBytes),
		Email:       email,
		Role:        role,
		MaxUses:     maxUses,
		CreatedBy:   createdBy,
		ExpiresAt:   time.Now().Add(ttl),
	}
	if err := s.store.CreateInvite(ctx, inv); err != nil {
		return nil, err
	}

	props := analytics.Props(workspaceID, "")
	props["role"] = string(role)
	track(s.tracker, createdBy, analytics.EventMemberInvited, props)

	return inv, nil
}

// APITokenSpec describes a token to mint. It is a struct rather than a
// parameter list because UserID, WorkspaceID, Name, AgentName and ScopesJSON
// are five adjacent strings, and adjacent strings get transposed.
type APITokenSpec struct {
	// UserID is the person the token belongs to and whose membership decides
	// what it may do. Required, machine token or not.
	UserID      string
	WorkspaceID string
	Name        string
	// AgentName, when set, makes this a machine token: work authored under it
	// is attributed to "agent/<AgentName>" instead of to UserID. Empty leaves
	// the token an ordinary personal one.
	AgentName string
	// ScopesJSON is a JSON array of scope strings (`["*"]`,
	// `["translate:fr,de"]`). Empty defaults to full access at the store.
	ScopesJSON string
	ExpiresAt  *time.Time
}

// CreateAPIToken generates a new API token from a spec. Returns the APIToken
// (with ID populated) and the plaintext token (shown once).
func (s *AuthService) CreateAPIToken(ctx context.Context, spec APITokenSpec) (*platauth.APIToken, string, error) {
	if spec.UserID == "" {
		return nil, "", errors.New("user ID is required")
	}
	if spec.WorkspaceID == "" {
		return nil, "", errors.New("workspace ID is required")
	}
	if spec.Name == "" {
		return nil, "", errors.New("token name is required")
	}
	if err := platauth.ValidateAgentName(spec.AgentName); err != nil {
		return nil, "", err
	}
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, "", fmt.Errorf("generate api token: %w", err)
	}
	plaintext := "bwt_" + hex.EncodeToString(tokenBytes)

	hash := sha256.Sum256([]byte(plaintext))
	tokenHash := hex.EncodeToString(hash[:])

	token := &platauth.APIToken{
		UserID:      spec.UserID,
		WorkspaceID: spec.WorkspaceID,
		Name:        spec.Name,
		AgentName:   spec.AgentName,
		TokenPrefix: plaintext[:8], // "bwt_" + first 4 hex chars
		Scopes:      spec.ScopesJSON,
		ExpiresAt:   spec.ExpiresAt,
	}

	if err := s.store.CreateAPIToken(ctx, token, tokenHash); err != nil {
		return nil, "", err
	}

	return token, plaintext, nil
}

// ValidateAPIToken validates a plaintext API token and returns the associated token record.
func (s *AuthService) ValidateAPIToken(ctx context.Context, plaintext string) (*platauth.APIToken, error) {
	hash := sha256.Sum256([]byte(plaintext))
	tokenHash := hex.EncodeToString(hash[:])

	token, err := s.store.GetAPITokenByHash(ctx, tokenHash)
	if err != nil {
		return nil, errors.New("invalid api token")
	}

	if token.ExpiresAt != nil && time.Now().After(*token.ExpiresAt) {
		return nil, errors.New("api token expired")
	}

	// Fire-and-forget last-used update: detached from the request's
	// cancellation (the write should complete even if the caller disconnects)
	// but keeps the request's trace/values.
	//
	// Nothing above this goroutine can recover for it, so it recovers for
	// itself: a panicking store would otherwise take the process down over a
	// bookkeeping write whose failure does not affect the validated token.
	lastUsedCtx := context.WithoutCancel(ctx)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("recovered panic in API token last-used update", "panic", r)
			}
		}()
		if err := s.store.UpdateAPITokenLastUsed(lastUsedCtx, token.ID); err != nil {
			slog.Debug("update API token last-used", "token_id", token.ID, "error", err)
		}
	}()

	return token, nil
}

// AcceptInvite adds a user to the workspace if the invite is valid.
//
// An invite carries a role, so possession of the code decides what the accepting
// user becomes — up to owner. Two invite shapes are supported, and the
// difference is exactly whether Email is set:
//
//   - An address-bound invite (Email set) is an invitation to one person. The
//     accepting user's account email must match it; the code alone is not
//     sufficient, so a link that leaks — forwarded, in a mailbox someone else
//     reads, in a link preview — cannot be redeemed by whoever holds it.
//   - A link invite (Email empty) is deliberately bearer-style: it is how a
//     workspace shares one join URL with several people, which is why MaxUses
//     is a first-class field. Its only gates are expiry and the use count.
//
// Creating either is already restricted to admins and owners
// (HandleCreateInvite), so the bearer shape stays an explicit choice by someone
// who could have added the member directly.
func (s *AuthService) AcceptInvite(ctx context.Context, code, userID string) error {
	inv, err := s.store.GetInviteByCode(ctx, code)
	if err != nil {
		return errors.New("invalid invite code")
	}

	if time.Now().After(inv.ExpiresAt) {
		return errors.New("invite expired")
	}

	if inv.MaxUses > 0 && inv.UseCount >= inv.MaxUses {
		return errors.New("invite has been fully used")
	}

	// Address-bound invite: the redeeming identity must be the invited one.
	if invEmail := strings.TrimSpace(inv.Email); invEmail != "" {
		u, err := s.store.GetUser(ctx, userID)
		if err != nil {
			return errors.New("invite is for a different account")
		}
		if !strings.EqualFold(strings.TrimSpace(u.Email), invEmail) {
			// Deliberately does not echo the invited address: the caller has
			// only proved they hold the code, which is not grounds to learn who
			// it was sent to.
			return errors.New("invite is for a different account")
		}
	}

	// If the user is already a member, treat as success (idempotent).
	if _, err := s.store.GetMembership(ctx, inv.WorkspaceID, userID); err == nil {
		return nil
	}

	if err := s.store.AddMember(ctx, inv.WorkspaceID, userID, inv.Role); err != nil {
		return fmt.Errorf("add member: %w", err)
	}

	if err := s.store.IncrementInviteUseCount(ctx, inv.ID); err != nil {
		return fmt.Errorf("increment use count: %w", err)
	}

	props := analytics.Props(inv.WorkspaceID, "")
	props["role"] = string(inv.Role)
	track(s.tracker, userID, analytics.EventMemberJoined, props)

	return nil
}
