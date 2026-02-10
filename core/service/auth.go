package service

import (
	"context"
	"fmt"
	"time"

	"github.com/gokapi/gokapi/core/auth"
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
func (s *AuthService) GetOrCreateUser(ctx context.Context, email, name, avatarURL string) (*auth.User, error) {
	u, err := s.store.GetUserByEmail(ctx, email)
	if err == nil {
		return u, nil
	}
	u = &auth.User{
		Email:     email,
		Name:      name,
		AvatarURL: avatarURL,
	}
	if err := s.store.CreateUser(ctx, u); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}
	return u, nil
}

// GenerateToken creates a JWT for the given user.
func (s *AuthService) GenerateToken(user *auth.User, expiry time.Duration) (string, error) {
	return auth.GenerateToken(user, s.jwtSecret, expiry)
}

// CreateWorkspaceWithOwner creates a workspace and adds the user as owner.
func (s *AuthService) CreateWorkspaceWithOwner(ctx context.Context, w *auth.Workspace, ownerID string) error {
	if err := s.store.CreateWorkspace(ctx, w); err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}
	if err := s.store.AddMember(ctx, w.ID, ownerID, auth.RoleOwner); err != nil {
		return fmt.Errorf("add owner: %w", err)
	}
	return nil
}

// Store returns the underlying AuthStore.
func (s *AuthService) Store() auth.AuthStore {
	return s.store
}
