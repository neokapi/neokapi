package auth

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/neokapi/neokapi/bowrain/resilience"
	"golang.org/x/oauth2"
)

// OIDCConfig holds settings for connecting to an OIDC identity provider.
type OIDCConfig struct {
	IssuerURL    string
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

// identityTimeout bounds a single call to the identity provider — discovery,
// JWKS, or a token exchange. Sign-in is interactive: a user is watching, so a
// hung IdP must surface as a fast, clear failure rather than a spinner held
// open by the caller's request deadline.
const identityTimeout = 10 * time.Second

// IdentityContext returns ctx carrying the HTTP client that go-oidc and
// golang.org/x/oauth2 will use: bounded by a timeout and guarded by the shared
// identity circuit breaker.
//
// Both halves close real gaps. Without it these libraries fall back to
// http.DefaultClient, which has no timeout at all — an unresponsive IdP would
// hang the handler goroutine indefinitely, and because discovery runs
// per-request that is a whole-server hazard, not a per-user one. The breaker
// then stops a sustained IdP outage from parking every in-flight sign-in.
//
// An HTTP client already on ctx (the dev URL-rewriting transport, say) is
// preserved and wrapped rather than replaced.
func IdentityContext(ctx context.Context) context.Context {
	existing, _ := ctx.Value(oauth2.HTTPClient).(*http.Client)
	return oidc.ClientContext(ctx,
		resilience.Guard(existing, "identity:oidc", resilience.KindIdentity, identityTimeout))
}

// NewOIDCVerifier creates an OIDC ID-token verifier for the given issuer.
func NewOIDCVerifier(ctx context.Context, issuerURL, clientID string) (*oidc.IDTokenVerifier, error) {
	ctx = IdentityContext(ctx)
	provider, err := oidc.NewProvider(ctx, issuerURL)
	if err != nil {
		return nil, fmt.Errorf("create OIDC provider for %s: %w", issuerURL, err)
	}
	return provider.Verifier(&oidc.Config{ClientID: clientID}), nil
}

// NewOIDCAccessTokenVerifier creates an OIDC verifier that skips the audience
// check. Keycloak access tokens use aud="account" rather than the client ID,
// so the standard ID-token verifier rejects them. The issuer and signature
// checks still apply.
func NewOIDCAccessTokenVerifier(ctx context.Context, issuerURL string) (*oidc.IDTokenVerifier, error) {
	ctx = IdentityContext(ctx)
	provider, err := oidc.NewProvider(ctx, issuerURL)
	if err != nil {
		return nil, fmt.Errorf("create OIDC provider for %s: %w", issuerURL, err)
	}
	return provider.Verifier(&oidc.Config{SkipClientIDCheck: true}), nil
}

// NewOAuth2Config builds an OAuth2 config from the OIDC settings.
// It performs OIDC discovery to resolve the authorization and token endpoints.
func NewOAuth2Config(ctx context.Context, cfg OIDCConfig) (*oauth2.Config, error) {
	ctx = IdentityContext(ctx)
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
