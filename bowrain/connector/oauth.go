package connector

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"

	"github.com/neokapi/neokapi/core/httputil"
)

// oauthConfig captures the credential material a remote OAuth2 connector needs
// to authenticate to a provider API (Google Workspace, Microsoft Graph). It is
// populated from the connector's config map and supports three modes, selected
// automatically by which fields are present:
//
//  1. App-only client credentials — clientID + clientSecret + tokenURL
//     (+ optional scopes), no user token. Tokens are minted and refreshed on
//     demand from the client secret, so nothing is persisted between calls.
//     This is the Microsoft 365 connector's default for unattended,
//     admin-consented server sync.
//  2. Refreshing user OAuth (3-legged) — accessToken + refreshToken + clientID
//     (+ clientSecret + tokenURL). golang.org/x/oauth2 refreshes the access
//     token transparently when it expires using the refresh token.
//  3. Static bearer — accessToken only. No refresh; used for short-lived tokens
//     (a Google Workspace add-on event's userOAuthToken, a pre-fetched Graph
//     token) and in tests, where it lets the mock server see a fixed token
//     without standing up a token endpoint.
//
// All modes layer on the repo's resilient HTTP transport so transient 5xx/429
// responses are retried (see [httputil.NewResilientClient]).
type oauthConfig struct {
	accessToken  string
	refreshToken string
	clientID     string
	clientSecret string
	tokenURL     string
	scopes       []string
	expiry       time.Time
}

// oauthConfigFromMap reads the standard oauth_* keys from a connector config
// map. defaultTokenURL and defaultScopes fill in provider defaults when the
// config omits them.
//
// Recognised keys:
//
//	oauth_access_token   short-lived bearer (or seed for the refreshing source)
//	oauth_refresh_token  long-lived refresh token (3-legged user OAuth)
//	oauth_client_id      OAuth client / Entra application ID
//	oauth_client_secret  OAuth client secret
//	oauth_token_url      token endpoint (defaults to defaultTokenURL)
//	oauth_scopes         space- or comma-separated scope list
func oauthConfigFromMap(config map[string]string, defaultTokenURL string, defaultScopes []string) oauthConfig {
	o := oauthConfig{
		accessToken:  config["oauth_access_token"],
		refreshToken: config["oauth_refresh_token"],
		clientID:     config["oauth_client_id"],
		clientSecret: config["oauth_client_secret"],
		tokenURL:     config["oauth_token_url"],
		scopes:       defaultScopes,
	}
	if s := strings.TrimSpace(config["oauth_scopes"]); s != "" {
		o.scopes = splitScopes(s)
	}
	if o.tokenURL == "" {
		o.tokenURL = defaultTokenURL
	}
	return o
}

// hasCredentials reports whether any usable credential mode is configured.
func (o oauthConfig) hasCredentials() bool {
	if o.accessToken != "" {
		return true
	}
	return o.clientID != "" && o.clientSecret != "" && o.tokenURL != ""
}

// httpClient returns an *http.Client that injects (and where possible refreshes)
// the bearer token on every request.
func (o oauthConfig) httpClient(ctx context.Context) (*http.Client, error) {
	// Borrow the resilient transport as the base for token + API requests.
	ctx = context.WithValue(ctx, oauth2.HTTPClient, httputil.NewResilientClient())

	switch {
	case o.accessToken == "" && o.clientID != "" && o.clientSecret != "" && o.tokenURL != "":
		// (1) App-only client credentials.
		cc := &clientcredentials.Config{
			ClientID:     o.clientID,
			ClientSecret: o.clientSecret,
			TokenURL:     o.tokenURL,
			Scopes:       o.scopes,
		}
		return cc.Client(ctx), nil

	case o.refreshToken != "" && o.clientID != "" && o.tokenURL != "":
		// (2) Refreshing 3-legged user OAuth.
		cfg := &oauth2.Config{
			ClientID:     o.clientID,
			ClientSecret: o.clientSecret,
			Endpoint:     oauth2.Endpoint{TokenURL: o.tokenURL},
			Scopes:       o.scopes,
		}
		tok := &oauth2.Token{
			AccessToken:  o.accessToken,
			RefreshToken: o.refreshToken,
			Expiry:       o.expiry,
			TokenType:    "Bearer",
		}
		return cfg.Client(ctx, tok), nil

	case o.accessToken != "":
		// (3) Static bearer.
		ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: o.accessToken, TokenType: "Bearer"})
		return oauth2.NewClient(ctx, ts), nil

	default:
		return nil, errors.New("no OAuth credentials: set oauth_access_token, a refresh token + client, or client-credentials (oauth_client_id/oauth_client_secret/oauth_token_url)")
	}
}

// splitScopes splits a space- or comma-separated scope list, trimming blanks.
func splitScopes(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == ',' || r == '\t' || r == '\n' })
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}
