package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/gokapi/gokapi/bowrain/auth"
	"github.com/gokapi/gokapi/core/id"
	platauth "github.com/gokapi/gokapi/platform/auth"
)

// AuthService provides authentication and workspace business logic.
type AuthService struct {
	store     auth.AuthStore
	jwtSecret string
}

// NewAuthService creates a new AuthService.
func NewAuthService(store auth.AuthStore, jwtSecret string) *AuthService {
	return &AuthService{store: store, jwtSecret: jwtSecret}
}

// GetOrCreateUser finds a user by email, or creates one if not found.
// Used during OIDC login to upsert the user record.
// On first creation, a personal workspace is auto-created.
// If oidcSub is provided and the existing user lacks one, it is backfilled.
func (s *AuthService) GetOrCreateUser(ctx context.Context, email, name, avatarURL, oidcSub string) (*platauth.User, error) {
	u, err := s.store.GetUserByEmail(ctx, email)
	if err == nil {
		// Backfill OIDC subject if not yet stored.
		if oidcSub != "" && u.OIDCSub == "" {
			u.OIDCSub = oidcSub
			_ = s.store.UpdateUser(ctx, u)
		}
		return u, nil
	}
	u = &platauth.User{
		Email:     email,
		Name:      name,
		AvatarURL: avatarURL,
		OIDCSub:   oidcSub,
	}
	if err := s.store.CreateUser(ctx, u); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	// Auto-create personal workspace for new users.
	slug := personalSlug(email)
	slug, err = s.uniqueSlug(ctx, slug)
	if err != nil {
		return u, nil // user created, workspace creation is best-effort
	}
	displayName := name
	if displayName == "" {
		displayName = slug
	}
	w := &platauth.Workspace{
		Name: displayName,
		Slug: slug,
		Type: platauth.WorkspaceTypePersonal,
	}
	if createErr := s.CreateWorkspaceWithOwner(ctx, w, u.ID); createErr != nil {
		// Log but don't fail — user was created successfully.
		return u, nil
	}

	return u, nil
}

// personalSlug derives a workspace slug from an email address.
func personalSlug(email string) string {
	parts := strings.SplitN(email, "@", 2)
	slug := strings.ToLower(parts[0])
	// Replace common non-slug characters.
	slug = strings.NewReplacer(".", "-", "_", "-", "+", "-").Replace(slug)
	return slug
}

// uniqueSlug checks if a slug is taken and appends -2, -3, etc. if needed.
func (s *AuthService) uniqueSlug(ctx context.Context, base string) (string, error) {
	slug := base
	for i := 2; i <= 100; i++ {
		_, err := s.store.GetWorkspaceBySlug(ctx, slug)
		if err != nil {
			return slug, nil // not found, slug is available
		}
		slug = fmt.Sprintf("%s-%d", base, i)
	}
	return "", fmt.Errorf("could not find unique slug for %s", base)
}

// GenerateToken creates a JWT for the given user.
func (s *AuthService) GenerateToken(user *platauth.User, expiry time.Duration) (string, error) {
	return platauth.GenerateToken(user, s.jwtSecret, expiry)
}

// CreateWorkspaceWithOwner creates a workspace and adds the user as owner.
func (s *AuthService) CreateWorkspaceWithOwner(ctx context.Context, w *platauth.Workspace, ownerID string) error {
	if err := s.store.CreateWorkspace(ctx, w); err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}
	if err := s.store.AddMember(ctx, w.ID, ownerID, platauth.RoleOwner); err != nil {
		return fmt.Errorf("add owner: %w", err)
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
		return "", "", fmt.Errorf("invalid claim token")
	}

	if time.Now().After(unclaimed.ExpiresAt) {
		return "", "", fmt.Errorf("claim token expired")
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
		return "", "", fmt.Errorf("no personal workspace found")
	}

	// Delete unclaimed record (project will be associated by the caller).
	if err := s.store.DeleteUnclaimed(ctx, unclaimed.ProjectID); err != nil {
		return "", "", fmt.Errorf("delete unclaimed: %w", err)
	}

	return unclaimed.ProjectID, personalWS.Slug, nil
}

// CreateInvite creates a workspace invitation.
func (s *AuthService) CreateInvite(ctx context.Context, workspaceID, createdBy string, role platauth.Role, email string, maxUses int, ttl time.Duration) (*platauth.Invite, error) {
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
	return inv, nil
}

// CreateAPIToken generates a new API token for the given user and workspace.
// Returns the APIToken (with ID populated) and the plaintext token (shown once).
func (s *AuthService) CreateAPIToken(ctx context.Context, userID, workspaceID, name string, expiresAt *time.Time) (*platauth.APIToken, string, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, "", fmt.Errorf("generate api token: %w", err)
	}
	plaintext := "bwt_" + hex.EncodeToString(tokenBytes)

	hash := sha256.Sum256([]byte(plaintext))
	tokenHash := hex.EncodeToString(hash[:])

	token := &platauth.APIToken{
		UserID:      userID,
		WorkspaceID: workspaceID,
		Name:        name,
		TokenPrefix: plaintext[:8], // "bwt_" + first 4 hex chars
		Scopes:      `["*"]`,
		ExpiresAt:   expiresAt,
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
		return nil, fmt.Errorf("invalid api token")
	}

	if token.ExpiresAt != nil && time.Now().After(*token.ExpiresAt) {
		return nil, fmt.Errorf("api token expired")
	}

	// Fire-and-forget last-used update.
	go func() {
		_ = s.store.UpdateAPITokenLastUsed(context.Background(), token.ID)
	}()

	return token, nil
}

// AcceptInvite adds a user to the workspace if the invite is valid.
func (s *AuthService) AcceptInvite(ctx context.Context, code, userID string) error {
	inv, err := s.store.GetInviteByCode(ctx, code)
	if err != nil {
		return fmt.Errorf("invalid invite code")
	}

	if time.Now().After(inv.ExpiresAt) {
		return fmt.Errorf("invite expired")
	}

	if inv.MaxUses > 0 && inv.UseCount >= inv.MaxUses {
		return fmt.Errorf("invite has been fully used")
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

	return nil
}
