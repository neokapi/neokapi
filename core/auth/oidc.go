package auth

import (
	"context"
	"fmt"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// OIDCConfig holds settings for connecting to an OIDC identity provider.
type OIDCConfig struct {
	IssuerURL    string
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

// NewOIDCVerifier creates an OIDC ID-token verifier for the given issuer.
func NewOIDCVerifier(ctx context.Context, issuerURL, clientID string) (*oidc.IDTokenVerifier, error) {
	provider, err := oidc.NewProvider(ctx, issuerURL)
	if err != nil {
		return nil, fmt.Errorf("create OIDC provider for %s: %w", issuerURL, err)
	}
	return provider.Verifier(&oidc.Config{ClientID: clientID}), nil
}

// NewOAuth2Config builds an OAuth2 config from the OIDC settings.
// It performs OIDC discovery to resolve the authorization and token endpoints.
func NewOAuth2Config(ctx context.Context, cfg OIDCConfig) (*oauth2.Config, error) {
	provider, err := oidc.NewProvider(ctx, cfg.IssuerURL)
	if err != nil {
		return nil, fmt.Errorf("create OIDC provider for %s: %w", cfg.IssuerURL, err)
	}
	return &oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  cfg.RedirectURL,
		Endpoint:     provider.Endpoint(),
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
	}, nil
}
